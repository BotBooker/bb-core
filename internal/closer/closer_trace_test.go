package closer

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"

	"github.com/botbooker/bb-core/pkg/logger"
)

var (
	// traceID и spanID — валидные идентификаторы тестовой трассы shutdown.
	traceID = trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	spanID  = trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8}
)

// TestCloseAllLogsTraceID проверяет, что записи завершения ресурсов несут
// trace_id/span_id активного спана: логи shutdown (см. startShutdownSpan)
// должны быть связаны с трассой, а не теряться вне контекста.
func TestCloseAllLogsTraceID(t *testing.T) {
	original := globalCloser
	t.Cleanup(func() { globalCloser = original })
	globalCloser = &closer{}

	var buffer bytes.Buffer

	previous := slog.Default()
	slog.SetDefault(slog.New(logger.NewTraceHandler(
		slog.NewTextHandler(&buffer, &slog.HandlerOptions{Level: slog.LevelDebug}),
	)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	Add("resource", func(context.Context) error { return nil })

	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), spanCtx)

	if err := CloseAll(ctx); err != nil {
		t.Fatalf("CloseAll() вернул ошибку: %v", err)
	}

	log := buffer.String()
	if !strings.Contains(log, "trace_id="+traceID.String()) {
		t.Errorf("trace_id не добавлен в логи closer: %q", log)
	}
	if !strings.Contains(log, "span_id="+spanID.String()) {
		t.Errorf("span_id не добавлен в логи closer: %q", log)
	}
}
