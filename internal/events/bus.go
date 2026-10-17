package events

import (
	"context"
	"log/slog"

	"github.com/botbooker/bb-core/pkg/observability"
)

// EventBus — интерфейс шины событий приложения.
// Чистый pub/sub брокер без внешних зависимостей.
// Сервисы сами публикуют события и подписываются на них.
//
// Методы принимают context.Context: через него в записи логов добавляется
// trace_id активной трассы (см. pkg/logger.TraceHandler), поэтому записи
// делаются контекстными вызовами slog.
type EventBus interface {
	Publish(ctx context.Context, event string)
	Subscribe(ctx context.Context, event string, handler func())
}

// eventBus — конкретная реализация, скрыта от внешних пакетов.
type eventBus struct{}

// NewEventBus создаёт шину событий.
func NewEventBus(ctx context.Context) EventBus {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "NewEventBus")
	defer spanEnd()

	slog.DebugContext(traceCtx, "event bus created")
	return &eventBus{}
}

// Publish публикует событие.
func (b *eventBus) Publish(ctx context.Context, event string) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "Publish")
	defer spanEnd()
	slog.DebugContext(traceCtx, "event published", slog.String("event", event))
}

// Subscribe подписывается на событие.
func (b *eventBus) Subscribe(ctx context.Context, event string, handler func()) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "Subscribe")
	defer spanEnd()
	slog.DebugContext(traceCtx, "subscribing to event", slog.String("event", event), "handler", handler)
}
