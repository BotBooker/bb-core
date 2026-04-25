package repository

import "log/slog"

// SessionDB — интерфейс базы данных, который нужен SessionRepo.
type SessionDB interface {
	Query(query string) error
	Exec(query string) error
	BeginTx() error
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
	slog.Debug("репозиторий сессий создан")
	return &sessionRepo{db: db, cache: cache}
}
