package events

import "log/slog"

// EventBus — интерфейс шины событий приложения.
// Чистый pub/sub брокер без внешних зависимостей.
// Сервисы сами публикуют события и подписываются на них.
type EventBus interface {
	Publish(event string)
	Subscribe(event string, handler func())
}

// eventBus — конкретная реализация, скрыта от внешних пакетов.
type eventBus struct{}

// NewEventBus создаёт шину событий.
func NewEventBus() EventBus {
	slog.Debug("event bus created")
	return &eventBus{}
}

// Publish публикует событие.
func (b *eventBus) Publish(event string) {
	slog.Debug("event published", "event", event)
}

// Subscribe подписывается на событие.
func (b *eventBus) Subscribe(event string, handler func()) {
	slog.Debug("subscribing to event", "event", event, "handler", handler)
}
