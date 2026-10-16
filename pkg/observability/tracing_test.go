package observability

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
)

var (
	// testTraceID и testSpanID — валидные идентификаторы для тестов спанов.
	testTraceID = trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	testSpanID  = trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8}
)

// withTestTracing подменяет глобальные провайдер трассировки и пропагатор на
// тестовые: спаны пишутся в in-memory экспортёр, а W3C Trace Context извлекается
// так же, как в приложении. Исходные значения возвращаются после теста.
func withTestTracing(t *testing.T, sampler sdktrace.Sampler) *tracetest.InMemoryExporter {
	t.Helper()

	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sampler),
		sdktrace.WithSyncer(exporter),
	)

	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()

	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(NewPropagator())

	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)

		_ = provider.Shutdown(context.Background())
	})

	return exporter
}

// spanContext собирает SpanContext с заданными флагами и признаком удалённости.
func spanContext(traceID trace.TraceID, spanID trace.SpanID, flags trace.TraceFlags, remote bool) trace.SpanContext {
	return trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: flags,
		Remote:     remote,
	})
}

func TestTracesEndpoint(t *testing.T) {
	t.Run("disabled without env vars", func(t *testing.T) {
		t.Setenv(envTracesEndpoint, "")
		t.Setenv(envCommonEndpoint, "")

		endpoint, enabled := tracesEndpoint()
		if enabled {
			t.Error("tracesEndpoint() должен быть выключен без переменных окружения")
		}
		if endpoint != "" {
			t.Errorf("tracesEndpoint() endpoint = %q, ожидалось пустое значение", endpoint)
		}
	})

	t.Run("common endpoint", func(t *testing.T) {
		t.Setenv(envTracesEndpoint, "")
		t.Setenv(envCommonEndpoint, "http://localhost:4317")

		endpoint, enabled := tracesEndpoint()
		if !enabled {
			t.Error("tracesEndpoint() должен быть включён при заданном OTEL_EXPORTER_OTLP_ENDPOINT")
		}
		if endpoint != "http://localhost:4317" {
			t.Errorf("tracesEndpoint() endpoint = %q, ожидалось %q", endpoint, "http://localhost:4317")
		}
	})

	t.Run("traces endpoint has priority", func(t *testing.T) {
		t.Setenv(envTracesEndpoint, "http://traces:4317")
		t.Setenv(envCommonEndpoint, "http://common:4317")

		endpoint, enabled := tracesEndpoint()
		if !enabled {
			t.Error("tracesEndpoint() должен быть включён при заданном OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")
		}
		if endpoint != "http://traces:4317" {
			t.Errorf("tracesEndpoint() endpoint = %q, ожидалось %q", endpoint, "http://traces:4317")
		}
	})
}

func TestTraceExporterOptions(t *testing.T) {
	endpoints := []string{
		"localhost:4317",
		"http://localhost:4317",
		"localhost", // порт по умолчанию — 4317
		"https://collector.example.com:4317",
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint, func(t *testing.T) {
			opts := traceExporterOptions(endpoint)
			if len(opts) != 1 {
				t.Fatalf("traceExporterOptions() вернул %d опций, ожидалась 1", len(opts))
			}

			if _, err := otlptracegrpc.New(context.Background(), opts...); err != nil {
				t.Fatalf("otlptracegrpc.New() вернул ошибку: %v", err)
			}
		})
	}
}

func TestForceTraceEnabled(t *testing.T) {
	tests := map[string]bool{
		"":      false,
		"true":  true,
		"TRUE":  true,
		"True":  true,
		"false": false,
		"1":     false,
	}

	for value, want := range tests {
		t.Run("value="+value, func(t *testing.T) {
			t.Setenv(envForceTrace, value)

			if got := ForceTraceEnabled(); got != want {
				t.Errorf("ForceTraceEnabled() = %t при %s=%q, ожидалось %t", got, envForceTrace, value, want)
			}
		})
	}
}

func TestTraceSampler_ShouldSample(t *testing.T) {
	remoteSampled := spanContext(testTraceID, testSpanID, trace.FlagsSampled, true)
	remoteNotSampled := spanContext(testTraceID, testSpanID, 0, true)
	localSampled := spanContext(testTraceID, testSpanID, trace.FlagsSampled, false)
	localNotSampled := spanContext(testTraceID, testSpanID, 0, false)

	tests := []struct {
		name    string
		sampler traceSampler
		params  sdktrace.SamplingParameters
		want    sdktrace.SamplingDecision
	}{
		{
			name:    "scenario 1: force trace records a root span",
			sampler: traceSampler{force: true},
			params:  sdktrace.SamplingParameters{ParentContext: context.Background()},
			want:    sdktrace.RecordAndSample,
		},
		{
			name:    "scenario 2: remote parent continues the trace",
			sampler: traceSampler{},
			params: sdktrace.SamplingParameters{
				ParentContext: trace.ContextWithRemoteSpanContext(context.Background(), remoteSampled),
			},
			want: sdktrace.RecordAndSample,
		},
		{
			name:    "scenario 2: remote parent without sampled flag is continued too",
			sampler: traceSampler{},
			params: sdktrace.SamplingParameters{
				ParentContext: trace.ContextWithRemoteSpanContext(context.Background(), remoteNotSampled),
			},
			want: sdktrace.RecordAndSample,
		},
		{
			name:    "scenario 3-4: span marked for mandatory recording",
			sampler: traceSampler{},
			params: sdktrace.SamplingParameters{
				ParentContext: context.Background(),
				Attributes:    []attribute.KeyValue{ForceSample()},
			},
			want: sdktrace.RecordAndSample,
		},
		{
			name:    "child of a sampled local parent is recorded",
			sampler: traceSampler{},
			params: sdktrace.SamplingParameters{
				ParentContext: trace.ContextWithSpanContext(context.Background(), localSampled),
			},
			want: sdktrace.RecordAndSample,
		},
		{
			name:    "child of a dropped local parent is dropped",
			sampler: traceSampler{},
			params: sdktrace.SamplingParameters{
				ParentContext: trace.ContextWithSpanContext(context.Background(), localNotSampled),
			},
			want: sdktrace.Drop,
		},
		{
			name:    "root span without parent and without force is dropped",
			sampler: traceSampler{},
			params:  sdktrace.SamplingParameters{ParentContext: context.Background()},
			want:    sdktrace.Drop,
		},
		{
			name:    "foreign attribute does not force sampling",
			sampler: traceSampler{},
			params: sdktrace.SamplingParameters{
				ParentContext: context.Background(),
				Attributes:    []attribute.KeyValue{attribute.String("bb.core.force_sample", "yes")},
			},
			want: sdktrace.Drop,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.sampler.ShouldSample(test.params)
			if got.Decision != test.want {
				t.Errorf("ShouldSample() decision = %v, ожидалось %v", got.Decision, test.want)
			}
		})
	}
}

func TestNewTraceSampler(t *testing.T) {
	t.Run("force trace from environment", func(t *testing.T) {
		t.Setenv(envForceTrace, forceTraceValue)

		description := NewTraceSampler().Description()
		if description != "bb-core-trace-sampler{force=true}" {
			t.Errorf("Description() = %q, ожидалось упоминание force=true", description)
		}
	})

	t.Run("force trace disabled by default", func(t *testing.T) {
		t.Setenv(envForceTrace, "")

		description := NewTraceSampler().Description()
		if description != "bb-core-trace-sampler{force=false}" {
			t.Errorf("Description() = %q, ожидалось упоминание force=false", description)
		}
	})
}

func TestSetupTraces_Disabled(t *testing.T) {
	t.Setenv(envTracesEndpoint, "")
	t.Setenv(envCommonEndpoint, "")

	before := otel.GetTracerProvider()

	shutdown, err := setupTraces(context.Background())
	if err != nil {
		t.Fatalf("setupTraces() вернул ошибку: %v", err)
	}
	if shutdown == nil {
		t.Fatal("setupTraces() вернул nil shutdown")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("no-op shutdown() вернул ошибку: %v", err)
	}

	if after := otel.GetTracerProvider(); after != before {
		t.Error("setupTraces() не должен менять провайдер трассировки без адреса коллектора")
	}
}

// otelTraceReceiver — минимальный OTLP-коллектор трасс для тестов: принимает
// ExportTraceServiceRequest и считает полученные спаны.
type otelTraceReceiver struct {
	collectortracepb.UnimplementedTraceServiceServer

	mu    sync.Mutex
	spans int
}

func (r *otelTraceReceiver) Export(
	_ context.Context,
	req *collectortracepb.ExportTraceServiceRequest,
) (*collectortracepb.ExportTraceServiceResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, resourceSpans := range req.GetResourceSpans() {
		for _, scopeSpans := range resourceSpans.GetScopeSpans() {
			r.spans += len(scopeSpans.GetSpans())
		}
	}

	return &collectortracepb.ExportTraceServiceResponse{}, nil
}

func (r *otelTraceReceiver) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.spans
}

// startTraceCollector поднимает gRPC-сервер OTLP-трасс на свободном порту и
// возвращает приёмник и его адрес.
func startTraceCollector(t *testing.T) (*otelTraceReceiver, string) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("не удалось занять порт под коллектор: %v", err)
	}

	receiver := &otelTraceReceiver{}
	server := grpc.NewServer()

	collectortracepb.RegisterTraceServiceServer(server, receiver)

	go func() { _ = server.Serve(listener) }()

	t.Cleanup(server.Stop)

	return receiver, listener.Addr().String()
}

// TestSetupTraces_ExportsToCollector — сквозная проверка: трассы приложения
// уходят в OTel Collector по OTLP/gRPC, когда задан адрес коллектора.
func TestSetupTraces_ExportsToCollector(t *testing.T) {
	receiver, addr := startTraceCollector(t)

	t.Setenv(envTracesEndpoint, "http://"+addr)
	t.Setenv(envCommonEndpoint, "")
	t.Setenv(envForceTrace, "")

	previous := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(previous) })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	shutdown, err := setupTraces(ctx)
	if err != nil {
		t.Fatalf("setupTraces() вернул ошибку: %v", err)
	}

	// Спан помечен маркером обязательной записи — так же помечаются app.startup
	// и app.shutdown в приложении, — поэтому он попадёт в экспорт даже без
	// форс-режима и входящего traceparent.
	_, span := Tracer().Start(ctx, "app.startup",
		trace.WithNewRoot(),
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(ForceSample()),
	)
	span.End()

	if err := shutdown(ctx); err != nil {
		t.Fatalf("shutdown() не смог выгрузить трассы: %v", err)
	}

	if got := receiver.count(); got == 0 {
		t.Error("коллектор не получил ни одного спана")
	}
}

func TestTraceSpan(t *testing.T) {
	t.Run("name, tracer name and tags", func(t *testing.T) {
		exporter := withTestTracing(t, sdktrace.AlwaysSample())

		ctx, end := TraceSpan(context.Background(), "save-to-db",
			WithTracerName("database-client"),
			WithTags(
				attribute.Int("invoice.id", 42),
				attribute.String("db.table", "invoices"),
			),
		)
		end()

		spans := exporter.GetSpans()
		if len(spans) != 1 {
			t.Fatalf("записано %d спанов, ожидался 1", len(spans))
		}

		span := spans[0]
		if span.Name != "save-to-db" {
			t.Errorf("Name = %q, ожидалось %q", span.Name, "save-to-db")
		}
		if span.InstrumentationScope.Name != "database-client" {
			t.Errorf("InstrumentationScope = %q, ожидалось %q", span.InstrumentationScope.Name, "database-client")
		}

		attrs := make(map[string]attribute.Value, len(span.Attributes))
		for _, attr := range span.Attributes {
			attrs[string(attr.Key)] = attr.Value
		}
		if got := attrs["invoice.id"].AsInt64(); got != 42 {
			t.Errorf("invoice.id = %d, ожидалось 42", got)
		}
		if got := attrs["db.table"].AsString(); got != "invoices" {
			t.Errorf("db.table = %q, ожидалось %q", got, "invoices")
		}

		// Контекст возвращается со спаном: вложенный код пишет спаны в ту же трассу.
		if got := trace.SpanContextFromContext(ctx).TraceID(); got != span.SpanContext.TraceID() {
			t.Errorf("TraceID контекста = %s, ожидался TraceID спана %s", got, span.SpanContext.TraceID())
		}
	})

	t.Run("default tracer name without tags", func(t *testing.T) {
		exporter := withTestTracing(t, sdktrace.AlwaysSample())

		_, end := TraceSpan(context.Background(), "run-background")
		end()

		spans := exporter.GetSpans()
		if len(spans) != 1 {
			t.Fatalf("записано %d спанов, ожидался 1", len(spans))
		}

		if got := spans[0].InstrumentationScope.Name; got != "runBackground" {
			t.Errorf("InstrumentationScope = %q, ожидалось %q", got, "runBackground")
		}
		if got := len(spans[0].Attributes); got != 0 {
			t.Errorf("атрибутов = %d, ожидалось 0", got)
		}
	})
}
