package main

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"testing"

	"github.com/botbooker/bb-core/internal/config"
	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/database/sqlite"
	"github.com/botbooker/bb-core/internal/model"
	"github.com/botbooker/bb-core/internal/repository"
)

// testMigrationsDir — каталог с SQL-миграциями относительно пакета: go test
// запускает тесты с рабочим каталогом пакета.
const testMigrationsDir = "../../migrations"

// sqliteConfig возвращает конфигурацию файловой SQLite во временном каталоге:
// каждый тест работает со своей базой, поэтому порядок тестов не важен.
func sqliteConfig(t *testing.T) config.DatabaseConfig {
	t.Helper()

	return config.DatabaseConfig{
		DSN:           filepath.Join(t.TempDir(), "database.db"),
		MaxDBConnect:  1,
		MigrationsDir: testMigrationsDir,
	}
}

func superuserConfig() config.BootstrapConfig {
	return config.BootstrapConfig{
		Email:     "admin@example.com",
		Password:  "super-secret-password",
		FirstName: "Super",
		LastName:  "Admin",
	}
}

// findSuperuser читает суперпользователя отдельным подключением.
func findSuperuser(t *testing.T, cfg config.DatabaseConfig) *model.User {
	t.Helper()

	ctx := context.Background()

	conn, err := sqlite.New(ctx, cfg)
	if err != nil {
		t.Fatalf("sqlite.New() unexpected error = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	user, err := repository.NewUserRepo(ctx, conn, database.DialectSQLite).GetByEmail(ctx, "admin@example.com")
	if err != nil {
		t.Fatalf("GetByEmail() unexpected error = %v", err)
	}

	return user
}

// findMembership читает членство суперпользователя в тенанте: роль admin
// хранится не в профиле, а в tenant_members.role_id.
func findMembership(t *testing.T, cfg config.DatabaseConfig, user *model.User) *model.TenantMember {
	t.Helper()

	ctx := context.Background()

	conn, err := sqlite.New(ctx, cfg)
	if err != nil {
		t.Fatalf("sqlite.New() unexpected error = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	member, err := repository.NewTenantRepo(ctx, conn, database.DialectSQLite).MemberOfUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("MemberOfUser() unexpected error = %v", err)
	}

	return member
}

func TestRun(t *testing.T) {
	ctx := context.Background()
	cfg := sqliteConfig(t)

	t.Run("создаёт суперпользователя", func(t *testing.T) {
		if err := run(ctx, cfg, superuserConfig()); err != nil {
			t.Fatalf("run() unexpected error = %v", err)
		}

		user := findSuperuser(t, cfg)

		if !user.IsSuperuser {
			t.Error("IsSuperuser = false, want true")
		}

		// Роль — атрибут членства в тенанте: суперпользователь владелец
		// личного тенанта с ролью admin.
		member := findMembership(t, cfg, user)

		if member.Role != model.RoleAdmin {
			t.Errorf("Role = %q, want %q", member.Role, model.RoleAdmin)
		}
		if !member.IsOwner {
			t.Error("IsOwner = false, want true: суперпользователь владеет личным тенантом")
		}
	})

	t.Run("повторный запуск идемпотентен", func(t *testing.T) {
		if err := run(ctx, cfg, superuserConfig()); err != nil {
			t.Fatalf("повторный run() unexpected error = %v", err)
		}
	})

	t.Run("без конфигурации — ошибка", func(t *testing.T) {
		if err := run(ctx, sqliteConfig(t), config.BootstrapConfig{}); err == nil {
			t.Error("run() error = nil, want ошибку конфигурации")
		}
	})
}

func TestRun_DialectError(t *testing.T) {
	cfg := config.DatabaseConfig{DSN: "mysql://localhost:3306/demo"}

	if err := run(context.Background(), cfg, superuserConfig()); !errors.Is(err, database.ErrUnsupportedDialect) {
		t.Fatalf("run() error = %v, want %v", err, database.ErrUnsupportedDialect)
	}
}

func TestRun_OpenDatabaseError(t *testing.T) {
	// Каталога нет — SQLite не сможет создать файл базы.
	cfg := config.DatabaseConfig{DSN: filepath.Join(t.TempDir(), "missing", "database.db")}

	if err := run(context.Background(), cfg, superuserConfig()); err == nil {
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
	// Порт освобождается сразу после резервирования: тест не зависит от
	// доступности реального сервера PostgreSQL.
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

// withConfig подменяет конфигурацию подключения и bootstrap на время теста:
// config.DBConfig() и config.Bootstrap() — синглтоны, их нужно вернуть назад.
func withConfig(t *testing.T, dbCfg config.DatabaseConfig, bootCfg config.BootstrapConfig) {
	t.Helper()

	originalDB := *config.DBConfig()
	originalBoot := *config.Bootstrap()

	*config.DBConfig() = dbCfg
	*config.Bootstrap() = bootCfg

	t.Cleanup(func() {
		*config.DBConfig() = originalDB
		*config.Bootstrap() = originalBoot
	})
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

func TestMain_CreatesSuperuser(t *testing.T) {
	cfg := sqliteConfig(t)
	withConfig(t, cfg, superuserConfig())

	if code := Main(); code != 0 {
		t.Errorf("Main() = %d, want 0", code)
	}

	user := findSuperuser(t, cfg)

	if !user.IsSuperuser {
		t.Error("IsSuperuser = false, want true")
	}

	if member := findMembership(t, cfg, user); member.Role != model.RoleAdmin {
		t.Errorf("Role = %q, want %q", member.Role, model.RoleAdmin)
	}
}

func TestMain_Error(t *testing.T) {
	withConfig(t, sqliteConfig(t), config.BootstrapConfig{})

	if code := Main(); code != 1 {
		t.Errorf("Main() = %d, want 1", code)
	}
}

func TestMain_ExitsOnError(t *testing.T) {
	code := withOSExit(t)
	withConfig(t, sqliteConfig(t), config.BootstrapConfig{})

	main()

	if *code != 1 {
		t.Errorf("main() exit code = %d, want 1", *code)
	}
}

func TestMain_NoExitOnSuccess(t *testing.T) {
	code := withOSExit(t)
	withConfig(t, sqliteConfig(t), superuserConfig())

	main()

	if *code != -1 {
		t.Errorf("main() called osExit(%d) on success, want no exit", *code)
	}
}
