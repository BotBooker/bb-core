package repository

import (
	"context"
	"database/sql"
	"testing"
)

// mockNotificationDB is a mock implementation of NotificationDB for testing.
type mockNotificationDB struct {
	queryFunc func(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	execFunc  func(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (m *mockNotificationDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, query, args...)
	}
	return nil, nil
}

func (m *mockNotificationDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if m.execFunc != nil {
		return m.execFunc(ctx, query, args...)
	}
	return nil, nil
}

func TestNewNotificationRepo(t *testing.T) {
	tests := []struct {
		name string
		db   NotificationDB
	}{
		{
			name: "valid db",
			db:   &mockNotificationDB{},
		},
		{
			name: "nil db",
			db:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewNotificationRepo(context.Background(), tt.db)
			if got == nil {
				t.Error("NewNotificationRepo() returned nil")
			}
		})
	}
}
