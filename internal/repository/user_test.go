package repository

import (
	"testing"
)

// mockUserDB is a mock implementation of UserDB for testing.
type mockUserDB struct {
	queryFunc    func(query string) error
	queryRowFunc func(query string) error
	execFunc     func(query string) error
}

func (m *mockUserDB) Query(query string) error {
	if m.queryFunc != nil {
		return m.queryFunc(query)
	}
	return nil
}

func (m *mockUserDB) QueryRow(query string) error {
	if m.queryRowFunc != nil {
		return m.queryRowFunc(query)
	}
	return nil
}

func (m *mockUserDB) Exec(query string) error {
	if m.execFunc != nil {
		return m.execFunc(query)
	}
	return nil
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
