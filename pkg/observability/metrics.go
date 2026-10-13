package observability

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

const (
	// envMetricsEndpoint — адрес OTel Collector именно для метрик (приоритет).
	envMetricsEndpoint = "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT"
)

// setupMetrics настраивает экспорт метрик приложения в OTel Collector по
// OTLP/gRPC.
//
// Экспорт включается, только если задан адрес коллектора: переменная окружения
// OTEL_EXPORTER_OTLP_METRICS_ENDPOINT (приоритет) или общая
// OTEL_EXPORTER_OTLP_ENDPOINT. Если ни одной из них нет, провайдер метрик не
// создаётся вовсе: глобальный otel.GetMeterProvider остаётся no-op, приложение не
// трогает сеть и не пишет ошибок экспорта, а функция возвращает no-op shutdown.
// Порт коллектора по умолчанию — 4317.
func setupMetrics(ctx context.Context) (func(context.Context) error, error) {
	endpoint, enabled := metricsEndpoint()
	if !enabled {
		slog.Debug("otlp metrics export disabled: collector endpoint is not set",
			"metrics_endpoint_env", envMetricsEndpoint,
			"common_endpoint_env", envCommonEndpoint,
		)

		return func(context.Context) error { return nil }, nil
	}

	exporter, err := otlpmetricgrpc.New(ctx, metricExporterOptions(endpoint)...)
	if err != nil {
		return func(context.Context) error { return nil }, fmt.Errorf("create otlp metric exporter: %w", err)
	}

	// PeriodicReader собирает измерения раз в минуту (интервал по умолчанию) и
	// выгружает их экспортёру. Последний сбор он делает на Shutdown, поэтому
	// метрики, накопленные к моменту остановки приложения, не теряются:
	// provider.Shutdown останавливает reader и дожидается финального экспорта.
	reader := sdkmetric.NewPeriodicReader(exporter)

	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	otel.SetMeterProvider(provider)

	slog.Info("otlp metrics export enabled", "endpoint", endpoint, "protocol", "grpc")

	return provider.Shutdown, nil
}

// metricsEndpoint возвращает адрес OTLP-коллектора для метрик и признак того, что
// экспорт метрик включён. OTEL_EXPORTER_OTLP_METRICS_ENDPOINT имеет приоритет над
// общей OTEL_EXPORTER_OTLP_ENDPOINT, как и требует спецификация OpenTelemetry.
func metricsEndpoint() (endpoint string, enabled bool) {
	if endpoint = os.Getenv(envMetricsEndpoint); endpoint != "" {
		return endpoint, true
	}
	if endpoint = os.Getenv(envCommonEndpoint); endpoint != "" {
		return endpoint, true
	}

	return "", false
}

// metricExporterOptions приводит адрес коллектора к виду, понятному
// OTLP-экспортёру метрик (см. normalizeEndpoint).
func metricExporterOptions(endpoint string) []otlpmetricgrpc.Option {
	return []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpointURL(normalizeEndpoint(endpoint))}
}
