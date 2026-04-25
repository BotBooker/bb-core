package repository

import (
	"testing"
)

// mockSessionDB is a mock implementation of SessionDB for testing.
type mockSessionDB struct {
	queryFunc   func(query string) error
	execFunc    func(query string) error
	beginTxFunc func() error
}

func (m *mockSessionDB) Query(query string) error {
	if m.queryFunc != nil {
		return m.queryFunc(query)
	}
	return nil
}

func (m *mockSessionDB) Exec(query string) error {
	if m.execFunc != nil {
		return m.execFunc(query)
	}
	return nil
}

func (m *mockSessionDB) BeginTx() error {
	if m.beginTxFunc != nil {
		return m.beginTxFunc()
	}
	return nil
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
