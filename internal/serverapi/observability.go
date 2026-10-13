package serverapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strings"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"

	"github.com/botbooker/bb-core/pkg/logger"
)

const (
	// instrumentationScope — имя инструментария (scope) для логов приложения.
	instrumentationScope = "github.com/botbooker/bb-core"

	// envLogsEndpoint — адрес OTel Collector именно для логов (приоритет).
	envLogsEndpoint = "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT"
	// envCommonEndpoint — общий адрес OTel Collector для всех сигналов.
	envCommonEndpoint = "OTEL_EXPORTER_OTLP_ENDPOINT"

	// defaultCollectorPort — порт OTLP/gRPC, на котором по умолчанию слушает
	// OTel Collector.
	defaultCollectorPort = "4317"
)

// setupOTelSDK bootstraps the OpenTelemetry pipeline.
// If it does not return an error, make sure to call shutdown for proper cleanup.
func setupOTelSDK(ctx context.Context) (func(context.Context) error, error) {
	// shutdownFuncs собирает функции очистки — по одной на сигнал (трассировка,
	// метрики, логи): все три добавляются всегда, независимо от ошибок создания
	// экспортёра, поэтому ёмкость слайса известна заранее.
	shutdownFuncs := make([]func(context.Context) error, 0, 3)

	var err error

	// shutdown calls cleanup functions registered via shutdownFuncs.
	// The errors from the calls are joined.
	// Each registered cleanup will be invoked once.
	shutdown := func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	// Set up propagator.
	prop := newPropagator()
	otel.SetTextMapPropagator(prop)

	// Set up trace export to the OTel Collector. Трассировка включается, только
	// если задан адрес коллектора (см. setupTraces): без него провайдер
	// трассировки не создаётся вовсе и глобальный трейсер остаётся no-op.
	traceShutdown, traceErr := setupTraces(ctx)
	if traceErr != nil {
		slog.Error("Create trace exporter", "error", traceErr)
		err = errors.Join(err, traceErr)
	}
	shutdownFuncs = append(shutdownFuncs, traceShutdown)

	// Set up metrics export to the OTel Collector. Метрики включаются, только
	// если задан адрес коллектора (см. setupMetrics): без него провайдер метрик
	// не создаётся вовсе и глобальный meter provider остаётся no-op.
	metricShutdown, metricErr := setupMetrics(ctx)
	if metricErr != nil {
		slog.Error("Create metric exporter", "error", metricErr)
		err = errors.Join(err, metricErr)
	}
	shutdownFuncs = append(shutdownFuncs, metricShutdown)

	// Set up log export to the OTel Collector and bridge slog to OpenTelemetry.
	// Логи уходят в коллектор только когда задан его адрес, см. setupLogs.
	logShutdown, logErr := setupLogs(ctx)
	if logErr != nil {
		slog.Error("Create log exporter", "error", logErr)
		err = errors.Join(err, logErr)
	}
	shutdownFuncs = append(shutdownFuncs, logShutdown)

	return shutdown, err
}

// setupLogs настраивает экспорт логов приложения в OTel Collector по OTLP/gRPC
// и подключает его к глобальному логгеру slog, сохраняя привычный вывод в
// stdout.
//
// Экспорт включается, только если задан адрес коллектора: переменная окружения
// OTEL_EXPORTER_OTLP_LOGS_ENDPOINT (приоритет) или общая
// OTEL_EXPORTER_OTLP_ENDPOINT. Если адрес не задан, логи пишутся только в
// stdout, а функция возвращает no-op shutdown.
func setupLogs(ctx context.Context) (func(context.Context) error, error) {
	endpoint, enabled := logsEndpoint()
	if !enabled {
		slog.Debug("otlp logs export disabled: collector endpoint is not set",
			"logs_endpoint_env", envLogsEndpoint,
			"common_endpoint_env", envCommonEndpoint,
		)

		return func(context.Context) error { return nil }, nil
	}

	exporter, err := otlploggrpc.New(ctx, logExporterOptions(endpoint)...)
	if err != nil {
		return func(context.Context) error { return nil }, fmt.Errorf("create otlp log exporter: %w", err)
	}

	provider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
	)

	logger.SetOTelHandler(otelslog.NewHandler(instrumentationScope, otelslog.WithLoggerProvider(provider)))

	slog.Info("otlp logs export enabled", "endpoint", endpoint, "protocol", "grpc")

	return provider.Shutdown, nil
}

// logsEndpoint возвращает адрес OTLP-коллектора для логов и признак того, что
// экспорт логов включён. OTEL_EXPORTER_OTLP_LOGS_ENDPOINT имеет приоритет над
// общей OTEL_EXPORTER_OTLP_ENDPOINT, как и требует спецификация OpenTelemetry.
func logsEndpoint() (endpoint string, enabled bool) {
	if endpoint = os.Getenv(envLogsEndpoint); endpoint != "" {
		return endpoint, true
	}
	if endpoint = os.Getenv(envCommonEndpoint); endpoint != "" {
		return endpoint, true
	}

	return "", false
}

// normalizeEndpoint приводит адрес коллектора к URL, понятному OTLP-экспортёру.
// Схема задаёт транспорт: "http" — plaintext gRPC, "https" — TLS. Адрес без схемы
// (например, localhost:4317) трактуется как http, потому что локальный OTel
// Collector по умолчанию слушает gRPC без TLS. Если порт не указан, подставляется
// порт коллектора по умолчанию — 4317.
func normalizeEndpoint(endpoint string) string {
	if !strings.Contains(endpoint, "://") {
		endpoint = "http://" + endpoint
	}

	parsed, err := url.Parse(endpoint)
	if err != nil {
		// Разобрать адрес всё равно попробует экспортёр — он вернёт понятную
		// ошибку создания, а мы не теряем исходное значение.
		return endpoint
	}

	if parsed.Port() == "" {
		parsed.Host = net.JoinHostPort(parsed.Hostname(), defaultCollectorPort)
	}

	return parsed.String()
}

// logExporterOptions приводит адрес коллектора к виду, понятному OTLP-экспортёру
// логов (см. normalizeEndpoint).
func logExporterOptions(endpoint string) []otlploggrpc.Option {
	return []otlploggrpc.Option{otlploggrpc.WithEndpointURL(normalizeEndpoint(endpoint))}
}

// глобальный пропогатор.
// стандарт W3C Trace Context
func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}
