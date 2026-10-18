package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"strings"

	"github.com/pressly/goose/v3"
)

// Подкаталоги каталога миграций: у каждого диалекта свой SQL.
const (
	migrationsPostgresDir = "postgres"
	migrationsSQLiteDir   = "sqlite"
)

// ErrMigrationsUnavailable возвращается, если подключение не отдаёт *sql.DB:
// без него goose работать не умеет (он использует database/sql напрямую).
var ErrMigrationsUnavailable = errors.New("migrations require a connection over *sql.DB")

// ErrMigrationsDirEmpty возвращается, если каталог миграций не задан.
var ErrMigrationsDirEmpty = errors.New("migrations directory is not set")

// stdDBProvider — подключение, которое умеет отдать пул database/sql.
type stdDBProvider interface {
	StdDB() *sql.DB
}

// Migrate применяет все неприменённые миграции диалекта к подключению.
//
// migrationsDir — каталог с SQL-файлами: внутри него у каждого диалекта свой
// подкаталог (postgres/ или sqlite/). Файлы читаются с диска, а не встроены в
// бинарник: при развёртывании каталог копируется в образ (см. Dockerfile), а
// путь задаётся MIGRATIONS_DIR. Миграции версионируются goose; повторный вызов
// ничего не делает: применённые версии записаны в служебной таблице goose.
func Migrate(ctx context.Context, db DB, dialect, migrationsDir string) error {
	provider, err := newMigrationProvider(db, dialect, migrationsDir)
	if err != nil {
		return err
	}

	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	for _, result := range results {
		slog.InfoContext(ctx, "migration applied",
			"version", result.Source.Version,
			"name", result.Source.Path,
			"duration", result.Duration,
		)
	}

	return nil
}

// MigrateDown откатывает последнюю применённую миграцию.
//
// migrationsDir — тот же каталог SQL-файлов, что и у Migrate. Основной сценарий
// — ручной откат при разработке и тесты. Если откатывать нечего, ошибка не
// возвращается: состояние уже соответствует ожидаемому.
func MigrateDown(ctx context.Context, db DB, dialect, migrationsDir string) error {
	provider, err := newMigrationProvider(db, dialect, migrationsDir)
	if err != nil {
		return err
	}

	result, err := provider.Down(ctx)
	if err != nil {
		if errors.Is(err, goose.ErrNoNextVersion) {
			slog.DebugContext(ctx, "no migrations to roll back")

			return nil
		}

		return fmt.Errorf("roll back migration: %w", err)
	}

	slog.InfoContext(ctx, "migration rolled back",
		"version", result.Source.Version,
		"name", result.Source.Path,
		"duration", result.Duration,
	)

	return nil
}

// newMigrationProvider собирает goose-провайдер миграций для диалекта.
func newMigrationProvider(db DB, dialect, migrationsDir string) (*goose.Provider, error) {
	if strings.TrimSpace(migrationsDir) == "" {
		return nil, ErrMigrationsDirEmpty
	}

	stdDB, dir, gooseDialect, err := migrationTarget(db, dialect)
	if err != nil {
		return nil, err
	}

	// Каталог диалекта — подкаталог каталога миграций: у goose свой набор файлов.
	sub, err := fs.Sub(os.DirFS(migrationsDir), dir)
	if err != nil {
		return nil, fmt.Errorf("open migrations dir %q: %w", dir, err)
	}

	provider, err := goose.NewProvider(gooseDialect, stdDB, sub, goose.WithSlog(slog.Default()))
	if err != nil {
		return nil, fmt.Errorf("create migration provider from %q: %w", migrationsDir, err)
	}

	return provider, nil
}

// migrationTarget подбирает для диалекта пул database/sql, каталог миграций
// и диалект goose.
func migrationTarget(db DB, dialect string) (*sql.DB, string, goose.Dialect, error) {
	conn, ok := db.(stdDBProvider)
	if !ok {
		return nil, "", "", fmt.Errorf("%w: %T", ErrMigrationsUnavailable, db)
	}

	stdDB := conn.StdDB()
	if stdDB == nil {
		return nil, "", "", fmt.Errorf("%w: %T", ErrMigrationsUnavailable, db)
	}

	switch dialect {
	case DialectPostgres:
		return stdDB, migrationsPostgresDir, goose.DialectPostgres, nil
	case DialectSQLite:
		return stdDB, migrationsSQLiteDir, goose.DialectSQLite3, nil
	default:
		return nil, "", "", fmt.Errorf("%w: %q", ErrUnsupportedDialect, dialect)
	}
}
