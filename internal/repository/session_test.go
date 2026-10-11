package repository

import (
	"context"
	"database/sql"
	"testing"
)

// mockSessionDB is a mock implementation of SessionDB for testing.
type mockSessionDB struct {
	queryFunc   func(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	execFunc    func(ctx context.Context, query string, args ...any) (sql.Result, error)
	beginTxFunc func(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

func (m *mockSessionDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, query, args...)
	}
	return nil, nil
}

func (m *mockSessionDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if m.execFunc != nil {
		return m.execFunc(ctx, query, args...)
	}
	return nil, nil
}

func (m *mockSessionDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	if m.beginTxFunc != nil {
		return m.beginTxFunc(ctx, opts)
	}
	return nil, nil
}

// mockSessionCache is a mock implementation of SessionCache for testing.
type mockSessionCache struct {
	getFunc func(key string) (string, error)
	setFunc func(key, value string) error
}

func (m *mockSessionCache) Get(key string) (string, error) {
	if m.getFunc != nil {
		return m.getFunc(key)
	}
	return "", nil
}

func (m *mockSessionCache) Set(key, value string) error {
	if m.setFunc != nil {
		return m.setFunc(key, value)
	}
	return nil
}

func TestNewSessionRepo(t *testing.T) {
	tests := []struct {
		name  string
		db    SessionDB
		cache SessionCache
	}{
		{
			name:  "valid db and cache",
			db:    &mockSessionDB{},
			cache: &mockSessionCache{},
		},
		{
			name:  "nil db",
			db:    nil,
			cache: &mockSessionCache{},
		},
		{
			name:  "nil cache",
			db:    &mockSessionDB{},
			cache: nil,
		},
		{
			name:  "both nil",
			db:    nil,
			cache: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewSessionRepo(tt.db, tt.cache)
			if got == nil {
				t.Error("NewSessionRepo() returned nil")
			}
		})
	}
}
