package database

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	trm "github.com/avito-tech/go-transaction-manager/trm/v2"

	// Регистрирует драйвер "sqlite" для тестового пула в памяти.
	_ "modernc.org/sqlite"
)

// errTest — сентинельная ошибка для проверки проброса ошибок из пула.
var errTest = errors.New("test error")

// newTestPool возвращает реальный пул database/sql поверх SQLite в памяти.
// Контракт описан методами database/sql, поэтому конкретный драйвер тестам не важен:
// реальный PostgreSQL проверяется интеграционным тестом в пакете postgres.
func newTestPool(t *testing.T) *sql.DB {
	t.Helper()

	pool, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() unexpected error = %v", err)
	}

	// База в памяти живёт ровно столько, сколько живёт её соединение.
	pool.SetMaxOpenConns(1)
	pool.SetMaxIdleConns(1)

	t.Cleanup(func() {
		if closeErr := pool.Close(); closeErr != nil {
			t.Errorf("pool.Close() error = %v", closeErr)
		}
	})

	return pool
}

// newTestDB возвращает подключение поверх тестового пула с созданной таблицей users.
func newTestDB(t *testing.T) DB {
	t.Helper()

	database, err := New(newTestPool(t))
	if err != nil {
		t.Fatalf("New() unexpected error = %v", err)
	}

	if _, err = database.ExecContext(context.Background(), "create table users (id integer primary key, name text)"); err != nil {
		t.Fatalf("ExecContext() unexpected error = %v", err)
	}

	return database
}

// fakePool — фейковый пул для проверки веток ошибок, недостижимых на реальной базе.
type fakePool struct {
	beginErr error

	beginCalls int
	closeCalls int
}

var _ SQLDB = (*fakePool)(nil)

func (f *fakePool) BeginTx(_ context.Context, _ *sql.TxOptions) (*sql.Tx, error) {
	f.beginCalls++

	return nil, f.beginErr
}

func (f *fakePool) QueryContext(_ context.Context, _ string, _ ...any) (*sql.Rows, error) {
	return nil, nil
}

func (f *fakePool) QueryRowContext(_ context.Context, _ string, _ ...any) *sql.Row {
	return nil
}

func (f *fakePool) ExecContext(_ context.Context, _ string, _ ...any) (sql.Result, error) {
	return nil, nil
}

func (f *fakePool) PingContext(_ context.Context) error {
	return nil
}

func (f *fakePool) Close() error {
	f.closeCalls++

	return nil
}

func TestNew(t *testing.T) {
	t.Run("nil pool", func(t *testing.T) {
		got, err := New(nil)

		if err == nil {
			t.Fatal("New() error = nil, want error")
		}
		if got != nil {
			t.Error("New() returned non-nil DB on error")
		}
	})

	t.Run("real pool", func(t *testing.T) {
		got, err := New(newTestPool(t))
		if err != nil {
			t.Fatalf("New() unexpected error = %v", err)
		}
		if got == nil {
			t.Fatal("New() returned nil DB, want non-nil")
		}
		if got.TrManager() == nil {
			t.Error("TrManager() = nil, want non-nil")
		}
		if err = got.PingContext(context.Background()); err != nil {
			t.Errorf("PingContext() error = %v", err)
		}
	})
}

func TestDBDelegatesToPool(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)

	if _, err := database.ExecContext(ctx, "insert into users (name) values (?)", "alice"); err != nil {
		t.Fatalf("ExecContext() error = %v", err)
	}

	var name string
	if err := database.QueryRowContext(ctx, "select name from users where name = ?", "alice").Scan(&name); err != nil {
		t.Fatalf("QueryRowContext() error = %v", err)
	}
	if name != "alice" {
		t.Errorf("name = %q, want %q", name, "alice")
	}

	rows, err := database.QueryContext(ctx, "select name from users")
	if err != nil {
		t.Fatalf("QueryContext() error = %v", err)
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		count++
	}
	if err = rows.Err(); err != nil {
		t.Fatalf("rows.Err() = %v", err)
	}
	if count != 1 {
		t.Errorf("rows count = %d, want 1", count)
	}

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	if err = tx.Rollback(); err != nil {
		t.Errorf("Rollback() error = %v", err)
	}
}

func TestDBClose(t *testing.T) {
	ctx := context.Background()
	database, err := New(newTestPool(t))
	if err != nil {
		t.Fatalf("New() unexpected error = %v", err)
	}

	if err = database.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// После закрытия пул не работает — обёртка делегирует вызовы в database/sql.
	if err = database.PingContext(ctx); err == nil {
		t.Error("PingContext() error = nil, want error after Close")
	}
}

func TestTrManagerCommit(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)

	err := database.TrManager().Do(ctx, func(ctx context.Context) error {
		tx, ok := GetTx(ctx)
		if !ok {
			t.Fatal("GetTx() did not find the transaction in context")
		}

		_, execErr := tx.ExecContext(ctx, "insert into users (name) values (?)", "alice")

		return execErr
	})
	if err != nil {
		t.Fatalf("TrManager().Do() unexpected error = %v", err)
	}

	if got := countUsers(t, database); got != 1 {
		t.Errorf("users count = %d, want 1 (transaction should be committed)", got)
	}
}

func TestTrManagerRollback(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)

	err := database.TrManager().Do(ctx, func(ctx context.Context) error {
		tx, ok := GetTx(ctx)
		if !ok {
			t.Fatal("GetTx() did not find the transaction in context")
		}

		if _, execErr := tx.ExecContext(ctx, "insert into users (name) values (?)", "alice"); execErr != nil {
			return execErr
		}

		return errTest
	})
	if !errors.Is(err, errTest) {
		t.Fatalf("TrManager().Do() error = %v, want %v", err, errTest)
	}

	if got := countUsers(t, database); got != 0 {
		t.Errorf("users count = %d, want 0 (transaction should be rolled back)", got)
	}
}

func TestGetTxWithoutTransaction(t *testing.T) {
	if tx, ok := GetTx(context.Background()); ok || tx != nil {
		t.Errorf("GetTx() = (%v, %v), want (nil, false)", tx, ok)
	}
}

func TestTxFactoryBeginError(t *testing.T) {
	pool := &fakePool{beginErr: errTest}

	_, _, err := TxFactory(pool)(context.Background(), nil)

	if !errors.Is(err, errTest) {
		t.Errorf("TxFactory() error = %v, want %v", err, errTest)
	}
	if !errors.Is(err, trm.ErrBegin) {
		t.Errorf("TxFactory() error = %v, want it to wrap %v", err, trm.ErrBegin)
	}
	if pool.beginCalls != 1 {
		t.Errorf("pool BeginTx() calls = %d, want 1", pool.beginCalls)
	}
}

func TestTransactionCommit(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx() unexpected error = %v", err)
	}

	tr := newTransaction(tx)

	if got := tr.Transaction(); got != any(tx) {
		t.Errorf("Transaction() = %v, want the original *sql.Tx", got)
	}
	if !tr.IsActive() {
		t.Error("IsActive() = false, want true before commit")
	}

	if _, err = tx.ExecContext(ctx, "insert into users (name) values (?)", "bob"); err != nil {
		t.Fatalf("ExecContext() error = %v", err)
	}
	if err = tr.Commit(ctx); err != nil {
		t.Fatalf("Commit() unexpected error = %v", err)
	}

	if tr.IsActive() {
		t.Error("IsActive() = true, want false after commit")
	}
	assertClosed(t, tr.Closed())

	if got := countUsers(t, database); got != 1 {
		t.Errorf("users count = %d, want 1 after commit", got)
	}

	// Повторный Commit возвращает ошибку database/sql — она не скрывается.
	if err = tr.Commit(ctx); !errors.Is(err, sql.ErrTxDone) {
		t.Errorf("second Commit() error = %v, want %v", err, sql.ErrTxDone)
	}
}

func TestTransactionRollback(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx() unexpected error = %v", err)
	}

	tr := newTransaction(tx)

	if _, err = tx.ExecContext(ctx, "insert into users (name) values (?)", "bob"); err != nil {
		t.Fatalf("ExecContext() error = %v", err)
	}
	if err = tr.Rollback(ctx); err != nil {
		t.Fatalf("Rollback() unexpected error = %v", err)
	}

	if tr.IsActive() {
		t.Error("IsActive() = true, want false after rollback")
	}
	assertClosed(t, tr.Closed())

	if got := countUsers(t, database); got != 0 {
		t.Errorf("users count = %d, want 0 after rollback", got)
	}
}

// countUsers возвращает количество строк в таблице users.
func countUsers(t *testing.T, database DB) int {
	t.Helper()

	var count int
	if err := database.QueryRowContext(context.Background(), "select count(*) from users").Scan(&count); err != nil {
		t.Fatalf("QueryRowContext() error = %v", err)
	}

	return count
}

// assertClosed проверяет, что канал закрыт.
func assertClosed(t *testing.T, ch <-chan struct{}) {
	t.Helper()

	select {
	case <-ch:
	default:
		t.Error("Closed() channel is still open, want it closed")
	}
}
