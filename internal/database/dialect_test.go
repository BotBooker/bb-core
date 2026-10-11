package database

import (
	"errors"
	"testing"
)

func TestDialectFromDSN(t *testing.T) {
	tests := []struct {
		name    string
		dsn     string
		want    string
		wantErr error
	}{
		// PostgreSQL
		{
			name: "postgres url",
			dsn:  "postgres://user:pass@localhost:5432/demo",
			want: DialectPostgres,
		},
		{
			name: "postgresql url with query",
			dsn:  "postgresql://localhost:5432/demo?sslmode=disable",
			want: DialectPostgres,
		},
		{
			name: "postgres url in upper case",
			dsn:  "POSTGRES://localhost:5432/demo",
			want: DialectPostgres,
		},
		{
			name: "pgx keyword value dsn",
			dsn:  "host=localhost port=5432 dbname=demo user=postgres",
			want: DialectPostgres,
		},
		{
			name: "pgx keyword value dsn with one parameter",
			dsn:  "dbname=demo",
			want: DialectPostgres,
		},
		// SQLite
		{
			name: "sqlite dsn with scheme",
			dsn:  "sqlite:/tmp/database.db",
			want: DialectSQLite,
		},
		{
			name: "sqlite3 dsn with scheme",
			dsn:  "sqlite3:/tmp/database.db",
			want: DialectSQLite,
		},
		{
			name: "sqlite dsn with three slashes",
			dsn:  "sqlite:///tmp/database.db",
			want: DialectSQLite,
		},
		{
			name: "file uri",
			dsn:  "file:/tmp/database.db",
			want: DialectSQLite,
		},
		{
			name: "file uri in upper case",
			dsn:  "FILE::memory:?cache=shared",
			want: DialectSQLite,
		},
		{
			name: "in-memory database",
			dsn:  ":memory:",
			want: DialectSQLite,
		},
		{
			name: "absolute file path",
			dsn:  "/tmp/database.db",
			want: DialectSQLite,
		},
		{
			name: "relative file path",
			dsn:  "./data/demo.sqlite",
			want: DialectSQLite,
		},
		{
			name: "file name with sqlite3 extension",
			dsn:  "demo.sqlite3",
			want: DialectSQLite,
		},
		{
			name: "windows file path",
			dsn:  `C:\data\demo.db`,
			want: DialectSQLite,
		},
		{
			name: "dsn with surrounding spaces",
			dsn:  "  sqlite:/tmp/database.db  ",
			want: DialectSQLite,
		},
		// Ошибки
		{
			name:    "empty dsn",
			dsn:     "",
			wantErr: ErrEmptyDSN,
		},
		{
			name:    "blank dsn",
			dsn:     "   ",
			wantErr: ErrEmptyDSN,
		},
		{
			name:    "unknown scheme",
			dsn:     "mysql://localhost:3306/demo",
			wantErr: ErrUnsupportedDialect,
		},
		{
			name:    "host and port instead of url",
			dsn:     "localhost:5432/demo",
			wantErr: ErrUnsupportedDialect,
		},
		{
			name:    "unrecognized dsn without scheme",
			dsn:     "database",
			wantErr: ErrDialectNotDetected,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DialectFromDSN(tt.dsn)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("DialectFromDSN(%q) error = %v, want %v", tt.dsn, err, tt.wantErr)
				}
				if got != "" {
					t.Errorf("DialectFromDSN(%q) = %q, want empty dialect on error", tt.dsn, got)
				}

				return
			}

			if err != nil {
				t.Fatalf("DialectFromDSN(%q) unexpected error = %v", tt.dsn, err)
			}
			if got != tt.want {
				t.Errorf("DialectFromDSN(%q) = %q, want %q", tt.dsn, got, tt.want)
			}
		})
	}
}

func TestDsnScheme(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want string
	}{
		{
			name: "url scheme",
			dsn:  "postgres://localhost/demo",
			want: "postgres",
		},
		{
			name: "scheme without slashes",
			dsn:  "sqlite:/tmp/database.db",
			want: "sqlite",
		},
		{
			name: "keyword value dsn has no scheme",
			dsn:  "host=localhost dbname=demo",
			want: "",
		},
		{
			name: "in-memory database has no scheme",
			dsn:  ":memory:",
			want: "",
		},
		{
			name: "drive letter is not a scheme",
			dsn:  `C:\data\demo.db`,
			want: "",
		},
		{
			name: "invalid scheme characters",
			dsn:  "host=localhost:5432",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dsnScheme(tt.dsn); got != tt.want {
				t.Errorf("dsnScheme(%q) = %q, want %q", tt.dsn, got, tt.want)
			}
		})
	}
}
