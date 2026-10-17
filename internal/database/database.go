// Package database описывает единый контракт подключения к базе данных,
// который реализуют драйверы internal/database/postgres и internal/database/sqlite,
// а также добавляет менеджер транзакций поверх пула database/sql.
package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	trm "github.com/avito-tech/go-transaction-manager/trm/v2"
	trmmanager "github.com/avito-tech/go-transaction-manager/trm/v2/manager"
)

// Диалекты баз данных, которые поддерживает приложение.
// Диалект не задаётся конфигурацией отдельным полем: он определяется по строке
// подключения (DialectFromDSN), поэтому не бывает конфликта «DSN sqlite, а драйвер pgx».
const (
	// DialectPostgres — PostgreSQL.
	DialectPostgres = "postgresql"
	// DialectSQLite — SQLite.
	DialectSQLite = "sqlite3"
)

// ErrEmptyDSN возвращается, если в конфигурации не задан DSN.
var ErrEmptyDSN = errors.New("database DSN is empty")

// ErrUnsupportedDialect возвращается, если схема DSN указывает на неподдерживаемую базу.
var ErrUnsupportedDialect = errors.New("unsupported database dialect")

// ErrDialectNotDetected возвращается, если по DSN без схемы не удалось понять диалект.
var ErrDialectNotDetected = errors.New("cannot detect database dialect from DSN")

// DialectFromDSN определяет диалект по строке подключения.
//
// Признаки по порядку:
//   - схема postgres://, postgresql:// → PostgreSQL;
//   - схема sqlite:, sqlite3:, file: → SQLite;
//   - DSN без схемы в формате pgx ("host=localhost dbname=demo") → PostgreSQL;
//   - DSN без схемы, похожий на путь к файлу (":memory:", "/tmp/database.db", "demo.sqlite") → SQLite.
//
// Неизвестная схема (например, "mysql://...") и нераспознанная строка без схемы
// дают ошибку: лучше упасть на старте, чем открыть не ту базу.
func DialectFromDSN(dsn string) (string, error) {
	trimmed := strings.TrimSpace(dsn)
	if trimmed == "" {
		return "", ErrEmptyDSN
	}

	scheme := dsnScheme(trimmed)

	switch strings.ToLower(scheme) {
	case DialectPostgres, "postgres":
		return DialectPostgres, nil
	case DialectSQLite, "sqlite":
		return DialectSQLite, nil
	case "file":
		// file: — URI-форма SQLite (file:/tmp/database.db).
		return DialectSQLite, nil
	}

	if scheme != "" {
		return "", fmt.Errorf("%w: scheme %q", ErrUnsupportedDialect, scheme)
	}

	// DSN без схемы: pgx понимает keyword/value, SQLite — путь к файлу.
	switch {
	case strings.Contains(trimmed, "="):
		return DialectPostgres, nil
	case isSQLitePath(trimmed):
		return DialectSQLite, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrDialectNotDetected, dsn)
	}
}

// dsnScheme возвращает схему DSN, если она есть: "postgres" из "postgres://...",
// "sqlite" из "sqlite:/path". Для строк без схемы (":memory:", "host=localhost")
// возвращает пустую строку.
func dsnScheme(dsn string) string {
	end := strings.Index(dsn, "://")
	if end < 0 {
		end = strings.Index(dsn, ":")
	}

	// Схема короче двух символов — это не схема: "C:\path" — буква диска,
	// ":memory:" — база SQLite в памяти.
	if end < 2 || !isASCIILetter(dsn[0]) {
		return ""
	}

	scheme := dsn[:end]
	for i := 1; i < len(scheme); i++ {
		if !isSchemeChar(scheme[i]) {
			return ""
		}
	}

	return scheme
}

// isSQLitePath сообщает, что DSN без схемы выглядит как путь к файлу SQLite.
func isSQLitePath(dsn string) bool {
	if strings.HasPrefix(dsn, ":memory:") {
		return true
	}

	if strings.ContainsAny(dsn, `/\`) {
		return true
	}

	lower := strings.ToLower(dsn)
	for _, ext := range []string{".db", ".db3", ".sqlite", ".sqlite3"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}

	return false
}

// isASCIILetter сообщает, что байт — латинская буква.
func isASCIILetter(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

// isSchemeChar сообщает, что байт допустим в схеме URL (RFC 3986).
func isSchemeChar(b byte) bool {
	return isASCIILetter(b) || b >= '0' && b <= '9' || b == '+' || b == '-' || b == '.'
}

// SQLDB — интерфейс пула соединений database/sql, необходимый для работы подключения.
// Его реализует *sql.DB: так и pgx (через stdlib), и sqlite отдают один и тот же тип.
type SQLDB interface {
	// BeginTx начинает транзакцию с заданными опциями.
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
	// QueryContext выполняет запрос и возвращает строки результата.
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	// QueryRowContext выполняет запрос, возвращающий одну строку.
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	// ExecContext выполняет запрос, не возвращающий строк.
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	// PingContext проверяет доступность базы данных.
	PingContext(ctx context.Context) error
	// Close закрывает пул соединений.
	Close() error
}

// DB — интерфейс подключения к базе данных: пул соединений и менеджер транзакций.
// Это единственная зависимость, которую репозитории получают от слоя доступа к данным.
type DB interface {
	SQLDB
	// TrManager возвращает менеджер транзакций.
	TrManager() trm.Manager
}

// Проверяем на этапе компиляции, что реальный пул database/sql удовлетворяет контракту.
var _ SQLDB = (*sql.DB)(nil)

// db — конкретная реализация DB, скрыта от внешних пакетов.
type db struct {
	pool      SQLDB
	trManager trm.Manager
}

// New создаёт подключение поверх готового пула database/sql,
// добавляя к нему менеджер транзакций.
//
// ctx нужен для логирования готовности подключения: trace_id активной трассы
// добавляет обработчик логов (см. pkg/logger.TraceHandler), поэтому запись
// делается контекстным вызовом.
func New(ctx context.Context, pool SQLDB) (DB, error) {
	if pool == nil {
		return nil, errors.New("database pool is nil")
	}

	trManager, err := trmmanager.New(TxFactory(pool))
	if err != nil {
		_ = pool.Close()

		return nil, fmt.Errorf("create transaction manager: %w", err)
	}

	slog.DebugContext(ctx, "database connection is ready")

	return &db{pool: pool, trManager: trManager}, nil
}

// StdDB возвращает пул database/sql, лежащий под контрактом.
//
// Нужен инструментам, которые работают только с *sql.DB (например, миграциям).
// Для фейковых пулов в тестах возвращает nil: реального database/sql под ними нет.
func (d *db) StdDB() *sql.DB {
	switch pool := d.pool.(type) {
	case *sql.DB:
		return pool
	case interface{ StdDB() *sql.DB }:
		return pool.StdDB()
	default:
		return nil
	}
}

// TrManager возвращает менеджер транзакций.
func (d *db) TrManager() trm.Manager {
	return d.trManager
}

// BeginTx начинает транзакцию с заданными опциями.
func (d *db) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return d.pool.BeginTx(ctx, opts)
}

// QueryContext выполняет запрос и возвращает строки результата.
func (d *db) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return d.pool.QueryContext(ctx, query, args...)
}

// QueryRowContext выполняет запрос, возвращающий одну строку.
func (d *db) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return d.pool.QueryRowContext(ctx, query, args...)
}

// ExecContext выполняет запрос, не возвращающий строк.
func (d *db) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return d.pool.ExecContext(ctx, query, args...)
}

// PingContext проверяет доступность базы данных.
func (d *db) PingContext(ctx context.Context) error {
	return d.pool.PingContext(ctx)
}

// Close закрывает пул соединений.
//
// Сигнатура без контекста задана контрактом SQLDB: его реализует *sql.DB,
// у которого Close() error, поэтому изменить сигнатуру нельзя. Запись о
// закрытии с trace_id активной трассы делает вызывающий, у которого есть
// контекст, — регистрация ресурса в closer (см. internal/servers/di.go).
func (d *db) Close() error {
	return d.pool.Close()
}
