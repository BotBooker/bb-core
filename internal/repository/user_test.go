package repository

import (
	"context"
	"database/sql"
	"testing"
)

// mockUserDB is a mock implementation of UserDB for testing.
type mockUserDB struct {
	queryFunc    func(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	queryRowFunc func(ctx context.Context, query string, args ...any) *sql.Row
	execFunc     func(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (m *mockUserDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, query, args...)
	}
	return nil, nil
}

func (m *mockUserDB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	if m.queryRowFunc != nil {
		return m.queryRowFunc(ctx, query, args...)
	}
	return nil
}

func (m *mockUserDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if m.execFunc != nil {
		return m.execFunc(ctx, query, args...)
	}
	return nil, nil
}

func TestNewUserRepo(t *testing.T) {
	tests := []struct {
		name string
		db   UserDB
	}{
		{
			name: "valid db",
			db:   &mockUserDB{},
		},
		{
			name: "nil db",
			db:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewUserRepo(tt.db)
			if got == nil {
				t.Error("NewUserRepo() returned nil")
			}
		})
	}
}
