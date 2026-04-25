package repository

import (
	"testing"
)

// mockNotificationDB is a mock implementation of NotificationDB for testing.
type mockNotificationDB struct {
	queryFunc      func(query string) error
	execFunc       func(query string) error
	bulkInsertFunc func(query string, args ...any) error
}

func (m *mockNotificationDB) Query(query string) error {
	if m.queryFunc != nil {
		return m.queryFunc(query)
	}
	return nil
}

func (m *mockNotificationDB) Exec(query string) error {
	if m.execFunc != nil {
		return m.execFunc(query)
	}
	return nil
}

func (m *mockNotificationDB) BulkInsert(query string, args ...any) error {
	if m.bulkInsertFunc != nil {
		return m.bulkInsertFunc(query, args...)
	}
	return nil
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
			got := NewNotificationRepo(tt.db)
			if got == nil {
				t.Error("NewNotificationRepo() returned nil")
			}
		})
	}
}
