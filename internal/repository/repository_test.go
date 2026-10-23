package repository

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/botbooker/bb-core/internal/config"
	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/database/postgres"
	"github.com/botbooker/bb-core/internal/database/sqlite"
)

// testMigrationsDir — каталог с SQL-миграциями относительно пакета: go test
// запускает тесты с рабочим каталогом пакета.
const testMigrationsDir = "../../migrations"

// newSQLiteTestDB открывает SQLite во временном каталоге и накатывает миграции
// теми же файлами, что и приложение: в тестах участвует настоящая схема.
func newSQLiteTestDB(t *testing.T) database.DB {
	t.Helper()

	ctx := context.Background()

	conn, err := sqlite.New(ctx, config.DatabaseConfig{
		DSN:          filepath.Join(t.TempDir(), "database.db"),
		MaxDBConnect: 1,
	})
	if err != nil {
		t.Fatalf("sqlite.New() unexpected error = %v", err)
	}

	if err := database.Migrate(ctx, conn, database.DialectSQLite, testMigrationsDir); err != nil {
		_ = conn.Close()

		t.Fatalf("database.Migrate() unexpected error = %v", err)
	}

	t.Cleanup(func() {
		if closeErr := conn.Close(); closeErr != nil {
			t.Errorf("Close() unexpected error = %v", closeErr)
		}
	})

	return conn
}

// newPostgresTestDB подключается к базе PostgreSQL, заданной переменной DSN_TEST,
// в отдельной схеме: таблицы приложения в public не трогаются.
//
// Без DSN_TEST тест пропускается: интеграционные проверки ходят только в базу,
// которую явно указали для тестов.
func newPostgresTestDB(t *testing.T) database.DB {
	t.Helper()

	dsn := os.Getenv("DSN_TEST")
	if dsn == "" {
		t.Skip("DSN_TEST не задан: интеграционная проверка репозитория на PostgreSQL пропущена")
	}

	ctx := context.Background()

	const schema = "bb_repo_test"

	conn, err := postgres.New(ctx, config.DatabaseConfig{
		DSN:          withSearchPath(dsn, schema),
		MaxDBConnect: 1,
	})
	if err != nil {
		t.Fatalf("postgres.New() unexpected error = %v", err)
	}

	t.Cleanup(func() {
		if _, dropErr := conn.ExecContext(ctx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE"); dropErr != nil {
			t.Errorf("DROP SCHEMA unexpected error = %v", dropErr)
		}

		if closeErr := conn.Close(); closeErr != nil {
			t.Errorf("Close() unexpected error = %v", closeErr)
		}
	})

	if _, err := conn.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS "+schema); err != nil {
		t.Fatalf("CREATE SCHEMA unexpected error = %v", err)
	}

	if err := database.Migrate(ctx, conn, database.DialectPostgres, testMigrationsDir); err != nil {
		t.Fatalf("database.Migrate() unexpected error = %v", err)
	}

	return conn
}

// withSearchPath дописывает search_path к DSN: тест работает в отдельной схеме.
func withSearchPath(dsn, schema string) string {
	if strings.Contains(dsn, "?") {
		return dsn + "&search_path=" + schema
	}

	return dsn + "?search_path=" + schema
}

// assertJSONEqual сравнивает JSON-значения по содержимому, а не по строке:
// PostgreSQL хранит JSONB в нормализованном виде и возвращает его с пробелами
// после двоеточий, тогда как SQLite отдаёт исходный текст.
func assertJSONEqual(t *testing.T, got []byte, want string) {
	t.Helper()

	var gotValue, wantValue any

	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("json.Unmarshal(got = %q) unexpected error = %v", got, err)
	}

	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("json.Unmarshal(want = %q) unexpected error = %v", want, err)
	}

	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Errorf("JSON = %s, want %s", got, want)
	}
}
