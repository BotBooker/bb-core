// Package postgres реализует подключение к PostgreSQL: пул соединений pgx
// и database/sql-фасад (pgx/v5/stdlib), через который отдаётся общий контракт database.DB.
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/botbooker/bb-core/internal/config"
	"github.com/botbooker/bb-core/internal/database"
)

const (
	// defaultMaxConn — размер пула соединений по умолчанию.
	// Используется, если MAX_DB_CONNECT не задан или задан неположительным.
	defaultMaxConn int32 = 10
	// pingTimeout — таймаут проверки доступности PostgreSQL при старте.
	pingTimeout = 5 * time.Second
)

// StatementBuilder — построитель SQL-запросов с плейсхолдерами PostgreSQL ($1, $2, ...).
var StatementBuilder = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

// connector создаёт пул соединений по конфигурации.
// Передаётся аргументом, а не вызывается напрямую, чтобы unit-тесты
// могли подставить фейковый пул без реального сервера PostgreSQL.
type connector func(ctx context.Context, cfg config.DatabaseConfig) (database.SQLDB, error)

// facade — database/sql-фасад поверх пула pgx.
// stdlib.OpenDBFromPool не закрывает пул (это задокументировано в pgx),
// поэтому Close закрывает и фасад, и пул соединений.
type facade struct {
	*sql.DB
	pool *pgxpool.Pool
}

// Close закрывает фасад и пул соединений pgx.
func (f *facade) Close() error {
	err := f.DB.Close()
	f.pool.Close()

	return err
}

// defaultConnector создаёт реальное подключение: пул pgx и database/sql-фасад.
func defaultConnector(ctx context.Context, cfg config.DatabaseConfig) (database.SQLDB, error) {
	poolCfg, err := buildPoolConfig(cfg)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()

		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	sqlPool := stdlib.OpenDBFromPool(pool)
	// Фасад берёт соединения из пула pgx: лимит должен совпадать,
	// иначе database/sql начнёт ждать свободное соединение у пула.
	sqlPool.SetMaxOpenConns(int(poolCfg.MaxConns))

	return &facade{DB: sqlPool, pool: pool}, nil
}

// buildPoolConfig проверяет конфигурацию и собирает настройки пула соединений.
func buildPoolConfig(cfg config.DatabaseConfig) (*pgxpool.Config, error) {
	if strings.TrimSpace(cfg.DSN) == "" {
		return nil, database.ErrEmptyDSN
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}

	// Значение из конфигурации может быть нулевым или отрицательным,
	// поэтому подставляем размер пула по умолчанию.
	poolCfg.MaxConns = defaultMaxConn
	if cfg.MaxDBConnect > 0 {
		poolCfg.MaxConns = cfg.MaxDBConnect
	}

	return poolCfg, nil
}

// New создаёт подключение к PostgreSQL: собирает пул соединений,
// проверяет доступность сервера и создаёт менеджер транзакций.
func New(ctx context.Context, cfg config.DatabaseConfig) (database.DB, error) {
	return newWithConnector(ctx, cfg, defaultConnector)
}

// newWithConnector создаёт подключение с подменяемым соединением.
func newWithConnector(ctx context.Context, cfg config.DatabaseConfig, connect connector) (database.DB, error) {
	pool, err := connect(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// database.New закрывает пул, если менеджер транзакций создать не удалось.
	db, err := database.New(pool)
	if err != nil {
		return nil, fmt.Errorf("create database connection: %w", err)
	}

	slog.Debug("connected to the database", "driver", "pgx")

	return db, nil
}
