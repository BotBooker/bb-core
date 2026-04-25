package repository

import "log/slog"

// NotificationDB — интерфейс базы данных, который нужен NotificationRepo.
type NotificationDB interface {
	Query(query string) error
	Exec(query string) error
	BulkInsert(query string, args ...any) error
}

// NotificationRepo — интерфейс репозитория уведомлений.
type NotificationRepo any

// notificationRepo — конкретная реализация, скрыта от внешних пакетов.
type notificationRepo struct {
	db NotificationDB
}

// NewNotificationRepo создаёт репозиторий уведомлений.
func NewNotificationRepo(db NotificationDB) NotificationRepo {
	slog.Debug("репозиторий уведомлений создан")
	return &notificationRepo{db: db}
}
