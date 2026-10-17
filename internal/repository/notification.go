package repository

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/botbooker/bb-core/pkg/observability"
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
func NewNotificationRepo(ctx context.Context, db NotificationDB) NotificationRepo {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "NewNotificationRepo")
	defer spanEnd()

	slog.DebugContext(traceCtx, "notification repository created")
	return &notificationRepo{db: db}
}
