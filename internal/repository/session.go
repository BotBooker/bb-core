package repository

import (
	"context"
	"database/sql"
	"log/slog"
)

// SessionDB — интерфейс базы данных, который нужен SessionRepo.
//
// Методы совпадают с методами database.DB, но объявлены узко: репозиторию
// нужны только запросы и ручные транзакции.
type SessionDB interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// SessionCache — интерфейс кэша, который нужен SessionRepo.
type SessionCache interface {
	Get(key string) (string, error)
	Set(key, value string) error
}

// SessionRepo — интерфейс репозитория сессий.
type SessionRepo any

// sessionRepo — конкретная реализация, скрыта от внешних пакетов.
type sessionRepo struct {
	db    SessionDB
	cache SessionCache
}

// NewSessionRepo создаёт репозиторий сессий.
func NewSessionRepo(db SessionDB, cache SessionCache) SessionRepo {
	slog.Debug("session repository created")
	return &sessionRepo{db: db, cache: cache}
}
