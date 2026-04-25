package service

import "log/slog"

// NotificationRepository — интерфейс репозитория уведомлений для NotificationService.
type NotificationRepository any

// методы, которые NotificationService использует из репозитория уведомлений

// NotificationUserService — интерфейс сервиса пользователей для NotificationService.
type NotificationUserService any

// методы, которые NotificationService использует из сервиса пользователей

// NotificationEventSubscriber — интерфейс для подписки на события.
type NotificationEventSubscriber interface {
	Subscribe(event string, handler func())
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
	notificationRepo NotificationRepository,
	userService NotificationUserService,
	events NotificationEventSubscriber,
) NotificationService {
	slog.Debug("сервис уведомлений создан")
	return &notificationService{
		notificationRepo: notificationRepo,
		userService:      userService,
		events:           events,
	}
}
