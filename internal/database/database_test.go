package database

import (
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name       string
		dsn        string
		wantErr    bool
		wantNotNil bool
	}{
		{
			name:       "valid DSN",
			dsn:        "postgres://user:pass@localhost:5432/db",
			wantErr:    false,
			wantNotNil: true,
		},
		{
			name:       "empty DSN",
			dsn:        "",
			wantErr:    true,
			wantNotNil: false,
		},
		{
			name:       "DSN with special characters",
			dsn:        "postgres://user:p@ssw0rd@db-host:5432/my_db?sslmode=disable",
			wantErr:    false,
			wantNotNil: true,
		},
		{
			name:       "minimal DSN",
			dsn:        "x",
			wantErr:    false,
			wantNotNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := New(tt.dsn)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantNotNil && got == nil {
				t.Error("New() returned nil, expected non-nil DB")
			}
			if tt.wantErr && got != nil {
				t.Error("New() should return nil on error")
			}
		})
	}
}

func TestDB_Query(t *testing.T) {
	db, _ := New("postgres://localhost:5432/db")

	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{
			name:    "simple select",
			query:   "SELECT * FROM users",
			wantErr: false,
		},
		{
			name:    "empty query",
			query:   "",
			wantErr: false,
		},
		{
			name:    "complex query",
			query:   "SELECT u.id, u.name, p.email FROM users u JOIN profiles p ON u.id = p.user_id WHERE u.active = true ORDER BY u.created_at DESC LIMIT 100",
			wantErr: false,
		},
		{
			name:    "query with special characters",
			query:   "SELECT * FROM users WHERE name = 'O''Brien' AND email LIKE '%@%.%'",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := db.Query(tt.query)
			if (err != nil) != tt.wantErr {
				t.Errorf("Query() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDB_QueryRow(t *testing.T) {
	db, _ := New("postgres://localhost:5432/db")

	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{
			name:    "select single row",
			query:   "SELECT name FROM users WHERE id = 1",
			wantErr: false,
		},
		{
			name:    "empty query",
			query:   "",
			wantErr: false,
		},
		{
			name:    "aggregate query",
			query:   "SELECT COUNT(*) FROM users",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := db.QueryRow(tt.query)
			if (err != nil) != tt.wantErr {
				t.Errorf("QueryRow() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDB_Exec(t *testing.T) {
	db, _ := New("postgres://localhost:5432/db")

	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{
			name:    "insert statement",
			query:   "INSERT INTO users (name, email) VALUES ('John', 'john@example.com')",
			wantErr: false,
		},
		{
			name:    "update statement",
			query:   "UPDATE users SET active = true WHERE id = 1",
			wantErr: false,
		},
		{
			name:    "delete statement",
			query:   "DELETE FROM users WHERE id = 999",
			wantErr: false,
		},
		{
			name:    "empty query",
			query:   "",
			wantErr: false,
		},
		{
			name:    "DDL statement",
			query:   "CREATE TABLE IF NOT EXISTS test (id SERIAL PRIMARY KEY)",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := db.Exec(tt.query)
			if (err != nil) != tt.wantErr {
				t.Errorf("Exec() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDB_BeginTx(t *testing.T) {
	db, _ := New("postgres://localhost:5432/db")

	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "begin transaction",
			wantErr: false,
		},
		{
			name:    "multiple begin calls",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := db.BeginTx()
			if (err != nil) != tt.wantErr {
				t.Errorf("BeginTx() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDB_BulkInsert(t *testing.T) {
	db, _ := New("postgres://localhost:5432/db")

	tests := []struct {
		name    string
		query   string
		args    []any
		wantErr bool
	}{
		{
			name:    "bulk insert with args",
			query:   "INSERT INTO users (name, email) VALUES ($1, $2), ($3, $4)",
			args:    []any{"Alice", "alice@example.com", "Bob", "bob@example.com"},
			wantErr: false,
		},
		{
			name:    "bulk insert no args",
			query:   "INSERT INTO logs (message) VALUES ('test')",
			args:    nil,
			wantErr: false,
		},
		{
			name:    "bulk insert empty query",
			query:   "",
			args:    []any{1, 2, 3},
			wantErr: false,
		},
		{
			name:    "bulk insert with many args",
			query:   "INSERT INTO items (a, b, c, d, e) VALUES ($1, $2, $3, $4, $5)",
			args:    []any{1, "two", 3.0, true, nil},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := db.BulkInsert(tt.query, tt.args...)
			if (err != nil) != tt.wantErr {
				t.Errorf("BulkInsert() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDB_Close(t *testing.T) {
	db, _ := New("postgres://localhost:5432/db")

	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "close after operations",
			wantErr: false,
		},
		{
			name:    "close twice",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := db.Close()
			if (err != nil) != tt.wantErr {
				t.Errorf("Close() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.name == "close twice" {
				err = db.Close()
				if (err != nil) != tt.wantErr {
					t.Errorf("Close() second call error = %v, wantErr %v", err, tt.wantErr)
				}
			}
		})
	}
}

func TestNew_ErrorMessage(t *testing.T) {
	_, err := New("")
	if err == nil {
		t.Fatal("Expected error for empty DSN")
	}
	expectedErr := "dsn пустой"
	if err.Error() != expectedErr {
		t.Errorf("New() error = %v, want %v", err.Error(), expectedErr)
	}
}
