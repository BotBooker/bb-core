package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/botbooker/bb-core/internal/config"
	"github.com/botbooker/bb-core/internal/database"
)

// errTest — сентинельная ошибка для проверки проброса ошибок из пула.
var errTest = errors.New("test error")

// memoryDSN — база данных SQLite в памяти: живой драйвер без работы с файлами.
const memoryDSN = ":memory:"

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

// newFakeFactory возвращает фабрику пула, которая отдаёт заранее подготовленный пул.
// capturedCfg позволяет проверить, какая конфигурация дошла до фабрики.
func newFakeFactory(pool database.SQLDB, err error, capturedCfg **config.DatabaseConfig) poolFactory {
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

	const wantQuery = "INSERT INTO users (name,email) VALUES (?,?)"
	if query != wantQuery {
		t.Errorf("query = %q, want %q", query, wantQuery)
	}
	if len(args) != 2 {
		t.Fatalf("args = %v, want 2 arguments", args)
	}
	if args[0] != "alice" || args[1] != "alice@example.com" {
		t.Errorf("args = %v, want [alice alice@example.com]", args)
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name            string
		dsn             string
		wantErr         error
		wantErrContains string
	}{
		{
			name: "in-memory database",
			dsn:  memoryDSN,
		},
		{
			name: "file uri",
			dsn:  "file:" + filepath.Join(t.TempDir(), "demo.db"),
		},
		{
			name: "dsn with sqlite scheme",
			dsn:  "sqlite:" + filepath.Join(t.TempDir(), "demo.db"),
		},
		{
			name: "dsn with sqlite3 scheme and file uri",
			dsn:  "sqlite3:file:" + filepath.Join(t.TempDir(), "demo.db"),
		},
		{
			name:    "dsn with scheme only",
			dsn:     "sqlite:",
			wantErr: database.ErrEmptyDSN,
		},
		{
			name:    "empty DSN",
			dsn:     "",
			wantErr: database.ErrEmptyDSN,
		},
		{
			name:    "blank DSN",
			dsn:     "   ",
			wantErr: database.ErrEmptyDSN,
		},
		{
			name:            "file in missing directory",
			dsn:             filepath.Join(t.TempDir(), "no-such-dir", "demo.db"),
			wantErrContains: "ping sqlite",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := New(context.Background(), config.DatabaseConfig{DSN: tt.dsn})

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("New() error = %v, want %v", err, tt.wantErr)
				}
				if got != nil {
					t.Error("New() returned non-nil DB on error")
				}

				return
			}

			if tt.wantErrContains != "" {
				if err == nil {
					t.Fatalf("New() error = nil, want error containing %q", tt.wantErrContains)
				}
				if !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Fatalf("New() error = %v, want error containing %q", err, tt.wantErrContains)
				}
				if got != nil {
					t.Error("New() returned non-nil DB on error")
				}

				return
			}

			if err != nil {
				t.Fatalf("New() unexpected error = %v", err)
			}
			if got == nil {
				t.Fatal("New() returned nil DB, want non-nil")
			}
			if got.TrManager() == nil {
				t.Error("TrManager() = nil, want non-nil")
			}
			t.Cleanup(func() {
				if closeErr := got.Close(); closeErr != nil {
					t.Errorf("Close() error = %v", closeErr)
				}
			})

			// Подключение действительно работает: создаём таблицу и пишем в неё.
			if _, err = got.ExecContext(context.Background(), "create table t (id integer primary key, name text)"); err != nil {
				t.Fatalf("ExecContext() error = %v", err)
			}
		})
	}
}

func TestNewTransactionManager(t *testing.T) {
	ctx := context.Background()

	db, err := New(ctx, config.DatabaseConfig{DSN: memoryDSN})
	if err != nil {
		t.Fatalf("New() unexpected error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("Close() error = %v", closeErr)
		}
	})

	if _, err = db.ExecContext(ctx, "create table users (id integer primary key, name text)"); err != nil {
		t.Fatalf("ExecContext() error = %v", err)
	}

	// Менеджер транзакций использует общий драйвер database/sql: репозиторий
	// получает транзакцию через database.GetTx.
	err = db.TrManager().Do(ctx, func(ctx context.Context) error {
		tx, ok := database.GetTx(ctx)
		if !ok {
			t.Fatal("GetTx() did not find the transaction in context")
		}

		_, execErr := tx.ExecContext(ctx, "insert into users (name) values (?)", "alice")

		return execErr
	})
	if err != nil {
		t.Fatalf("TrManager().Do() unexpected error = %v", err)
	}

	var count int
	if err = db.QueryRowContext(ctx, "select count(*) from users").Scan(&count); err != nil {
		t.Fatalf("QueryRowContext() error = %v", err)
	}
	if count != 1 {
		t.Errorf("users count = %d, want 1 (transaction should be committed)", count)
	}
}

func TestNewPersistsDataInFile(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "demo.db")

	db, err := New(ctx, config.DatabaseConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("New() unexpected error = %v", err)
	}

	if _, err = db.ExecContext(ctx, "create table users (id integer primary key, name text)"); err != nil {
		t.Fatalf("ExecContext() error = %v", err)
	}
	if _, err = db.ExecContext(ctx, "insert into users (name) values (?)", "alice"); err != nil {
		t.Fatalf("ExecContext() error = %v", err)
	}
	if err = db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Данные должны сохраниться между подключениями: база файловая, а не в памяти.
	reopened, err := New(ctx, config.DatabaseConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("New() unexpected error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := reopened.Close(); closeErr != nil {
			t.Errorf("Close() error = %v", closeErr)
		}
	})

	var name string
	if err = reopened.QueryRowContext(ctx, "select name from users where id = ?", 1).Scan(&name); err != nil {
		t.Fatalf("QueryRowContext() error = %v", err)
	}
	if name != "alice" {
		t.Errorf("name = %q, want %q", name, "alice")
	}
}

func TestNewWithFactory(t *testing.T) {
	tests := []struct {
		name            string
		cfg             config.DatabaseConfig
		factoryErr      error
		pingErr         error
		wantErr         error
		wantErrContains string
		wantFactoryCall bool
		wantPoolClosed  bool
	}{
		{
			name:            "successful connection",
			cfg:             config.DatabaseConfig{DSN: memoryDSN, MaxDBConnect: 3},
			wantFactoryCall: true,
		},
		{
			name:    "empty DSN stops before creating pool",
			cfg:     config.DatabaseConfig{},
			wantErr: database.ErrEmptyDSN,
		},
		{
			name:            "pool creation error",
			cfg:             config.DatabaseConfig{DSN: memoryDSN},
			factoryErr:      errTest,
			wantErr:         errTest,
			wantErrContains: "create connection pool",
			wantFactoryCall: true,
		},
		{
			name:            "ping error closes pool",
			cfg:             config.DatabaseConfig{DSN: memoryDSN},
			pingErr:         errTest,
			wantErr:         errTest,
			wantErrContains: "ping sqlite",
			wantFactoryCall: true,
			wantPoolClosed:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := &fakePool{pingErr: tt.pingErr}

			var captured *config.DatabaseConfig
			factory := newFakeFactory(pool, tt.factoryErr, &captured)

			got, err := newWithFactory(context.Background(), tt.cfg, factory)

			if (captured != nil) != tt.wantFactoryCall {
				t.Errorf("pool factory called = %v, want %v", captured != nil, tt.wantFactoryCall)
			}

			if tt.wantErr != nil || tt.wantErrContains != "" {
				if err == nil {
					t.Fatal("newWithFactory() error = nil, want error")
				}
				if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
					t.Errorf("newWithFactory() error = %v, want %v", err, tt.wantErr)
				}
				if tt.wantErrContains != "" && !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Errorf("newWithFactory() error = %v, want error containing %q", err, tt.wantErrContains)
				}
				if got != nil {
					t.Error("newWithFactory() returned non-nil DB on error")
				}
				if tt.wantFactoryCall && (pool.closeCalls > 0) != tt.wantPoolClosed {
					t.Errorf("pool Close() calls = %d, want closed = %v", pool.closeCalls, tt.wantPoolClosed)
				}

				return
			}

			if err != nil {
				t.Fatalf("newWithFactory() unexpected error = %v", err)
			}
			if got == nil {
				t.Fatal("newWithFactory() returned nil DB, want non-nil")
			}
			if got.TrManager() == nil {
				t.Error("TrManager() = nil, want non-nil")
			}
			if pool.pingCalls != 1 {
				t.Errorf("PingContext() calls = %d, want 1", pool.pingCalls)
			}
			if pool.closeCalls != 0 {
				t.Errorf("Close() calls = %d, want 0", pool.closeCalls)
			}
		})
	}
}

func TestDefaultPoolFactory(t *testing.T) {
	tests := []struct {
		name         string
		cfg          config.DatabaseConfig
		wantMaxConns int
	}{
		{
			name:         "default pool size",
			cfg:          config.DatabaseConfig{DSN: memoryDSN},
			wantMaxConns: defaultMaxOpenConns,
		},
		{
			name:         "custom pool size",
			cfg:          config.DatabaseConfig{DSN: memoryDSN, MaxDBConnect: 4},
			wantMaxConns: 4,
		},
		{
			name:         "negative pool size falls back to default",
			cfg:          config.DatabaseConfig{DSN: memoryDSN, MaxDBConnect: -1},
			wantMaxConns: defaultMaxOpenConns,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := defaultPoolFactory(context.Background(), tt.cfg)
			if err != nil {
				t.Fatalf("defaultPoolFactory() unexpected error = %v", err)
			}
			t.Cleanup(func() {
				if closeErr := got.Close(); closeErr != nil {
					t.Errorf("Close() error = %v", closeErr)
				}
			})

			// Настройки пула доступны только у реального *sql.DB.
			pool, ok := got.(*sql.DB)
			if !ok {
				t.Fatalf("defaultPoolFactory() returned %T, want *sql.DB", got)
			}
			if maxConns := pool.Stats().MaxOpenConnections; maxConns != tt.wantMaxConns {
				t.Errorf("MaxOpenConnections = %d, want %d", maxConns, tt.wantMaxConns)
			}
		})
	}
}

func TestNormalizeDSN(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want string
	}{
		{
			name: "plain file path",
			dsn:  "/tmp/database.db",
			want: "/tmp/database.db",
		},
		{
			name: "in-memory database",
			dsn:  memoryDSN,
			want: memoryDSN,
		},
		{
			name: "file uri",
			dsn:  "file:/tmp/database.db",
			want: "file:/tmp/database.db",
		},
		{
			name: "sqlite scheme",
			dsn:  "sqlite:/tmp/database.db",
			want: "/tmp/database.db",
		},
		{
			name: "sqlite scheme with three slashes",
			dsn:  "sqlite:///tmp/database.db",
			want: "/tmp/database.db",
		},
		{
			name: "sqlite3 scheme",
			dsn:  "sqlite3:/tmp/database.db",
			want: "/tmp/database.db",
		},
		{
			name: "sqlite scheme with file uri",
			dsn:  "sqlite:file:/tmp/database.db",
			want: "file:/tmp/database.db",
		},
		{
			name: "sqlite scheme with in-memory database",
			dsn:  "sqlite:" + memoryDSN,
			want: memoryDSN,
		},
		{
			name: "surrounding spaces",
			dsn:  "  sqlite:/tmp/database.db  ",
			want: "/tmp/database.db",
		},
		{
			name: "scheme only",
			dsn:  "sqlite:",
			want: "",
		},
		{
			name: "empty dsn",
			dsn:  "",
			want: "",
		},
		{
			name: "foreign scheme is kept",
			dsn:  "postgres://localhost:5432/demo",
			want: "postgres://localhost:5432/demo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeDSN(tt.dsn); got != tt.want {
				t.Errorf("normalizeDSN(%q) = %q, want %q", tt.dsn, got, tt.want)
			}
		})
	}
}
