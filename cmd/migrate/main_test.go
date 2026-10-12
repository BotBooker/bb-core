package main

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/botbooker/bb-core/internal/config"
	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/database/sqlite"
)

// sqliteConfig возвращает конфигурацию файловой SQLite во временном каталоге:
// каждый тест работает со своей базой, поэтому порядок тестов не важен.
func sqliteConfig(t *testing.T) config.DatabaseConfig {
	t.Helper()

	return config.DatabaseConfig{DSN: filepath.Join(t.TempDir(), "database.db"), MaxDBConnect: 1}
}

// tableExists открывает отдельное подключение и проверяет, что таблица есть.
func tableExists(t *testing.T, cfg config.DatabaseConfig, table string) bool {
	t.Helper()

	ctx := context.Background()

	conn, err := sqlite.New(ctx, cfg)
	if err != nil {
		t.Fatalf("sqlite.New() unexpected error = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	var count int

	// Имя таблицы подставляет тест, пользовательский ввод сюда не попадает.
	err = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count)

	return err == nil
}

func TestRun_Up(t *testing.T) {
	ctx := context.Background()
	cfg := sqliteConfig(t)

	t.Run("без аргументов применяет миграции", func(t *testing.T) {
		if err := run(ctx, cfg, nil); err != nil {
			t.Fatalf("run(nil) unexpected error = %v", err)
		}

		if !tableExists(t, cfg, "users") {
			t.Error("после run() таблицы users нет, want созданную миграцией")
		}
	})

	t.Run("повторный запуск идемпотентен", func(t *testing.T) {
		if err := run(ctx, cfg, []string{commandUp}); err != nil {
			t.Fatalf("повторный run(up) unexpected error = %v", err)
		}
	})
}

func TestRun_BadArgs(t *testing.T) {
	ctx := context.Background()

	t.Run("неизвестная команда", func(t *testing.T) {
		err := run(ctx, sqliteConfig(t), []string{"rollback"})

		if !errors.Is(err, errUsage) {
			t.Fatalf("run() error = %v, want %v", err, errUsage)
		}
	})

	t.Run("лишние аргументы", func(t *testing.T) {
		// Опечатка "migrate down DSN=..." не должна молча выполниться
		// для DSN из окружения.
		err := run(ctx, sqliteConfig(t), []string{commandDown, "DSN=/tmp/database.db"})

		if !errors.Is(err, errUsage) {
			t.Fatalf("run() error = %v, want %v", err, errUsage)
		}
	})
}

func TestRun_DialectError(t *testing.T) {
	ctx := context.Background()

	t.Run("неизвестная схема DSN", func(t *testing.T) {
		cfg := config.DatabaseConfig{DSN: "mysql://localhost:3306/demo"}

		if err := run(ctx, cfg, []string{commandDown}); !errors.Is(err, database.ErrUnsupportedDialect) {
			t.Fatalf("run() error = %v, want %v", err, database.ErrUnsupportedDialect)
		}
	})

	t.Run("пустой DSN", func(t *testing.T) {
		if err := run(ctx, config.DatabaseConfig{}, []string{commandDown}); !errors.Is(err, database.ErrEmptyDSN) {
			t.Fatalf("run() error = %v, want %v", err, database.ErrEmptyDSN)
		}
	})
}

func TestRun_OpenDatabaseError(t *testing.T) {
	// Каталога нет — SQLite не сможет создать файл базы.
	cfg := config.DatabaseConfig{DSN: filepath.Join(t.TempDir(), "missing", "database.db")}

	err := run(context.Background(), cfg, []string{commandDown})

	if err == nil {
		t.Fatal("run() error = nil, want ошибку открытия базы")
	}
}

func TestOpenDatabase_UnsupportedDialect(t *testing.T) {
	db, err := openDatabase(context.Background(), sqliteConfig(t), "mysql")

	if !errors.Is(err, database.ErrUnsupportedDialect) {
		t.Fatalf("openDatabase() error = %v, want %v", err, database.ErrUnsupportedDialect)
	}
	if db != nil {
		t.Errorf("openDatabase() = %v, want nil", db)
	}
}

func TestOpenDatabase_Postgres(t *testing.T) {
	// Порт освобождается сразу после резервирования: pgx получает отказ на
	// подключении, и тест не зависит от доступности реального сервера PostgreSQL.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() unexpected error = %v", err)
	}

	addr := listener.Addr().String()

	if err := listener.Close(); err != nil {
		t.Fatalf("listener.Close() unexpected error = %v", err)
	}

	cfg := config.DatabaseConfig{DSN: "postgres://user:password@" + addr + "/demo"}

	db, err := openDatabase(context.Background(), cfg, database.DialectPostgres)

	if err == nil {
		t.Fatal("openDatabase() error = nil, want ошибку подключения к PostgreSQL")
	}
	if db != nil {
		t.Errorf("openDatabase() = %v, want nil", db)
	}
}

// withConfig подменяет конфигурацию подключения на время теста:
// config.DBConfig() — синглтон, и тесты обязаны вернуть его в исходное состояние.
func withConfig(t *testing.T, cfg config.DatabaseConfig) {
	t.Helper()

	original := *config.DBConfig()
	*config.DBConfig() = cfg
	t.Cleanup(func() { *config.DBConfig() = original })
}

// withArgs подменяет аргументы командной строки на время теста.
func withArgs(t *testing.T, args ...string) {
	t.Helper()

	original := os.Args
	os.Args = append([]string{"migrate"}, args...)
	t.Cleanup(func() { os.Args = original })
}

// withOSExit подменяет osExit и возвращает указатель на записанный код выхода.
// Если osExit не вызывался, значение остаётся равным -1.
func withOSExit(t *testing.T) *int {
	t.Helper()

	original := osExit
	code := -1
	osExit = func(c int) { code = c }
	t.Cleanup(func() { osExit = original })

	return &code
}

func TestMain_AppliesMigrations(t *testing.T) {
	cfg := sqliteConfig(t)
	withConfig(t, cfg)
	withArgs(t)

	if code := Main(); code != 0 {
		t.Errorf("Main() = %d, want 0", code)
	}

	if !tableExists(t, cfg, "users") {
		t.Error("после Main() таблицы users нет, want созданную миграцией")
	}
}

func TestMain_Error(t *testing.T) {
	withConfig(t, sqliteConfig(t))
	withArgs(t, "rollback")

	if code := Main(); code != 1 {
		t.Errorf("Main() = %d, want 1", code)
	}
}

func TestMain_ExitsOnError(t *testing.T) {
	code := withOSExit(t)
	withConfig(t, sqliteConfig(t))
	withArgs(t, "rollback")

	main()

	if *code != 1 {
		t.Errorf("main() exit code = %d, want 1", *code)
	}
}

func TestMain_NoExitOnSuccess(t *testing.T) {
	code := withOSExit(t)
	withConfig(t, sqliteConfig(t))
	withArgs(t, commandUp)

	main()

	if *code != -1 {
		t.Errorf("main() called osExit(%d) on success, want no exit", *code)
	}
}
