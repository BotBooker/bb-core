package observability

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	collectormetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/grpc"
)

func TestMetricsEndpoint(t *testing.T) {
	t.Run("disabled without env vars", func(t *testing.T) {
		t.Setenv(envMetricsEndpoint, "")
		t.Setenv(envCommonEndpoint, "")

		endpoint, enabled := metricsEndpoint()
		if enabled {
			t.Error("metricsEndpoint() должен быть выключен без переменных окружения")
		}
		if endpoint != "" {
			t.Errorf("metricsEndpoint() endpoint = %q, ожидалось пустое значение", endpoint)
		}
	})

	t.Run("common endpoint", func(t *testing.T) {
		t.Setenv(envMetricsEndpoint, "")
		t.Setenv(envCommonEndpoint, "http://localhost:4317")

		endpoint, enabled := metricsEndpoint()
		if !enabled {
			t.Error("metricsEndpoint() должен быть включён при заданном OTEL_EXPORTER_OTLP_ENDPOINT")
		}
		if endpoint != "http://localhost:4317" {
			t.Errorf("metricsEndpoint() endpoint = %q, ожидалось %q", endpoint, "http://localhost:4317")
		}
	})

	t.Run("metrics endpoint has priority", func(t *testing.T) {
		t.Setenv(envMetricsEndpoint, "http://metrics:4317")
		t.Setenv(envCommonEndpoint, "http://common:4317")

		endpoint, enabled := metricsEndpoint()
		if !enabled {
			t.Error("metricsEndpoint() должен быть включён при заданном OTEL_EXPORTER_OTLP_METRICS_ENDPOINT")
		}
		if endpoint != "http://metrics:4317" {
			t.Errorf("metricsEndpoint() endpoint = %q, ожидалось %q", endpoint, "http://metrics:4317")
		}
	})
}

func TestMetricExporterOptions(t *testing.T) {
	endpoints := []string{
		"localhost:4317",
		"http://localhost:4317",
		"https://collector.example.com:4317",
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint, func(t *testing.T) {
			opts := metricExporterOptions(endpoint)
			if len(opts) != 1 {
				t.Fatalf("metricExporterOptions() вернул %d опций, ожидалась 1", len(opts))
			}

			// Адрес без схемы тоже должен приниматься: экспортёр разбирает его как URL.
			exporter, err := otlpmetricgrpc.New(context.Background(), opts...)
			if err != nil {
				t.Fatalf("otlpmetricgrpc.New() вернул ошибку: %v", err)
			}
			t.Cleanup(func() { _ = exporter.Shutdown(context.Background()) })
		})
	}
}

func TestSetupMetrics_Disabled(t *testing.T) {
	t.Setenv(envMetricsEndpoint, "")
	t.Setenv(envCommonEndpoint, "")

	before := otel.GetMeterProvider()

	shutdown, err := setupMetrics(context.Background())
	if err != nil {
		t.Fatalf("setupMetrics() вернул ошибку: %v", err)
	}
	if shutdown == nil {
		t.Fatal("setupMetrics() вернул nil shutdown")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("no-op shutdown() вернул ошибку: %v", err)
	}

	if after := otel.GetMeterProvider(); after != before {
		t.Error("setupMetrics() не должен менять провайдер метрик без адреса коллектора")
	}
}

// otelMetricReceiver — минимальный OTLP-коллектор метрик для тестов: принимает
// ExportMetricsServiceRequest и считает полученные точки данных.
type otelMetricReceiver struct {
	collectormetricspb.UnimplementedMetricsServiceServer

	mu     sync.Mutex
	points int
}

func (r *otelMetricReceiver) Export(
	_ context.Context,
	req *collectormetricspb.ExportMetricsServiceRequest,
) (*collectormetricspb.ExportMetricsServiceResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, resourceMetrics := range req.GetResourceMetrics() {
		for _, scopeMetrics := range resourceMetrics.GetScopeMetrics() {
			for _, metric := range scopeMetrics.GetMetrics() {
				r.points += countDataPoints(metric)
			}
		}
	}

	return &collectormetricspb.ExportMetricsServiceResponse{}, nil
}

func (r *otelMetricReceiver) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.points
}

// countDataPoints считает точки данных метрики независимо от её типа.
func countDataPoints(metric *metricpb.Metric) int {
	switch data := metric.GetData().(type) {
	case *metricpb.Metric_Gauge:
		return len(data.Gauge.GetDataPoints())
	case *metricpb.Metric_Sum:
		return len(data.Sum.GetDataPoints())
	case *metricpb.Metric_Histogram:
		return len(data.Histogram.GetDataPoints())
	case *metricpb.Metric_ExponentialHistogram:
		return len(data.ExponentialHistogram.GetDataPoints())
	case *metricpb.Metric_Summary:
		return len(data.Summary.GetDataPoints())
	default:
		return 0
	}
}

// startMetricCollector поднимает gRPC-сервер OTLP-метрик на свободном порту и
// возвращает приёмник и его адрес.
func startMetricCollector(t *testing.T) (*otelMetricReceiver, string) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("не удалось занять порт под коллектор: %v", err)
	}

	receiver := &otelMetricReceiver{}
	server := grpc.NewServer()

	collectormetricspb.RegisterMetricsServiceServer(server, receiver)

	go func() { _ = server.Serve(listener) }()

	t.Cleanup(server.Stop)

	return receiver, listener.Addr().String()
}

// TestSetupMetrics_ExportsToCollector — сквозная проверка: метрики приложения
// уходят в OTel Collector по OTLP/gRPC, когда задан адрес коллектора.
func TestSetupMetrics_ExportsToCollector(t *testing.T) {
	receiver, addr := startMetricCollector(t)

	t.Setenv(envMetricsEndpoint, "")
	t.Setenv(envCommonEndpoint, "http://"+addr)

	previous := otel.GetMeterProvider()
	t.Cleanup(func() { otel.SetMeterProvider(previous) })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	shutdown, err := setupMetrics(ctx)
	if err != nil {
		t.Fatalf("setupMetrics() вернул ошибку: %v", err)
	}

	counter, err := otel.Meter("test").Int64Counter("bb.core.test.requests")
	if err != nil {
		t.Fatalf("не удалось создать счётчик: %v", err)
	}
	counter.Add(ctx, 1)

	// Периодический reader отправляет данные и на Shutdown, поэтому метрику
	// достаточно записать до остановки провайдера.
	if err := shutdown(ctx); err != nil {
		t.Fatalf("shutdown() не смог выгрузить метрики: %v", err)
	}

	if got := receiver.count(); got == 0 {
		t.Error("коллектор не получил ни одной точки метрик")
	}
}
