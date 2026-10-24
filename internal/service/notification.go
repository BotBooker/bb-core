package service

import (
	"context"
	"log/slog"

	"github.com/botbooker/bb-core/internal/domain"
)

// NotificationUserService — действие над пользователями, которое в будущем
// понадобится уведомлениям. Пока заглушка: порт объявлен в домене, чтобы
// сервис не зависел от инфраструктуры.
type NotificationUserService any

// NotificationEventSubscriber — интерфейс для подписки на события.
// Метод принимает контекст: через него в запись лога добавляется trace_id
// активной трассы (см. pkg/logger.TraceHandler).
type NotificationEventSubscriber interface {
	Subscribe(ctx context.Context, event string, handler func())
}

// NotificationService — интерфейс сервиса уведомлений.
type NotificationService any

// notificationService — конкретная реализация, скрыта от внешних пакетов.
type notificationService struct {
	notificationRepo domain.NotificationRepository
	userService      NotificationUserService
	events           NotificationEventSubscriber
}

// NewNotificationService создаёт сервис уведомлений.
func NewNotificationService(
	ctx context.Context,
	notificationRepo domain.NotificationRepository,
	userService NotificationUserService,
	events NotificationEventSubscriber,
) NotificationService {
	slog.DebugContext(ctx, "notification service created")
	return &notificationService{
		notificationRepo: notificationRepo,
		userService:      userService,
		events:           events,
	}
}
