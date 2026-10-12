package serverapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/botbooker/bb-core/pkg/logger"
)

const (
	// instrumentationScope — имя инструментария (scope) для логов приложения.
	instrumentationScope = "github.com/botbooker/bb-core"

	// envLogsEndpoint — адрес OTel Collector именно для логов (приоритет).
	envLogsEndpoint = "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT"
	// envCommonEndpoint — общий адрес OTel Collector для всех сигналов.
	envCommonEndpoint = "OTEL_EXPORTER_OTLP_ENDPOINT"
)

// setupOTelSDK bootstraps the OpenTelemetry pipeline.
// If it does not return an error, make sure to call shutdown for proper cleanup.
func setupOTelSDK(ctx context.Context) (func(context.Context) error, error) {
	var shutdownFuncs []func(context.Context) error
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

	// Create trace exporter using environment variables
	spanExporter, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		slog.Error("Create trace exporter", "error", err)
	}

	// Create trace provider with the exporter
	if !autoexport.IsNoneSpanExporter(spanExporter) {
		tracerProvider := sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(spanExporter),
		)
		shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
		otel.SetTracerProvider(tracerProvider)
	}

	// Create metric reader using environment variables
	metricReader, err := autoexport.NewMetricReader(ctx)
	if err != nil {
		slog.Error("Create metric exporter", "error", err)
	}

	// Create meter provider with the reader
	if !autoexport.IsNoneMetricReader(metricReader) {
		meterProvider := sdkmetric.NewMeterProvider(
			sdkmetric.WithReader(metricReader),
		)
		shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
		otel.SetMeterProvider(meterProvider)
	}

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

// logExporterOptions приводит адрес коллектора к виду, понятному OTLP-экспортёру.
// Схема задаёт транспорт: "http" — plaintext gRPC, "https" — TLS. Адрес без
// схемы (например, localhost:4317) трактуется как http, потому что локальный
// OTel Collector по умолчанию слушает gRPC без TLS.
func logExporterOptions(endpoint string) []otlploggrpc.Option {
	if !strings.Contains(endpoint, "://") {
		endpoint = "http://" + endpoint
	}

	return []otlploggrpc.Option{otlploggrpc.WithEndpointURL(endpoint)}
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}
