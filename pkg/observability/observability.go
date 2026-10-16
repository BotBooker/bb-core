// Package observability инициализирует OpenTelemetry SDK приложения: настраивает
// экспорт трассировки, метрик и логов в OTel Collector по OTLP/gRPC и подключает
// глобальный логгер slog к OpenTelemetry.
//
// Каждый сигнал включается, только если задан адрес коллектора — переменная
// окружения конкретного сигнала (OTEL_EXPORTER_OTLP_TRACES_ENDPOINT,
// OTEL_EXPORTER_OTLP_METRICS_ENDPOINT, OTEL_EXPORTER_OTLP_LOGS_ENDPOINT) или
// общая OTEL_EXPORTER_OTLP_ENDPOINT. Без адреса соответствующий провайдер не
// создаётся вовсе: глобальные провайдеры остаются no-op, приложение не трогает
// сеть. Порт коллектора по умолчанию — 4317, схема в адресе задаёт транспорт
// (http — plaintext gRPC, https — TLS).
//
// Точка входа — SetupOTelSDK: она возвращает функцию остановки, которую нужно
// вызвать при завершении приложения, чтобы выгрузить накопленные сигналы.
package observability

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/url"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

const (
	// instrumentationScope — имя инструментария (scope) для сигналов приложения.
	instrumentationScope = "github.com/botbooker/bb-core"

	// envCommonEndpoint — общий адрес OTel Collector для всех сигналов.
	envCommonEndpoint = "OTEL_EXPORTER_OTLP_ENDPOINT"

	// defaultCollectorPort — порт OTLP/gRPC, на котором по умолчанию слушает
	// OTel Collector.
	defaultCollectorPort = "4317"

	// protocolGRPC — транспорт OTLP-экспортёров приложения: все три сигнала
	// уходят по gRPC (otlptracegrpc, otlpmetricgrpc, otlploggrpc). Значение
	// поля protocol в единственной Info-записи о включении экспорта.
	protocolGRPC = "gRPC"
)

// SetupOTelSDK bootstraps the OpenTelemetry pipeline.
// If it does not return an error, make sure to call shutdown for proper cleanup.
func SetupOTelSDK(ctx context.Context) (func(context.Context) error, error) {
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
	prop := NewPropagator()
	otel.SetTextMapPropagator(prop)

	// Set up trace export to the OTel Collector. Трассировка включается, только
	// если задан адрес коллектора (см. setupTraces): без него провайдер
	// трассировки не создаётся вовсе и глобальный трейсер остаётся no-op.
	traceShutdown, traceErr := setupTraces(ctx)
	if traceErr != nil {
		slog.ErrorContext(ctx, "Create trace exporter", "error", traceErr)
		err = errors.Join(err, traceErr)
	}
	shutdownFuncs = append(shutdownFuncs, traceShutdown)

	// Set up metrics export to the OTel Collector. Метрики включаются, только
	// если задан адрес коллектора (см. setupMetrics): без него провайдер метрик
	// не создаётся вовсе и глобальный meter provider остаётся no-op.
	metricShutdown, metricErr := setupMetrics(ctx)
	if metricErr != nil {
		slog.ErrorContext(ctx, "Create metric exporter", "error", metricErr)
		err = errors.Join(err, metricErr)
	}
	shutdownFuncs = append(shutdownFuncs, metricShutdown)

	// Set up log export to the OTel Collector and bridge slog to OpenTelemetry.
	// Логи уходят в коллектор только когда задан его адрес, см. setupLogs.
	logShutdown, logErr := setupLogs(ctx)
	if logErr != nil {
		slog.ErrorContext(ctx, "Create log exporter", "error", logErr)
		err = errors.Join(err, logErr)
	}
	shutdownFuncs = append(shutdownFuncs, logShutdown)

	return shutdown, err
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

// NewPropagator собирает пропагатор приложения: W3C Trace Context
// (traceparent, tracestate) и Baggage. Экспортируется, чтобы код приложения мог
// выставить тот же пропагатор вручную — например, в тестах.
func NewPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}
