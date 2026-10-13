package serverapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/botbooker/bb-core/pkg/observability"
)

// envForceTrace повторяет имя переменной окружения, которой управляет
// pkg/observability: тесты middleware включают форс-трассировку так же, как это
// делает окружение приложения.
const envForceTrace = "ENABLE_FORCE_TRACE"

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
	otel.SetTextMapPropagator(observability.NewPropagator())

	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)

		_ = provider.Shutdown(context.Background())
	})

	return exporter
}

func TestTraceMiddleware(t *testing.T) {
	t.Run("continues incoming trace", func(t *testing.T) {
		t.Setenv(envForceTrace, "")

		exporter := withTestTracing(t, observability.NewTraceSampler())
		handler := traceMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.Header.Set("traceparent", "00-"+testTraceID.String()+"-"+testSpanID.String()+"-01")
		handler.ServeHTTP(httptest.NewRecorder(), req)

		spans := exporter.GetSpans()
		if len(spans) != 1 {
			t.Fatalf("записано %d спанов, ожидался 1", len(spans))
		}

		if got := spans[0].SpanContext.TraceID(); got != testTraceID {
			t.Errorf("TraceID = %s, ожидался %s из traceparent", got, testTraceID)
		}
		if !spans[0].SpanContext.IsSampled() {
			t.Error("спан входящей трассы должен быть сэмплирован")
		}
		if spans[0].SpanKind != trace.SpanKindServer {
			t.Errorf("SpanKind = %v, ожидался SpanKindServer", spans[0].SpanKind)
		}
	})

	t.Run("drops request without traceparent", func(t *testing.T) {
		t.Setenv(envForceTrace, "")

		exporter := withTestTracing(t, observability.NewTraceSampler())
		handler := traceMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))

		if got := len(exporter.GetSpans()); got != 0 {
			t.Errorf("записано %d спанов без traceparent, ожидалось 0", got)
		}
	})

	t.Run("force trace records request without traceparent", func(t *testing.T) {
		t.Setenv(envForceTrace, "true")

		exporter := withTestTracing(t, observability.NewTraceSampler())
		handler := traceMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))

		spans := exporter.GetSpans()
		if len(spans) != 1 {
			t.Fatalf("записано %d спанов в форс-режиме, ожидался 1", len(spans))
		}
		if !spans[0].SpanContext.IsSampled() {
			t.Error("спан в форс-режиме должен быть сэмплирован")
		}
	})
}

// TestStartupAndShutdownSpans проверяет спаны жизненного цикла приложения: они
// начинают собственные трассы и записываются без входящего traceparent за счёт
// атрибута observability.ForceSample().
func TestStartupAndShutdownSpans(t *testing.T) {
	t.Setenv(envForceTrace, "")

	exporter := withTestTracing(t, observability.NewTraceSampler())

	_, startupSpan := startStartupSpan(context.Background())
	startupSpan.End()

	_, shutdownSpan := startShutdownSpan(context.Background())
	shutdownSpan.End()

	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("записано %d спанов, ожидалось 2 (startup и shutdown)", len(spans))
	}

	byName := make(map[string]tracetest.SpanStub, len(spans))
	for _, span := range spans {
		byName[span.Name] = span
	}

	startup, ok := byName["app.startup"]
	if !ok {
		t.Fatal("спан app.startup не записан")
	}

	shutdown, ok := byName["app.shutdown"]
	if !ok {
		t.Fatal("спан app.shutdown не записан")
	}

	for name, span := range byName {
		if !span.SpanContext.TraceID().IsValid() {
			t.Errorf("%s: TraceID невалиден", name)
		}
		if !span.SpanContext.IsSampled() {
			t.Errorf("%s: спан должен быть сэмплирован", name)
		}
	}

	if startup.SpanContext.TraceID() == shutdown.SpanContext.TraceID() {
		t.Error("стартовый и завершающий спаны должны иметь разные TraceID")
	}
}
