package logger

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

var (
	// initHandlerTraceID и initHandlerSpanID — идентификаторы тестового спана
	// для проверок обработчика вывода.
	initHandlerTraceID = trace.TraceID{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	initHandlerSpanID  = trace.SpanID{8, 7, 6, 5, 4, 3, 2, 1}
)

// initHandlerTracedContext — контекст с активным спаном для вызовов логгера.
func initHandlerTracedContext() context.Context {
	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    initHandlerTraceID,
		SpanID:     initHandlerSpanID,
		TraceFlags: trace.FlagsSampled,
	})

	return trace.ContextWithSpanContext(context.Background(), spanCtx)
}

func TestInitHandler_Plain_AddsTraceIDs(t *testing.T) {
	var buf bytes.Buffer

	log := slog.New(initHandler(&buf, slog.LevelDebug, LogFormatText))
	log.InfoContext(initHandlerTracedContext(), "plain with trace")

	out := buf.String()
	if !strings.Contains(out, "trace_id="+initHandlerTraceID.String()) {
		t.Errorf("trace_id не попал в plain-вывод: %q", out)
	}
	if !strings.Contains(out, "span_id="+initHandlerSpanID.String()) {
		t.Errorf("span_id не попал в plain-вывод: %q", out)
	}
}

func TestInitHandler_JSON_AddsTraceIDs(t *testing.T) {
	var buf bytes.Buffer

	log := slog.New(initHandler(&buf, slog.LevelDebug, LogFormatJSON))
	log.InfoContext(initHandlerTracedContext(), "json with trace")

	out := buf.String()
	if !strings.Contains(out, `"trace_id":"`+initHandlerTraceID.String()+`"`) {
		t.Errorf("trace_id не попал в json-вывод: %q", out)
	}
	if !strings.Contains(out, `"span_id":"`+initHandlerSpanID.String()+`"`) {
		t.Errorf("span_id не попал в json-вывод: %q", out)
	}
}

func TestInitHandler_RespectsLevel(t *testing.T) {
	var buf bytes.Buffer

	log := slog.New(initHandler(&buf, slog.LevelWarn, LogFormatText))
	log.InfoContext(initHandlerTracedContext(), "info must be dropped")

	if buf.Len() != 0 {
		t.Errorf("запись ниже уровня WARN не должна попадать в вывод: %q", buf.String())
	}

	log.WarnContext(initHandlerTracedContext(), "warn must pass")

	if !strings.Contains(buf.String(), "warn must pass") {
		t.Errorf("запись уровня WARN должна попадать в вывод: %q", buf.String())
	}
}
