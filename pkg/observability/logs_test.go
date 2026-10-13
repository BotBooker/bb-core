package observability

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
)

func TestLogsEndpoint(t *testing.T) {
	t.Run("disabled without env vars", func(t *testing.T) {
		t.Setenv(envLogsEndpoint, "")
		t.Setenv(envCommonEndpoint, "")

		endpoint, enabled := logsEndpoint()
		if enabled {
			t.Error("logsEndpoint() должен быть выключен без переменных окружения")
		}
		if endpoint != "" {
			t.Errorf("logsEndpoint() endpoint = %q, ожидалось пустое значение", endpoint)
		}
	})

	t.Run("common endpoint", func(t *testing.T) {
		t.Setenv(envLogsEndpoint, "")
		t.Setenv(envCommonEndpoint, "http://localhost:4317")

		endpoint, enabled := logsEndpoint()
		if !enabled {
			t.Error("logsEndpoint() должен быть включён при заданном OTEL_EXPORTER_OTLP_ENDPOINT")
		}
		if endpoint != "http://localhost:4317" {
			t.Errorf("logsEndpoint() endpoint = %q, ожидалось %q", endpoint, "http://localhost:4317")
		}
	})

	t.Run("logs endpoint has priority", func(t *testing.T) {
		t.Setenv(envLogsEndpoint, "http://logs:4317")
		t.Setenv(envCommonEndpoint, "http://common:4317")

		endpoint, enabled := logsEndpoint()
		if !enabled {
			t.Error("logsEndpoint() должен быть включён при заданном OTEL_EXPORTER_OTLP_LOGS_ENDPOINT")
		}
		if endpoint != "http://logs:4317" {
			t.Errorf("logsEndpoint() endpoint = %q, ожидалось %q", endpoint, "http://logs:4317")
		}
	})
}

func TestLogExporterOptions(t *testing.T) {
	endpoints := []string{
		"localhost:4317",
		"http://localhost:4317",
		"https://collector.example.com:4317",
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint, func(t *testing.T) {
			opts := logExporterOptions(endpoint)
			if len(opts) != 1 {
				t.Fatalf("logExporterOptions() вернул %d опций, ожидалась 1", len(opts))
			}

			// Адрес без схемы тоже должен приниматься: экспортёр разбирает его как URL.
			if _, err := otlploggrpc.New(context.Background(), opts...); err != nil {
				t.Fatalf("otlploggrpc.New() вернул ошибку: %v", err)
			}
		})
	}
}

func TestSetupLogs_Disabled(t *testing.T) {
	t.Setenv(envLogsEndpoint, "")
	t.Setenv(envCommonEndpoint, "")

	shutdown, err := setupLogs(context.Background())
	if err != nil {
		t.Fatalf("setupLogs() вернул ошибку: %v", err)
	}
	if shutdown == nil {
		t.Fatal("setupLogs() вернул nil shutdown")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("no-op shutdown() вернул ошибку: %v", err)
	}
}

func TestSetupLogs_Enabled(t *testing.T) {
	t.Setenv(envLogsEndpoint, "")
	t.Setenv(envCommonEndpoint, "http://localhost:4317")

	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	shutdown, err := setupLogs(ctx)
	if err != nil {
		t.Fatalf("setupLogs() вернул ошибку: %v", err)
	}
	if shutdown == nil {
		t.Fatal("setupLogs() вернул nil shutdown")
	}

	// Коллектора в тестовом окружении может не быть, поэтому ошибка экспорта
	// при закрытии допустима — важно, что провайдер корректно закрывается.
	if err := shutdown(ctx); err != nil {
		t.Logf("shutdown() вернул ошибку (коллектор недоступен?): %v", err)
	}
}
