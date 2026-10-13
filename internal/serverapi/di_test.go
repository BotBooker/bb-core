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

// withDatabase подменяет подключение к базе данных и применение миграций
// на время теста.
func withDatabase(t *testing.T, db database.DB, dbErr error) {
	t.Helper()

	originalDatabase, originalMigrate := newDatabase, migrateDatabase
	newDatabase = func(context.Context) (database.DB, error) { return db, dbErr }
	migrateDatabase = func(context.Context, database.DB) error { return nil }
	t.Cleanup(func() {
		newDatabase = originalDatabase
		migrateDatabase = originalMigrate
	})
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

// TestDIContainer_MigrationsError проверяет, что ошибка миграций не даёт
// поднять приложение, а подключение при этом закрывается.
func TestDIContainer_MigrationsError(t *testing.T) {
	db := &fakeDB{}

	originalDatabase, originalMigrate := newDatabase, migrateDatabase

	newDatabase = func(context.Context) (database.DB, error) { return db, nil }
	migrateDatabase = func(context.Context, database.DB) error { return errTest }

	t.Cleanup(func() {
		newDatabase, migrateDatabase = originalDatabase, originalMigrate
	})

	container, err := newDIContainer(context.Background())

	if !errors.Is(err, errTest) {
		t.Errorf("newDIContainer() error = %v, want %v", err, errTest)
	}
	if container != nil {
		t.Error("newDIContainer() returned non-nil container on error")
	}
	if db.closeCalls != 1 {
		t.Errorf("closeCalls = %d, want 1: при ошибке миграций пул закрывается", db.closeCalls)
	}
}

// TestApplyMigrations проверяет гейт MIGRATE_ON_START и выбор диалекта по DSN.
func TestApplyMigrations(t *testing.T) {
	ctx := context.Background()
	originalDSN, originalMigrateOnStart := config.DBConfig().DSN, config.DBConfig().MigrateOnStart

	t.Cleanup(func() {
		config.DBConfig().DSN = originalDSN
		config.DBConfig().MigrateOnStart = originalMigrateOnStart
	})

	t.Run("миграции выключены настройкой", func(t *testing.T) {
		config.DBConfig().MigrateOnStart = false
		config.DBConfig().DSN = "mysql://localhost:3306/demo"

		if err := applyMigrations(ctx, &fakeDB{}); err != nil {
			t.Fatalf("applyMigrations() error = %v, want nil: DSN не должен читаться", err)
		}
	})

	t.Run("неизвестный диалект", func(t *testing.T) {
		config.DBConfig().MigrateOnStart = true
		config.DBConfig().DSN = "mysql://localhost:3306/demo"

		if err := applyMigrations(ctx, &fakeDB{}); !errors.Is(err, database.ErrUnsupportedDialect) {
			t.Fatalf("applyMigrations() error = %v, want %v", err, database.ErrUnsupportedDialect)
		}
	})

	t.Run("подключение без пула database/sql", func(t *testing.T) {
		config.DBConfig().MigrateOnStart = true
		config.DBConfig().DSN = "sqlite:/tmp/migrations-test.db"

		if err := applyMigrations(ctx, &fakeDB{}); !errors.Is(err, database.ErrMigrationsUnavailable) {
			t.Fatalf("applyMigrations() error = %v, want %v", err, database.ErrMigrationsUnavailable)
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
	ctx := context.Background()
	c := newTestContainer(t)

	cache := c.Cache(ctx)
	if cache == nil {
		t.Fatal("Cache() returned nil")
	}

	// Second call should return same instance
	cache2 := c.Cache(ctx)
	if cache != cache2 {
		t.Error("Cache() should return same instance on multiple calls")
	}
}

func TestDIContainer_EventBus(t *testing.T) {
	ctx := context.Background()
	c := newTestContainer(t)

	bus := c.EventBus(ctx)
	if bus == nil {
		t.Fatal("EventBus() returned nil")
	}

	// Second call should return same instance
	bus2 := c.EventBus(ctx)
	if bus != bus2 {
		t.Error("EventBus() should return same instance on multiple calls")
	}
}

func TestDIContainer_UserRepo(t *testing.T) {
	ctx := context.Background()
	c := newTestContainer(t)

	repo := c.UserRepo(ctx)
	if repo == nil {
		t.Fatal("UserRepo() returned nil")
	}

	// Second call should return same instance
	repo2 := c.UserRepo(ctx)
	if repo != repo2 {
		t.Error("UserRepo() should return same instance on multiple calls")
	}
}

func TestDIContainer_TokenRepo(t *testing.T) {
	ctx := context.Background()
	c := newTestContainer(t)

	repo := c.TokenRepo(ctx)
	if repo == nil {
		t.Fatal("TokenRepo() returned nil")
	}

	// Second call should return same instance
	repo2 := c.TokenRepo(ctx)
	if repo != repo2 {
		t.Error("TokenRepo() should return same instance on multiple calls")
	}
}

func TestDIContainer_NotificationRepo(t *testing.T) {
	ctx := context.Background()
	c := newTestContainer(t)

	repo := c.NotificationRepo(ctx)
	if repo == nil {
		t.Fatal("NotificationRepo() returned nil")
	}

	// Second call should return same instance
	repo2 := c.NotificationRepo(ctx)
	if repo != repo2 {
		t.Error("NotificationRepo() should return same instance on multiple calls")
	}
}

func TestDIContainer_AuthService(t *testing.T) {
	ctx := context.Background()
	c := newTestContainer(t)

	svc := c.AuthService(ctx)
	if svc == nil {
		t.Fatal("AuthService() returned nil")
	}

	// Second call should return same instance
	svc2 := c.AuthService(ctx)
	if svc != svc2 {
		t.Error("AuthService() should return same instance on multiple calls")
	}
}

func TestDIContainer_UserService(t *testing.T) {
	ctx := context.Background()
	c := newTestContainer(t)

	svc := c.UserService(ctx)
	if svc == nil {
		t.Fatal("UserService() returned nil")
	}

	// Second call should return same instance
	svc2 := c.UserService(ctx)
	if svc != svc2 {
		t.Error("UserService() should return same instance on multiple calls")
	}
}

func TestDIContainer_NotificationService(t *testing.T) {
	ctx := context.Background()
	c := newTestContainer(t)

	svc := c.NotificationService(ctx)
	if svc == nil {
		t.Fatal("NotificationService() returned nil")
	}

	// Second call should return same instance
	svc2 := c.NotificationService(ctx)
	if svc != svc2 {
		t.Error("NotificationService() should return same instance on multiple calls")
	}
}

func TestDIContainer_Handler(t *testing.T) {
	ctx := context.Background()
	c := newTestContainer(t)

	h, err := c.Handler(ctx)
	if err != nil {
		t.Fatalf("Handler() unexpected error = %v", err)
	}
	if h == nil {
		t.Fatal("Handler() returned nil")
	}

	// Second call should return same instance
	h2, err := c.Handler(ctx)
	if err != nil {
		t.Fatalf("Handler() unexpected error = %v", err)
	}
	if h != h2 {
		t.Error("Handler() should return same instance on multiple calls")
	}
}

func TestDIContainer_UserServiceServer(t *testing.T) {
	ctx := context.Background()
	c := newTestContainer(t)

	server := c.UserServiceServer(ctx)
	if server == nil {
		t.Fatal("UserServiceServer() returned nil")
	}

	// Один адаптер обслуживает и gRPC, и grpc-gateway: поведение транспортов
	// не может разойтись.
	server2 := c.UserServiceServer(ctx)
	if server != server2 {
		t.Error("UserServiceServer() should return same instance on multiple calls")
	}
}

func TestDIContainer_Dialect(t *testing.T) {
	c := newTestContainer(t)

	want, err := database.DialectFromDSN(config.DBConfig().DSN)
	if err != nil {
		t.Fatalf("DialectFromDSN() unexpected error = %v", err)
	}

	if c.dialect != want {
		t.Errorf("dialect = %q, want %q", c.dialect, want)
	}
}

func TestDIContainer_Integration(t *testing.T) {
	// Контейнер создаётся с фейковым подключением к БД: кэш, репозитории,
	// сервисы и хендлер собираются настоящие, без внешних зависимостей.
	c := newTestContainer(t)
	ctx := context.Background()

	// Get all dependencies
	db := c.DB()
	cache := c.Cache(ctx)
	bus := c.EventBus(ctx)
	userRepo := c.UserRepo(ctx)
	tokenRepo := c.TokenRepo(ctx)
	notifRepo := c.NotificationRepo(ctx)
	authSvc := c.AuthService(ctx)
	userSvc := c.UserService(ctx)
	notifSvc := c.NotificationService(ctx)
	userServiceServer := c.UserServiceServer(ctx)
	handler, err := c.Handler(ctx)
	if err != nil {
		t.Fatalf("Handler() unexpected error = %v", err)
	}

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
	if tokenRepo == nil {
		t.Error("TokenRepo is nil")
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
	if userServiceServer == nil {
		t.Error("UserServiceServer is nil")
	}
	if handler == nil {
		t.Error("Handler is nil")
	}
}
