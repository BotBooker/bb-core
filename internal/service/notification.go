package service

import (
	"context"
	"log/slog"
)

// NotificationRepository — интерфейс репозитория уведомлений для NotificationService.
type NotificationRepository any

// методы, которые NotificationService использует из репозитория уведомлений

// NotificationUserService — интерфейс сервиса пользователей для NotificationService.
type NotificationUserService any

// методы, которые NotificationService использует из сервиса пользователей

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
	notificationRepo NotificationRepository
	userService      NotificationUserService
	events           NotificationEventSubscriber
}

// NewNotificationService создаёт сервис уведомлений.
func NewNotificationService(
	ctx context.Context,
	notificationRepo NotificationRepository,
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
