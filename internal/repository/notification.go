package repository

import (
	"context"
	"database/sql"
	"log/slog"
)

// NotificationDB — интерфейс базы данных, который нужен NotificationRepo.
//
// Методы совпадают с методами database.DB, но объявлены узко: репозиторию
// нужны только запросы. Массовая вставка выполняется через ExecContext
// одним многострочным INSERT (или через транзакцию — TrManager).
type NotificationDB interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// NotificationRepo — интерфейс репозитория уведомлений.
type NotificationRepo any

// notificationRepo — конкретная реализация, скрыта от внешних пакетов.
type notificationRepo struct {
	db NotificationDB
}

// NewNotificationRepo создаёт репозиторий уведомлений.
func NewNotificationRepo(db NotificationDB) NotificationRepo {
	slog.Debug("notification repository created")
	return &notificationRepo{db: db}
}
