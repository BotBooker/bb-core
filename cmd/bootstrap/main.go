// Command bootstrap первично инициализирует систему:
//
//	DSN=sqlite:/tmp/database.db \
//	  SUPERUSER_EMAIL=admin@example.com \
//	  SUPERUSER_PASSWORD='change-me-please' \
//	  go run ./cmd/bootstrap
//
// Команда применяет миграции (как и старт приложения, см. MIGRATE_ON_START) и
// создаёт первого суперпользователя (роль admin) с паролем из окружения, если
// его ещё нет. Повторный запуск безопасен: существующий суперпользователь не
// меняется, пароль ему не сбрасывается.
//
// Диалект определяется по DSN (database.DialectFromDSN) — как у cmd/migrate.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/botbooker/bb-core/internal/config"
	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/database/postgres"
	"github.com/botbooker/bb-core/internal/database/sqlite"
	"github.com/botbooker/bb-core/internal/repository"
	"github.com/botbooker/bb-core/internal/service"
)

// osExit — обёртка над os.Exit: переменная, а не прямой вызов, чтобы main
// можно было проверить в тестах, не завершая процесс.
var osExit = os.Exit

func main() {
	if code := Main(); code != 0 {
		osExit(code)
	}
}

// Main выполняет bootstrap и возвращает код возврата процесса.
// Вынесена из main, чтобы её можно было тестировать напрямую.
func Main() int {
	if err := run(context.Background(), *config.DBConfig(), *config.Bootstrap()); err != nil {
		slog.Error("bootstrap failed", "error", err)

		return 1
	}

	return 0
}

// run применяет миграции и создаёт первого суперпользователя.
func run(ctx context.Context, dbCfg config.DatabaseConfig, superuser config.BootstrapConfig) error {
	dialect, err := database.DialectFromDSN(dbCfg.DSN)
	if err != nil {
		return fmt.Errorf("detect dialect: %w", err)
	}

	db, err := openDatabase(ctx, dbCfg, dialect)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	slog.Info("applying migrations", "dialect", dialect)

	if err := database.Migrate(ctx, db, dialect); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	svc := service.NewBootstrapService(
		repository.NewUserRepo(db, dialect),
		repository.NewCredentialRepo(db, dialect),
	)

	user, created, err := svc.EnsureSuperuser(ctx, service.BootstrapInput{
		Email:     superuser.Email,
		Password:  superuser.Password,
		FirstName: superuser.FirstName,
		LastName:  superuser.LastName,
	})
	if err != nil {
		return fmt.Errorf("ensure superuser: %w", err)
	}

	slog.Info("bootstrap finished", "email", user.Email, "uuid", user.ID, "created", created)

	return nil
}

// openDatabase открывает подключение драйвером диалекта.
//
// Выбор драйвера повторяет internal/servers, но без HTTP-слоя: CLI выполняет
// свою задачу отдельным процессом и не должен тянуть транспортные зависимости.
func openDatabase(ctx context.Context, cfg config.DatabaseConfig, dialect string) (database.DB, error) {
	switch dialect {
	case database.DialectPostgres:
		return postgres.New(ctx, cfg)
	case database.DialectSQLite:
		return sqlite.New(ctx, cfg)
	default:
		return nil, fmt.Errorf("%w: %q", database.ErrUnsupportedDialect, dialect)
	}
}
