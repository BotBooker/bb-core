package database

import (
	"errors"
	"log/slog"
)

// DB — интерфейс подключения к базе данных.
type DB interface {
	Query(query string) error
	QueryRow(query string) error
	Exec(query string) error
	BeginTx() error
	BulkInsert(query string, args ...any) error
	Close() error
}

// db — конкретная реализация, скрыта от внешних пакетов.
type db struct {
	dsn string
}

// New создаёт подключение к базе данных.
func New(dsn string) (DB, error) {
	if dsn == "" {
		return nil, errors.New("dsn пустой")
	}

	slog.Debug("подключились к базе данных")
	return &db{dsn: dsn}, nil
}

// Query выполняет запрос на чтение.
func (d *db) Query(query string) error {
	slog.Debug("database.Query", "query", query)
	return nil
}

// QueryRow выполняет запрос, возвращающий одну строку.
func (d *db) QueryRow(query string) error {
	slog.Debug("database.QueryRow", "query", query)
	return nil
}

// Exec выполняет запрос на запись.
func (d *db) Exec(query string) error {
	slog.Debug("database.Exec", "query", query)
	return nil
}

// BeginTx начинает транзакцию.
func (d *db) BeginTx() error {
	return nil
}

// BulkInsert выполняет массовую вставку.
func (d *db) BulkInsert(query string, args ...any) error {
	slog.Debug("database.BulkInsert", "query", query, "args", args)
	return nil
}

// Close закрывает подключение к базе данных.
func (d *db) Close() error {
	slog.Debug("подключение к базе данных закрыто")
	return nil
}
