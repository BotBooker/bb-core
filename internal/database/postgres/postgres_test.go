package postgres

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/botbooker/bb-core/internal/config"
	"github.com/botbooker/bb-core/internal/database"
)

// errTest — сентинельная ошибка для проверки проброса ошибок из пула.
var errTest = errors.New("test error")

// unreachableDSN указывает на закрытый порт: пул pgx ленивый,
// поэтому соединение не устанавливается до первого Ping.
const unreachableDSN = "postgres://user:pass@127.0.0.1:1/demo?sslmode=disable"

// fakePool — фейковый пул database/sql: считает вызовы и возвращает заданные ошибки.
type fakePool struct {
	beginErr error
	queryErr error
	execErr  error
	pingErr  error
	closeErr error

	beginCalls    int
	queryCalls    int
	queryRowCalls int
	execCalls     int
	pingCalls     int
	closeCalls    int

	lastQuery string
	lastArgs  []any
}

var _ database.SQLDB = (*fakePool)(nil)

func (f *fakePool) BeginTx(_ context.Context, _ *sql.TxOptions) (*sql.Tx, error) {
	f.beginCalls++

	return nil, f.beginErr
}

func (f *fakePool) QueryContext(_ context.Context, query string, args ...any) (*sql.Rows, error) {
	f.queryCalls++
	f.lastQuery, f.lastArgs = query, args

	return nil, f.queryErr
}

func (f *fakePool) QueryRowContext(_ context.Context, query string, args ...any) *sql.Row {
	f.queryRowCalls++
	f.lastQuery, f.lastArgs = query, args

	return nil
}

func (f *fakePool) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	f.execCalls++
	f.lastQuery, f.lastArgs = query, args

	return nil, f.execErr
}

func (f *fakePool) PingContext(_ context.Context) error {
	f.pingCalls++

	return f.pingErr
}

func (f *fakePool) Close() error {
	f.closeCalls++

	return f.closeErr
}

// newFakeConnector возвращает соединение, отдающее заранее подготовленный пул.
// capturedCfg позволяет проверить, какая конфигурация дошла до соединения.
func newFakeConnector(pool database.SQLDB, err error, capturedCfg **config.DatabaseConfig) connector {
	return func(_ context.Context, cfg config.DatabaseConfig) (database.SQLDB, error) {
		if capturedCfg != nil {
			copied := cfg
			*capturedCfg = &copied
		}

		if err != nil {
			return nil, err
		}

		return pool, nil
	}
}

func TestStatementBuilder(t *testing.T) {
	query, args, err := StatementBuilder.
		Insert("users").
		Columns("name", "email").
		Values("alice", "alice@example.com").
		ToSql()
	if err != nil {
		t.Fatalf("ToSql() unexpected error = %v", err)
	}

	const wantQuery = "INSERT INTO users (name,email) VALUES ($1,$2)"
	if query != wantQuery {
		t.Errorf("query = %q, want %q", query, wantQuery)
	}
	if len(args) != 2 {
		t.Fatalf("args = %v, want 2 arguments", args)
	}
}

func TestBuildPoolConfig(t *testing.T) {
	tests := []struct {
		name            string
		cfg             config.DatabaseConfig
		wantErr         error
		wantErrContains string
		wantMaxConns    int32
	}{
		{
			name:         "url dsn with default pool size",
			cfg:          config.DatabaseConfig{DSN: "postgres://user:pass@localhost:5432/demo"},
			wantMaxConns: defaultMaxConn,
		},
		{
			name:         "keyword value dsn",
			cfg:          config.DatabaseConfig{DSN: "host=localhost port=5432 dbname=demo"},
			wantMaxConns: defaultMaxConn,
		},
		{
			name:         "custom pool size",
			cfg:          config.DatabaseConfig{DSN: "postgres://localhost:5432/demo", MaxDBConnect: 25},
			wantMaxConns: 25,
		},
		{
			name:         "zero pool size falls back to default",
			cfg:          config.DatabaseConfig{DSN: "postgres://localhost:5432/demo", MaxDBConnect: 0},
			wantMaxConns: defaultMaxConn,
		},
		{
			name:         "negative pool size falls back to default",
			cfg:          config.DatabaseConfig{DSN: "postgres://localhost:5432/demo", MaxDBConnect: -5},
			wantMaxConns: defaultMaxConn,
		},
		{
			name:    "empty dsn",
			cfg:     config.DatabaseConfig{},
			wantErr: database.ErrEmptyDSN,
		},
		{
			name:    "blank dsn",
			cfg:     config.DatabaseConfig{DSN: "   "},
			wantErr: database.ErrEmptyDSN,
		},
		{
			name:            "invalid dsn",
			cfg:             config.DatabaseConfig{DSN: "postgres://user:pass@localhost:port/demo"},
			wantErrContains: "parse dsn",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildPoolConfig(tt.cfg)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("buildPoolConfig() error = %v, want %v", err, tt.wantErr)
				}
				if got != nil {
					t.Error("buildPoolConfig() returned non-nil config on error")
				}

				return
			}

			if tt.wantErrContains != "" {
				if err == nil {
					t.Fatalf("buildPoolConfig() error = nil, want error containing %q", tt.wantErrContains)
				}
				if !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Fatalf("buildPoolConfig() error = %v, want error containing %q", err, tt.wantErrContains)
				}

				return
			}

			if err != nil {
				t.Fatalf("buildPoolConfig() unexpected error = %v", err)
			}
			if got.MaxConns != tt.wantMaxConns {
				t.Errorf("MaxConns = %d, want %d", got.MaxConns, tt.wantMaxConns)
			}
		})
	}
}

func TestNewWithConnector(t *testing.T) {
	tests := []struct {
		name            string
		cfg             config.DatabaseConfig
		connectorErr    error
		wantErr         error
		wantErrContains string
	}{
		{
			name: "successful connection",
			cfg:  config.DatabaseConfig{DSN: "postgres://localhost:5432/demo", MaxDBConnect: 3},
		},
		{
			name:         "connector error is wrapped",
			cfg:          config.DatabaseConfig{DSN: "postgres://localhost:5432/demo"},
			connectorErr: errTest,
			wantErr:      errTest,
		},
		{
			name:            "empty dsn is reported by connector",
			cfg:             config.DatabaseConfig{},
			wantErr:         database.ErrEmptyDSN,
			wantErrContains: "database DSN is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Соединение по умолчанию проверяет DSN, поэтому пустой DSN
			// проверяем на реальном соединении, а остальные случаи — на фейковом пуле.
			connect := defaultConnector
			if tt.cfg.DSN != "" {
				connect = newFakeConnector(&fakePool{}, tt.connectorErr, nil)
			}

			got, err := newWithConnector(context.Background(), tt.cfg, connect)

			if tt.wantErr != nil || tt.wantErrContains != "" {
				if err == nil {
					t.Fatal("newWithConnector() error = nil, want error")
				}
				if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
					t.Errorf("newWithConnector() error = %v, want %v", err, tt.wantErr)
				}
				if tt.wantErrContains != "" && !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Errorf("newWithConnector() error = %v, want error containing %q", err, tt.wantErrContains)
				}
				if got != nil {
					t.Error("newWithConnector() returned non-nil DB on error")
				}

				return
			}

			if err != nil {
				t.Fatalf("newWithConnector() unexpected error = %v", err)
			}
			if got == nil {
				t.Fatal("newWithConnector() returned nil DB, want non-nil")
			}
			if got.TrManager() == nil {
				t.Error("TrManager() = nil, want non-nil")
			}
		})
	}
}

func TestDBDelegatesToPool(t *testing.T) {
	pool := &fakePool{
		beginErr: errTest,
		queryErr: errTest,
		execErr:  errTest,
		pingErr:  errTest,
		closeErr: errTest,
	}

	ctx := context.Background()
	cfg := config.DatabaseConfig{DSN: "postgres://localhost:5432/demo"}

	db, err := newWithConnector(ctx, cfg, newFakeConnector(pool, nil, nil))
	if err != nil {
		t.Fatalf("newWithConnector() unexpected error = %v", err)
	}

	if _, err = db.QueryContext(ctx, "select 1"); !errors.Is(err, errTest) {
		t.Errorf("QueryContext() error = %v, want %v", err, errTest)
	}
	if _, err = db.ExecContext(ctx, "select $1", "arg"); !errors.Is(err, errTest) {
		t.Errorf("ExecContext() error = %v, want %v", err, errTest)
	}
	if _, err = db.BeginTx(ctx, nil); !errors.Is(err, errTest) {
		t.Errorf("BeginTx() error = %v, want %v", err, errTest)
	}
	if err = db.PingContext(ctx); !errors.Is(err, errTest) {
		t.Errorf("PingContext() error = %v, want %v", err, errTest)
	}
	if err = db.Close(); !errors.Is(err, errTest) {
		t.Errorf("Close() error = %v, want %v", err, errTest)
	}

	// QueryRowContext ошибку не возвращает, поэтому проверяем сам факт делегирования.
	_ = db.QueryRowContext(ctx, "select $1", "arg")

	if pool.queryCalls != 1 || pool.execCalls != 1 || pool.beginCalls != 1 ||
		pool.pingCalls != 1 || pool.closeCalls != 1 || pool.queryRowCalls != 1 {
		t.Errorf(
			"pool calls = (query %d, exec %d, begin %d, ping %d, close %d, queryRow %d), want one of each",
			pool.queryCalls, pool.execCalls, pool.beginCalls, pool.pingCalls, pool.closeCalls, pool.queryRowCalls,
		)
	}
	if pool.lastQuery != "select $1" {
		t.Errorf("last query = %q, want %q", pool.lastQuery, "select $1")
	}
	if len(pool.lastArgs) != 1 || pool.lastArgs[0] != "arg" {
		t.Errorf("last args = %v, want [arg]", pool.lastArgs)
	}
}

func TestDefaultConnector(t *testing.T) {
	tests := []struct {
		name            string
		cfg             config.DatabaseConfig
		wantErr         error
		wantErrContains string
	}{
		{
			name:    "empty dsn",
			cfg:     config.DatabaseConfig{},
			wantErr: database.ErrEmptyDSN,
		},
		{
			name:            "invalid dsn",
			cfg:             config.DatabaseConfig{DSN: "postgres://user:pass@localhost:port/demo"},
			wantErrContains: "parse dsn",
		},
		{
			name:            "unreachable server",
			cfg:             config.DatabaseConfig{DSN: unreachableDSN},
			wantErrContains: "ping postgres",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := defaultConnector(context.Background(), tt.cfg)

			if err == nil {
				if closeErr := got.Close(); closeErr != nil {
					t.Errorf("Close() error = %v", closeErr)
				}

				t.Fatal("defaultConnector() error = nil, want error")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("defaultConnector() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErrContains != "" && !strings.Contains(err.Error(), tt.wantErrContains) {
				t.Errorf("defaultConnector() error = %v, want error containing %q", err, tt.wantErrContains)
			}
			if got != nil {
				t.Error("defaultConnector() returned non-nil pool on error")
			}
		})
	}
}

func TestFacadeCloseClosesPool(t *testing.T) {
	ctx := context.Background()

	poolCfg, err := buildPoolConfig(config.DatabaseConfig{DSN: unreachableDSN})
	if err != nil {
		t.Fatalf("buildPoolConfig() unexpected error = %v", err)
	}

	// Пул pgx ленивый: без обращения к серверу он создаётся без соединений.
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		t.Fatalf("pgxpool.NewWithConfig() unexpected error = %v", err)
	}

	f := &facade{DB: stdlib.OpenDBFromPool(pool), pool: pool}

	if err = f.Close(); err != nil {
		t.Fatalf("Close() unexpected error = %v", err)
	}

	// stdlib.OpenDBFromPool не закрывает пул, поэтому это делает фасад.
	if err = pool.Ping(ctx); err == nil {
		t.Error("pool.Ping() error = nil, want error after Close")
	}
	if err = f.Close(); err != nil {
		t.Errorf("second Close() error = %v, want nil", err)
	}
}

// TestIntegration проверяет работу с настоящим PostgreSQL.
// Тест включается переменной окружения PG (её задаёт CI), локально пропускается.
func TestIntegration(t *testing.T) {
	dsn := os.Getenv("PG")
	if dsn == "" {
		t.Skip("PG is not set: PostgreSQL integration test skipped")
	}

	ctx := context.Background()

	db, err := New(ctx, config.DatabaseConfig{DSN: dsn, MaxDBConnect: 2})
	if err != nil {
		t.Fatalf("New() unexpected error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("Close() error = %v", closeErr)
		}
	})

	if _, err = db.ExecContext(ctx, "drop table if exists users"); err != nil {
		t.Fatalf("ExecContext() error = %v", err)
	}
	if _, err = db.ExecContext(ctx, "create table users (id serial primary key, name text not null)"); err != nil {
		t.Fatalf("ExecContext() error = %v", err)
	}

	if _, err = db.ExecContext(ctx, "insert into users (name) values ($1)", "alice"); err != nil {
		t.Fatalf("ExecContext() error = %v", err)
	}

	var name string
	if err = db.QueryRowContext(ctx, "select name from users where name = $1", "alice").Scan(&name); err != nil {
		t.Fatalf("QueryRowContext() error = %v", err)
	}
	if name != "alice" {
		t.Errorf("name = %q, want %q", name, "alice")
	}

	// Транзакция с откатом: запись не должна сохраниться.
	err = db.TrManager().Do(ctx, func(ctx context.Context) error {
		tx, ok := database.GetTx(ctx)
		if !ok {
			t.Fatal("GetTx() did not find the transaction in context")
		}

		if _, execErr := tx.ExecContext(ctx, "insert into users (name) values ($1)", "bob"); execErr != nil {
			return execErr
		}

		return errTest
	})
	if !errors.Is(err, errTest) {
		t.Fatalf("TrManager().Do() error = %v, want %v", err, errTest)
	}

	var count int
	if err = db.QueryRowContext(ctx, "select count(*) from users").Scan(&count); err != nil {
		t.Fatalf("QueryRowContext() error = %v", err)
	}
	if count != 1 {
		t.Errorf("users count = %d, want 1 (transaction should be rolled back)", count)
	}
}
