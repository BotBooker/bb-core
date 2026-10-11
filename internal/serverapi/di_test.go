package serverapi

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	trm "github.com/avito-tech/go-transaction-manager/trm/v2"

	"github.com/botbooker/bb-core/internal/config"
	"github.com/botbooker/bb-core/internal/database"
)

// errTest — сентинельная ошибка для проверки проброса ошибок подключения.
var errTest = errors.New("test error")

// fakeDB — фейковое подключение к базе данных: тесты DI не ходят в настоящую БД.
type fakeDB struct {
	closeCalls int
}

var _ database.DB = (*fakeDB)(nil)

func (f *fakeDB) BeginTx(_ context.Context, _ *sql.TxOptions) (*sql.Tx, error) {
	return nil, nil
}

func (f *fakeDB) QueryContext(_ context.Context, _ string, _ ...any) (*sql.Rows, error) {
	return nil, nil
}

func (f *fakeDB) QueryRowContext(_ context.Context, _ string, _ ...any) *sql.Row {
	return nil
}

func (f *fakeDB) ExecContext(_ context.Context, _ string, _ ...any) (sql.Result, error) {
	return nil, nil
}

func (f *fakeDB) PingContext(_ context.Context) error {
	return nil
}

func (f *fakeDB) Close() error {
	f.closeCalls++

	return nil
}

func (f *fakeDB) TrManager() trm.Manager {
	return nil
}

// withDatabase подменяет подключение к базе данных на время теста.
func withDatabase(t *testing.T, db database.DB, dbErr error) {
	t.Helper()

	original := newDatabase
	newDatabase = func(context.Context) (database.DB, error) { return db, dbErr }
	t.Cleanup(func() { newDatabase = original })
}

// newTestContainer создаёт контейнер с фейковым подключением к базе данных.
func newTestContainer(t *testing.T) *diContainer {
	t.Helper()

	withDatabase(t, &fakeDB{}, nil)

	container, err := newDIContainer(context.Background())
	if err != nil {
		t.Fatalf("newDIContainer() unexpected error = %v", err)
	}

	return container
}

func TestDIContainer_NewDIContainer(t *testing.T) {
	t.Run("database is created together with container", func(t *testing.T) {
		c := newTestContainer(t)

		if c == nil {
			t.Fatal("newDIContainer() returned nil")
		}
		if c.db == nil {
			t.Error("db should be created together with the container")
		}
		if c.cache != nil {
			t.Error("cache should be nil initially")
		}
		if c.eventBus != nil {
			t.Error("eventBus should be nil initially")
		}
	})

	t.Run("database error is returned", func(t *testing.T) {
		withDatabase(t, nil, errTest)

		container, err := newDIContainer(context.Background())

		if !errors.Is(err, errTest) {
			t.Errorf("newDIContainer() error = %v, want %v", err, errTest)
		}
		if container != nil {
			t.Error("newDIContainer() returned non-nil container on error")
		}
	})
}

func TestNewDatabaseByDSN(t *testing.T) {
	originalPostgres, originalSQLite := newPostgresDB, newSQLiteDB
	originalDSN := config.DBConfig().DSN

	var postgresCalled, sqliteCalled bool

	t.Cleanup(func() {
		newPostgresDB, newSQLiteDB = originalPostgres, originalSQLite
		config.DBConfig().DSN = originalDSN
	})

	newPostgresDB = func(context.Context, config.DatabaseConfig) (database.DB, error) {
		postgresCalled = true

		return &fakeDB{}, nil
	}
	newSQLiteDB = func(context.Context, config.DatabaseConfig) (database.DB, error) {
		sqliteCalled = true

		return &fakeDB{}, nil
	}

	tests := []struct {
		name         string
		dsn          string
		wantPostgres bool
		wantSQLite   bool
		wantErr      error
	}{
		{
			name:         "postgres url",
			dsn:          "postgres://user:pass@localhost:5432/demo",
			wantPostgres: true,
		},
		{
			name:         "postgres keyword value dsn",
			dsn:          "host=localhost port=5432 dbname=demo",
			wantPostgres: true,
		},
		{
			name:       "sqlite file path",
			dsn:        "/tmp/database.db",
			wantSQLite: true,
		},
		{
			name:       "sqlite dsn with scheme",
			dsn:        "sqlite:/tmp/database.db",
			wantSQLite: true,
		},
		{
			name:    "empty dsn",
			dsn:     "",
			wantErr: database.ErrEmptyDSN,
		},
		{
			name:    "unknown scheme",
			dsn:     "mysql://localhost:3306/demo",
			wantErr: database.ErrUnsupportedDialect,
		},
		{
			name:    "unrecognized dsn",
			dsn:     "database",
			wantErr: database.ErrDialectNotDetected,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			postgresCalled, sqliteCalled = false, false
			config.DBConfig().DSN = tt.dsn

			got, err := newDatabaseByDSN(context.Background())

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("newDatabaseByDSN() error = %v, want %v", err, tt.wantErr)
				}
				if got != nil {
					t.Error("newDatabaseByDSN() returned non-nil DB on error")
				}
			} else if err != nil {
				t.Fatalf("newDatabaseByDSN() unexpected error = %v", err)
			}

			if postgresCalled != tt.wantPostgres {
				t.Errorf("postgres driver called = %v, want %v", postgresCalled, tt.wantPostgres)
			}
			if sqliteCalled != tt.wantSQLite {
				t.Errorf("sqlite driver called = %v, want %v", sqliteCalled, tt.wantSQLite)
			}
		})
	}
}

func TestDIContainer_DB(t *testing.T) {
	c := newTestContainer(t)

	db := c.DB()
	if db == nil {
		t.Fatal("DB() returned nil")
	}

	// Second call should return same instance
	db2 := c.DB()
	if db != db2 {
		t.Error("DB() should return same instance on multiple calls")
	}
}

func TestDIContainer_Cache(t *testing.T) {
	c := newTestContainer(t)
	cache := c.Cache()
	if cache == nil {
		t.Fatal("Cache() returned nil")
	}

	// Second call should return same instance
	cache2 := c.Cache()
	if cache != cache2 {
		t.Error("Cache() should return same instance on multiple calls")
	}
}

func TestDIContainer_EventBus(t *testing.T) {
	c := newTestContainer(t)
	bus := c.EventBus()
	if bus == nil {
		t.Fatal("EventBus() returned nil")
	}

	// Second call should return same instance
	bus2 := c.EventBus()
	if bus != bus2 {
		t.Error("EventBus() should return same instance on multiple calls")
	}
}

func TestDIContainer_UserRepo(t *testing.T) {
	c := newTestContainer(t)
	repo := c.UserRepo()
	if repo == nil {
		t.Fatal("UserRepo() returned nil")
	}

	// Second call should return same instance
	repo2 := c.UserRepo()
	if repo != repo2 {
		t.Error("UserRepo() should return same instance on multiple calls")
	}
}

func TestDIContainer_SessionRepo(t *testing.T) {
	c := newTestContainer(t)
	repo := c.SessionRepo()
	if repo == nil {
		t.Fatal("SessionRepo() returned nil")
	}

	// Second call should return same instance
	repo2 := c.SessionRepo()
	if repo != repo2 {
		t.Error("SessionRepo() should return same instance on multiple calls")
	}
}

func TestDIContainer_NotificationRepo(t *testing.T) {
	c := newTestContainer(t)
	repo := c.NotificationRepo()
	if repo == nil {
		t.Fatal("NotificationRepo() returned nil")
	}

	// Second call should return same instance
	repo2 := c.NotificationRepo()
	if repo != repo2 {
		t.Error("NotificationRepo() should return same instance on multiple calls")
	}
}

func TestDIContainer_AuthService(t *testing.T) {
	c := newTestContainer(t)
	svc := c.AuthService()
	if svc == nil {
		t.Fatal("AuthService() returned nil")
	}

	// Second call should return same instance
	svc2 := c.AuthService()
	if svc != svc2 {
		t.Error("AuthService() should return same instance on multiple calls")
	}
}

func TestDIContainer_UserService(t *testing.T) {
	c := newTestContainer(t)
	svc := c.UserService()
	if svc == nil {
		t.Fatal("UserService() returned nil")
	}

	// Second call should return same instance
	svc2 := c.UserService()
	if svc != svc2 {
		t.Error("UserService() should return same instance on multiple calls")
	}
}

func TestDIContainer_NotificationService(t *testing.T) {
	c := newTestContainer(t)
	svc := c.NotificationService()
	if svc == nil {
		t.Fatal("NotificationService() returned nil")
	}

	// Second call should return same instance
	svc2 := c.NotificationService()
	if svc != svc2 {
		t.Error("NotificationService() should return same instance on multiple calls")
	}
}

func TestDIContainer_Handler(t *testing.T) {
	c := newTestContainer(t)
	h := c.Handler()
	if h == nil {
		t.Fatal("Handler() returned nil")
	}

	// Second call should return same instance
	h2 := c.Handler()
	if h != h2 {
		t.Error("Handler() should return same instance on multiple calls")
	}
}

func TestDIContainer_Integration(t *testing.T) {
	// Контейнер создаётся с фейковым подключением к БД: кэш, репозитории,
	// сервисы и хендлер собираются настоящие, без внешних зависимостей.
	c := newTestContainer(t)

	// Get all dependencies
	db := c.DB()
	cache := c.Cache()
	bus := c.EventBus()
	userRepo := c.UserRepo()
	sessionRepo := c.SessionRepo()
	notifRepo := c.NotificationRepo()
	authSvc := c.AuthService()
	userSvc := c.UserService()
	notifSvc := c.NotificationService()
	handler := c.Handler()

	if db == nil {
		t.Error("DB is nil")
	}
	if cache == nil {
		t.Error("Cache is nil")
	}
	if bus == nil {
		t.Error("EventBus is nil")
	}
	if userRepo == nil {
		t.Error("UserRepo is nil")
	}
	if sessionRepo == nil {
		t.Error("SessionRepo is nil")
	}
	if notifRepo == nil {
		t.Error("NotificationRepo is nil")
	}
	if authSvc == nil {
		t.Error("AuthService is nil")
	}
	if userSvc == nil {
		t.Error("UserService is nil")
	}
	if notifSvc == nil {
		t.Error("NotificationService is nil")
	}
	if handler == nil {
		t.Error("Handler is nil")
	}
}
