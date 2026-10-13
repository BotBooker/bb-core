package logger

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

const (
	// traceIDKey — имя атрибута записи с TraceID активной трассы.
	traceIDKey = "trace_id"
	// spanIDKey — имя атрибута записи с SpanID текущего спана.
	spanIDKey = "span_id"
)

// TraceHandler — реализация slog.Handler, которая добавляет к каждой записи
// trace_id и span_id активного спана OpenTelemetry.
//
// Спан берётся из контекста записи (trace.SpanContextFromContext), поэтому
// контекст до обработчика доносят только контекстные вызовы логгера —
// slog.InfoContext, slog.ErrorContext, slog.WarnContext, slog.DebugContext.
// Записи вне активной трассы (без контекстного вызова или без спана) уходят
// без этих атрибутов: пустых trace_id и span_id в логах не появляется.
//
// Атрибуты нужны для stdout, где у slog нет отдельного поля с трассой: в
// OTLP-логи TraceID и SpanID подставляет SDK логов OpenTelemetry из того же
// контекста (см. pkg/observability). Если запись уже содержит trace_id или
// span_id, готовое значение не перезаписывается.
type TraceHandler struct {
	handler slog.Handler
}

// NewTraceHandler оборачивает обработчик, добавляя к записям trace_id и span_id
// активного спана из контекста.
func NewTraceHandler(handler slog.Handler) *TraceHandler {
	return &TraceHandler{handler: handler}
}

// Enabled делегирует решение вложенному обработчику.
func (h *TraceHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

// Handle дописывает в запись trace_id и span_id активного спана и передаёт её
// вложенному обработчику.
func (h *TraceHandler) Handle(ctx context.Context, record slog.Record) error {
	record.AddAttrs(traceAttrs(ctx, record)...)

	return h.handler.Handle(ctx, record)
}

// WithAttrs возвращает обработчик, в котором атрибуты добавлены вложенному
// обработчику.
func (h *TraceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &TraceHandler{handler: h.handler.WithAttrs(attrs)}
}

// WithGroup возвращает обработчик, в котором открыта группа name у вложенного
// обработчика.
func (h *TraceHandler) WithGroup(name string) slog.Handler {
	return &TraceHandler{handler: h.handler.WithGroup(name)}
}

// traceAttrs возвращает атрибуты трассы для записи: trace_id и span_id активного
// спана из контекста. Вне активной трассы спан невалиден, и функция возвращает
// nil — приложение без адреса OTel Collector пишет логи, как раньше, без пустых
// атрибутов. Уже добавленные вручную trace_id и span_id не перезаписываются.
func traceAttrs(ctx context.Context, record slog.Record) []slog.Attr {
	spanCtx := trace.SpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		return nil
	}

	attrs := make([]slog.Attr, 0, 2)

	if !hasAttr(record, traceIDKey) {
		attrs = append(attrs, slog.String(traceIDKey, spanCtx.TraceID().String()))
	}

	if !hasAttr(record, spanIDKey) {
		attrs = append(attrs, slog.String(spanIDKey, spanCtx.SpanID().String()))
	}

	return attrs
}

// hasAttr сообщает, есть ли у записи атрибут с ключом key.
func hasAttr(record slog.Record, key string) bool {
	found := false

	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == key {
			found = true

			return false
		}

		return true
	})

	return found
}
