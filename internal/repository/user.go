package repository

import "log/slog"

// UserDB — интерфейс базы данных, который нужен UserRepo.
type UserDB interface {
	Query(query string) error
	QueryRow(query string) error
	Exec(query string) error
}

// UserRepo — интерфейс репозитория пользователей.
type UserRepo any

// userRepo — конкретная реализация, скрыта от внешних пакетов.
type userRepo struct {
	db UserDB
}

// NewUserRepo создаёт репозиторий пользователей.
func NewUserRepo(db UserDB) UserRepo {
	slog.Debug("репозиторий пользователей создан")
	return &userRepo{db: db}
}
