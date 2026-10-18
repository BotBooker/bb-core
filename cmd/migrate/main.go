// Command migrate применяет и откатывает миграции вручную:
//
//	DSN=/tmp/database.db go run ./cmd/migrate up
//	DSN=postgres://user:pass@localhost:5432/db?sslmode=disable go run ./cmd/migrate down
//
// Диалект определяется по DSN (database.DialectFromDSN), SQL-файлы читаются из
// каталога MIGRATIONS_DIR (по умолчанию ./migrations) — это тот же код, что и
// авто-применение при старте приложения (MIGRATE_ON_START), поэтому ручная и
// автоматическая раскатка не расходятся.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/botbooker/bb-core/internal/config"
	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/database/postgres"
	"github.com/botbooker/bb-core/internal/database/sqlite"
)

// Команды CLI: up применяет все неприменённые миграции,
// down откатывает последнюю применённую.
const (
	commandUp   = "up"
	commandDown = "down"
)

// errUsage возвращается при неизвестной команде.
var errUsage = errors.New("usage: migrate <up|down>")

// osExit — обёртка над os.Exit: переменная, а не прямой вызов,
// чтобы main можно было проверить в тестах, не завершая процесс.
var osExit = os.Exit

func main() {
	if code := Main(); code != 0 {
		osExit(code)
	}
}

// Main выполняет команду и возвращает код возврата процесса.
func Main() int {
	if err := run(context.Background(), *config.DBConfig(), os.Args[1:]); err != nil {
		slog.ErrorContext(context.Background(), "migrations failed", "error", err)

		return 1
	}

	return 0
}

// run выполняет команду CLI с заданной конфигурацией подключения.
func run(ctx context.Context, cfg config.DatabaseConfig, args []string) error {
	command, err := parseCommand(args)
	if err != nil {
		return err
	}

	dialect, err := database.DialectFromDSN(cfg.DSN)
	if err != nil {
		return fmt.Errorf("detect dialect: %w", err)
	}

	db, err := openDatabase(ctx, cfg, dialect)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	slog.InfoContext(ctx, "running migrations", "command", command, "dialect", dialect)

	if command == commandDown {
		return database.MigrateDown(ctx, db, dialect, cfg.MigrationsDir)
	}

	return database.Migrate(ctx, db, dialect, cfg.MigrationsDir)
}

// parseCommand разбирает команду CLI: без аргументов выполняется up.
//
// Лишние аргументы — ошибка, а не молчаливое игнорирование: иначе опечатка вида
// "migrate down DSN=..." незаметно выполнится для DSN из окружения.
func parseCommand(args []string) (string, error) {
	if len(args) == 0 {
		return commandUp, nil
	}

	if command := args[0]; len(args) == 1 && (command == commandUp || command == commandDown) {
		return command, nil
	}

	return "", fmt.Errorf("%w: got %q", errUsage, args)
}

// openDatabase открывает подключение драйвером диалекта.
//
// Выбор драйвера повторяет internal/servers, но без HTTP-слоя: CLI раскатывает
// миграции отдельным процессом и не должен тянуть транспортные зависимости.
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
