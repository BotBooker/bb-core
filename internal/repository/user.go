package repository

import (
	"context"
	"database/sql"
	"log/slog"
)

// UserDB — интерфейс базы данных, который нужен UserRepo.
//
// Методы совпадают с методами database.DB, но объявлены узко: репозиторию
// нужны только запросы, а менеджер транзакций и закрытие пула — нет.
type UserDB interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// UserRepo — интерфейс репозитория пользователей.
type UserRepo any

// userRepo — конкретная реализация, скрыта от внешних пакетов.
type userRepo struct {
	db UserDB
}

// NewUserRepo создаёт репозиторий пользователей.
func NewUserRepo(db UserDB) UserRepo {
	slog.Debug("user repository created")
	return &userRepo{db: db}
}
