package logger

import (
	"context"
	"errors"
	"log/slog"
)

// MultiHandler — реализация slog.Handler, которая передаёт каждую запись сразу
// нескольким обработчикам. Используется, чтобы писать логи одновременно в
// stdout и в OpenTelemetry, не теряя привычный вывод.
type MultiHandler struct {
	handlers []slog.Handler
}

// NewMultiHandler создаёт MultiHandler поверх переданных обработчиков.
// nil-обработчики игнорируются.
func NewMultiHandler(handlers ...slog.Handler) *MultiHandler {
	multi := &MultiHandler{}

	for _, handler := range handlers {
		if handler != nil {
			multi.handlers = append(multi.handlers, handler)
		}
	}

	return multi
}

// Enabled сообщает true, если хотя бы один обработчик готов принять запись
// уровня level.
func (h *MultiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}

	return false
}

// Handle по очереди передаёт запись всем обработчикам. Каждому достаётся своя
// копия записи: обработчики вправе её изменять (например, добавлять атрибуты).
// Обработчики с более высоким порогом уровня запись пропускают. Ошибки
// обработчиков объединяются.
func (h *MultiHandler) Handle(ctx context.Context, record slog.Record) error {
	var err error

	for _, handler := range h.handlers {
		if !handler.Enabled(ctx, record.Level) {
			continue
		}

		err = errors.Join(err, handler.Handle(ctx, record.Clone()))
	}

	return err
}

// WithAttrs возвращает MultiHandler, в котором атрибуты добавлены каждому
// обработчику.
func (h *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		handlers[i] = handler.WithAttrs(attrs)
	}

	return &MultiHandler{handlers: handlers}
}

// WithGroup возвращает MultiHandler, в котором открыта группа name у каждого
// обработчика.
func (h *MultiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		handlers[i] = handler.WithGroup(name)
	}

	return &MultiHandler{handlers: handlers}
}

// levelHandler ограничивает обработчик минимальным уровнем: записи ниже него не
// проходят дальше. Нужен для обработчиков, у которых своей настройки уровня нет
// (например, мост в OpenTelemetry принимает все уровни).
type levelHandler struct {
	handler slog.Handler
	level   slog.Level
}

// withLevel оборачивает обработчик ограничением по минимальному уровню level.
func withLevel(handler slog.Handler, level slog.Level) slog.Handler {
	return levelHandler{handler: handler, level: level}
}

func (h levelHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.level && h.handler.Enabled(ctx, level)
}

func (h levelHandler) Handle(ctx context.Context, record slog.Record) error {
	if record.Level < h.level {
		return nil
	}

	return h.handler.Handle(ctx, record)
}

func (h levelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return levelHandler{handler: h.handler.WithAttrs(attrs), level: h.level}
}

func (h levelHandler) WithGroup(name string) slog.Handler {
	return levelHandler{handler: h.handler.WithGroup(name), level: h.level}
}
