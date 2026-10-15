package observability

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	sdklog "go.opentelemetry.io/otel/sdk/log"

	"github.com/botbooker/bb-core/pkg/logger"
)

const (
	// envLogsEndpoint — адрес OTel Collector именно для логов (приоритет).
	envLogsEndpoint = "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT"
)

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
		slog.Debug("otlp logs export disabled")

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

	slog.Info("otlp logs export enabled", slog.String("endpoint", endpoint), "protocol", "gRPC")

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

// logExporterOptions приводит адрес коллектора к виду, понятному OTLP-экспортёру
// логов (см. normalizeEndpoint).
func logExporterOptions(endpoint string) []otlploggrpc.Option {
	return []otlploggrpc.Option{otlploggrpc.WithEndpointURL(normalizeEndpoint(endpoint))}
}
