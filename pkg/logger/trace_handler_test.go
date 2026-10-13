package logger_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/botbooker/bb-core/pkg/logger"
)

var (
	// testTraceID и testSpanID — валидные идентификаторы для тестовых спанов.
	testTraceID = trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	testSpanID  = trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8}
)

// tracedContext — контекст с активным спаном: таким его видят логи внутри
// обработки запроса (traceMiddleware) или внутри observability.TraceSpan.
func tracedContext() context.Context {
	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    testTraceID,
		SpanID:     testSpanID,
		TraceFlags: trace.FlagsSampled,
	})

	return trace.ContextWithSpanContext(context.Background(), spanCtx)
}

func TestTraceHandler_Handle_AddsTraceAndSpanID(t *testing.T) {
	var buf bytes.Buffer

	log := slog.New(logger.NewTraceHandler(slog.NewTextHandler(&buf, nil)))
	log.InfoContext(tracedContext(), "with trace")

	out := buf.String()
	if !strings.Contains(out, "trace_id="+testTraceID.String()) {
		t.Errorf("trace_id не добавлен в запись: %q", out)
	}
	if !strings.Contains(out, "span_id="+testSpanID.String()) {
		t.Errorf("span_id не добавлен в запись: %q", out)
	}
}

func TestTraceHandler_Handle_WithoutSpan(t *testing.T) {
	var buf bytes.Buffer

	log := slog.New(logger.NewTraceHandler(slog.NewTextHandler(&buf, nil)))
	log.InfoContext(context.Background(), "without span")
	log.Info("without context")

	out := buf.String()
	if strings.Contains(out, "trace_id") || strings.Contains(out, "span_id") {
		t.Errorf("вне активной трассы атрибуты не должны добавляться: %q", out)
	}
}

func TestTraceHandler_Handle_KeepsExistingAttrs(t *testing.T) {
	var buf bytes.Buffer

	log := slog.New(logger.NewTraceHandler(slog.NewTextHandler(&buf, nil)))
	log.InfoContext(tracedContext(), "with own ids", "trace_id", "custom", "span_id", "custom-span")

	out := buf.String()
	if strings.Contains(out, testTraceID.String()) {
		t.Errorf("готовый trace_id не должен перезаписываться: %q", out)
	}
	if !strings.Contains(out, "trace_id=custom") {
		t.Errorf("свой trace_id потерян: %q", out)
	}
	if !strings.Contains(out, "span_id=custom-span") {
		t.Errorf("свой span_id потерян: %q", out)
	}
}

func TestTraceHandler_Handle_WithoutSampling_AddsIDs(t *testing.T) {
	// Сэмплер отбрасывает корневые спаны без traceparent, но контекст спана
	// остаётся валидным: атрибуты добавляются так же, как их добавляет SDK логов
	// OpenTelemetry для OTLP (проверка сэмплирования там тоже не делается).
	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: testTraceID,
		SpanID:  testSpanID,
	})

	var buf bytes.Buffer

	log := slog.New(logger.NewTraceHandler(slog.NewTextHandler(&buf, nil)))
	log.InfoContext(trace.ContextWithSpanContext(context.Background(), spanCtx), "unsampled trace")

	out := buf.String()
	if !strings.Contains(out, "trace_id="+testTraceID.String()) {
		t.Errorf("trace_id не добавлен для несэмплированной трассы: %q", out)
	}
	if !strings.Contains(out, "span_id="+testSpanID.String()) {
		t.Errorf("span_id не добавлен для несэмплированной трассы: %q", out)
	}
}

func TestTraceHandler_DelegatesEnabled(t *testing.T) {
	handler := logger.NewTraceHandler(
		slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelWarn}),
	)

	if handler.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("Enabled(INFO) должен вернуть false при уровне WARN")
	}
	if !handler.Enabled(context.Background(), slog.LevelWarn) {
		t.Error("Enabled(WARN) должен вернуть true при уровне WARN")
	}
}

func TestTraceHandler_WithAttrsAndWithGroup(t *testing.T) {
	var buf bytes.Buffer

	handler := logger.NewTraceHandler(slog.NewTextHandler(&buf, nil)).
		WithAttrs([]slog.Attr{slog.String("component", "test")}).
		WithGroup("g")

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	record.AddAttrs(slog.String("key", "value"))

	if err := handler.Handle(tracedContext(), record); err != nil {
		t.Fatalf("Handle() вернул ошибку: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "component=test") {
		t.Errorf("атрибуты не добавлены: %q", out)
	}
	if !strings.Contains(out, "g.key=value") {
		t.Errorf("группа не открыта: %q", out)
	}
	if !strings.Contains(out, "trace_id="+testTraceID.String()) {
		t.Errorf("trace_id не добавлен: %q", out)
	}
}

func TestInit_WrapsOutputHandlerWithTrace(t *testing.T) {
	log := logger.Init(slog.LevelInfo, logger.LogFormatJSON)

	if _, ok := log.Handler().(*logger.TraceHandler); !ok {
		t.Errorf("Init() собрал %T, ожидался обработчик с TraceHandler", log.Handler())
	}
}
