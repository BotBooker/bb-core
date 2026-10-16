package logger

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

const (
	// traceIDKey — имя атрибута записи с TraceID активной трассы.
	traceIDKey = "trace_id"
)

// TraceHandler — реализация slog.Handler, которая добавляет к каждой записи
// trace_id активной трассы OpenTelemetry.
//
// TraceID берётся из контекста записи (trace.SpanContextFromContext), поэтому
// контекст до обработчика доносят только контекстные вызовы логгера —
// slog.InfoContext, slog.ErrorContext, slog.WarnContext, slog.DebugContext.
// Записи вне активной трассы (без контекстного вызова или без TraceID) уходят
// без атрибута: пустого trace_id в логах не появляется.
//
// span_id в stdout-логи не пишется: для связки записи с трассой достаточно
// trace_id, а SpanID нужен только внутри самой трассы. Атрибут нужен для
// stdout, где у slog нет отдельного поля с трассой: в OTLP-логи TraceID
// подставляет SDK логов OpenTelemetry из того же контекста
// (см. pkg/observability). Если запись уже содержит trace_id, готовое значение
// не перезаписывается.
type TraceHandler struct {
	handler slog.Handler
}

// NewTraceHandler оборачивает обработчик, добавляя к записям trace_id активной
// трассы из контекста.
func NewTraceHandler(handler slog.Handler) *TraceHandler {
	return &TraceHandler{handler: handler}
}

// Enabled делегирует решение вложенному обработчику.
func (h *TraceHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

// Handle дописывает в запись trace_id активной трассы и передаёт её вложенному
// обработчику.
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

// traceAttrs возвращает атрибуты трассы для записи: trace_id активной трассы из
// контекста. Если TraceID в контексте отсутствует (трасса не инициализирована),
// функция возвращает nil — приложение без адреса OTel Collector пишет логи, как
// раньше, без пустого атрибута. Уже добавленный вручную trace_id не
// перезаписывается.
func traceAttrs(ctx context.Context, record slog.Record) []slog.Attr {
	spanCtx := trace.SpanContextFromContext(ctx)
	if !spanCtx.HasTraceID() {
		return nil
	}

	if hasAttr(record, traceIDKey) {
		return nil
	}

	return []slog.Attr{slog.String(traceIDKey, spanCtx.TraceID().String())}
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
