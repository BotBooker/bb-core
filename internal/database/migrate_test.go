// Тесты миграций ходят в настоящие базы, поэтому лежат во внешнем пакете:
// так тест может импортировать драйверы, которые зависят от internal/database.
package database_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/botbooker/bb-core/internal/config"
	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/database/postgres"
	"github.com/botbooker/bb-core/internal/database/sqlite"
	"github.com/botbooker/bb-core/migrations"
)

// wantUserColumns — набор колонок таблицы users: одинаковый в обоих диалектах.
var wantUserColumns = []string{
	"id", "email", "first_name", "last_name", "middle_name",
	"phone_number", "gender", "birth_date", "created_at", "updated_at",
	"last_login_at", "is_active", "is_delete",
}

// fakePool — пул без настоящего database/sql: проверяет отказ миграций
// на подключении, которое не умеет отдать *sql.DB.
type fakePool struct{}

var _ database.SQLDB = fakePool{}

func (fakePool) BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error) { return nil, nil }

func (fakePool) QueryContext(context.Context, string, ...any) (*sql.Rows, error) { return nil, nil }

func (fakePool) QueryRowContext(context.Context, string, ...any) *sql.Row { return nil }

func (fakePool) ExecContext(context.Context, string, ...any) (sql.Result, error) { return nil, nil }

func (fakePool) PingContext(context.Context) error { return nil }

func (fakePool) Close() error { return nil }

// newSQLiteConnection открывает подключение к файловой базе во временном каталоге.
func newSQLiteConnection(t *testing.T) database.DB {
	t.Helper()

	dsn := filepath.Join(t.TempDir(), "database.db")

	conn, err := sqlite.New(context.Background(), config.DatabaseConfig{DSN: dsn, MaxDBConnect: 1})
	if err != nil {
		t.Fatalf("sqlite.New() unexpected error = %v", err)
	}

	t.Cleanup(func() { _ = conn.Close() })

	return conn
}

// countRows проверяет, что таблица существует, и возвращает число строк в ней.
func countRows(t *testing.T, ctx context.Context, conn database.DB, table string) int {
	t.Helper()

	var count int

	// Имя таблицы подставляет тест, пользовательский ввод сюда не попадает.
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
		t.Fatalf("SELECT COUNT(*) FROM %s: %v", table, err)
	}

	return count
}

// columns возвращает колонки таблицы users в порядке объявления.
func columns(t *testing.T, ctx context.Context, conn database.DB, dialect string) []string {
	t.Helper()

	if dialect == database.DialectPostgres {
		return postgresColumns(t, ctx, conn)
	}

	return sqliteColumns(t, ctx, conn)
}

func sqliteColumns(t *testing.T, ctx context.Context, conn database.DB) []string {
	t.Helper()

	rows, err := conn.QueryContext(ctx, "PRAGMA table_info(users)")
	if err != nil {
		t.Fatalf("PRAGMA table_info(users): %v", err)
	}
	defer rows.Close()

	var result []string

	for rows.Next() {
		var (
			cid, notNull, pk int
			name, columnType string
			defaultValue     sql.NullString
		)

		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}

		result = append(result, name)
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err() = %v", err)
	}

	return result
}

func postgresColumns(t *testing.T, ctx context.Context, conn database.DB) []string {
	t.Helper()

	rows, err := conn.QueryContext(ctx, `
		SELECT column_name
		FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'users'
		ORDER BY ordinal_position`)
	if err != nil {
		t.Fatalf("information_schema.columns: %v", err)
	}
	defer rows.Close()

	var result []string

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan column_name: %v", err)
		}

		result = append(result, name)
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err() = %v", err)
	}

	return result
}

// indexNames возвращает индексы таблицы users: в SQLite — по pragma_index_list,
// в PostgreSQL — по pg_indexes.
func indexNames(t *testing.T, ctx context.Context, conn database.DB, dialect string) []string {
	t.Helper()

	query := "SELECT name FROM pragma_index_list('users')"
	if dialect == database.DialectPostgres {
		query = `SELECT indexname FROM pg_indexes
			WHERE schemaname = current_schema() AND tablename = 'users'`
	}

	rows, err := conn.QueryContext(ctx, query)
	if err != nil {
		t.Fatalf("индексы users: %v", err)
	}
	defer rows.Close()

	var result []string

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan индекса: %v", err)
		}

		result = append(result, name)
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err() = %v", err)
	}

	return result
}

// indexDef возвращает DDL индекса таблицы users: в SQLite — из sqlite_master,
// в PostgreSQL — из pg_indexes.
func indexDef(t *testing.T, ctx context.Context, conn database.DB, dialect, name string) string {
	t.Helper()

	query := "SELECT sql FROM sqlite_master WHERE type = 'index' AND name = ?"
	if dialect == database.DialectPostgres {
		query = `SELECT indexdef FROM pg_indexes
			WHERE schemaname = current_schema() AND tablename = 'users' AND indexname = $1`
	}

	var def sql.NullString
	if err := conn.QueryRowContext(ctx, query, name).Scan(&def); err != nil {
		t.Fatalf("DDL индекса %s: %v", name, err)
	}

	return def.String
}

// assertUniqueIndex требует, чтобы индекс был уникальным, а при partial —
// ещё и частичным (с условием WHERE).
func assertUniqueIndex(t *testing.T, ctx context.Context, conn database.DB, dialect, name string, partial bool) {
	t.Helper()

	def := indexDef(t, ctx, conn, dialect, name)
	if !strings.Contains(def, "UNIQUE") {
		t.Errorf("DDL индекса %s = %q, want уникальный индекс", name, def)
	}

	if partial && !strings.Contains(def, "WHERE") {
		t.Errorf("DDL индекса %s = %q, want частичный индекс (WHERE ...)", name, def)
	}
}

// insertUser вставляет пользователя минимальным набором полей:
// created_at и updated_at обязательны, остальное берётся из DEFAULT.
func insertUser(ctx context.Context, conn database.DB, dialect, id, email string) error {
	query := "INSERT INTO users (id, email, first_name, last_name, created_at, updated_at)" +
		" VALUES (?, ?, ?, ?, ?, ?)"
	if dialect == database.DialectPostgres {
		query = "INSERT INTO users (id, email, first_name, last_name, created_at, updated_at)" +
			" VALUES ($1, $2, $3, $4, $5, $6)"
	}

	now := time.Now().UTC()

	_, err := conn.ExecContext(ctx, query, id, email, "Иван", "Иванов", now, now)

	return err
}

// insertInactiveUser вставляет soft-deleted пользователя (is_active = false):
// email такой записи не считается занятым — индекс users_email_idx частичный.
func insertInactiveUser(ctx context.Context, conn database.DB, dialect, id, email string) error {
	query := "INSERT INTO users (id, email, first_name, last_name, is_active, created_at, updated_at)" +
		" VALUES (?, ?, ?, ?, ?, ?, ?)"
	if dialect == database.DialectPostgres {
		query = "INSERT INTO users (id, email, first_name, last_name, is_active, created_at, updated_at)" +
			" VALUES ($1, $2, $3, $4, $5, $6, $7)"
	}

	now := time.Now().UTC()

	_, err := conn.ExecContext(ctx, query, id, email, "Иван", "Иванов", false, now, now)

	return err
}

// insertUserWithPhone вставляет пользователя с телефоном: пустая строка
// означает «телефон не задан» (значение по умолчанию).
func insertUserWithPhone(ctx context.Context, conn database.DB, dialect, id, email, phone string) error {
	query := "INSERT INTO users (id, email, first_name, last_name, phone_number, created_at, updated_at)" +
		" VALUES (?, ?, ?, ?, ?, ?, ?)"
	if dialect == database.DialectPostgres {
		query = "INSERT INTO users (id, email, first_name, last_name, phone_number, created_at, updated_at)" +
			" VALUES ($1, $2, $3, $4, $5, $6, $7)"
	}

	now := time.Now().UTC()

	_, err := conn.ExecContext(ctx, query, id, email, "Иван", "Иванов", phone, now, now)

	return err
}

// insertPhoneUser вставляет пользователя с телефоном и роняет тест, если вставка
// не прошла: для случаев, когда она обязана пройти.
func insertPhoneUser(t *testing.T, ctx context.Context, conn database.DB, dialect, id, email, phone string) {
	t.Helper()

	if err := insertUserWithPhone(ctx, conn, dialect, id, email, phone); err != nil {
		t.Fatalf("insertUserWithPhone(%q, %q) unexpected error = %v", email, phone, err)
	}
}

func TestMigrateSQLite(t *testing.T) {
	ctx := context.Background()
	conn := newSQLiteConnection(t)

	if err := database.Migrate(ctx, conn, database.DialectSQLite); err != nil {
		t.Fatalf("Migrate() unexpected error = %v", err)
	}

	if got := countRows(t, ctx, conn, "users"); got != 0 {
		t.Errorf("users rows = %d, want 0", got)
	}

	if got := columns(t, ctx, conn, database.DialectSQLite); !slices.Equal(got, wantUserColumns) {
		t.Errorf("колонки users = %v, want %v", got, wantUserColumns)
	}

	t.Run("индексы", func(t *testing.T) {
		// Оба индекса уникальные и частичные: users_email_idx держит уникальность
		// email только среди активных строк (WHERE is_active), users_phone_number_idx —
		// только для заданных телефонов. Ещё в списке будет sqlite_autoindex_users_1:
		// SQLite сам создаёт индекс под TEXT PRIMARY KEY.
		got := indexNames(t, ctx, conn, database.DialectSQLite)

		for _, name := range []string{"users_email_idx", "users_phone_number_idx"} {
			if !slices.Contains(got, name) {
				t.Errorf("индексы users = %v, want %s", got, name)
			}
		}

		assertUniqueIndex(t, ctx, conn, database.DialectSQLite, "users_email_idx", true)
		assertUniqueIndex(t, ctx, conn, database.DialectSQLite, "users_phone_number_idx", true)
	})

	// Повторный запуск ничего не делает: версии уже применены.
	if err := database.Migrate(ctx, conn, database.DialectSQLite); err != nil {
		t.Fatalf("повторный Migrate() unexpected error = %v", err)
	}

	if err := insertUser(ctx, conn, database.DialectSQLite,
		"11111111-1111-4111-8111-111111111111", "ivan@example.com"); err != nil {
		t.Fatalf("insertUser() unexpected error = %v", err)
	}

	var gender, phone, middle string
	var isActive, isDelete int

	err := conn.QueryRowContext(ctx,
		"SELECT gender, phone_number, middle_name, is_active, is_delete FROM users WHERE email = ?", "ivan@example.com").
		Scan(&gender, &phone, &middle, &isActive, &isDelete)
	if err != nil {
		t.Fatalf("select значений по умолчанию: %v", err)
	}

	if gender != "unspecified" || phone != "" || middle != "" || isActive != 1 || isDelete != 0 {
		t.Errorf("значения по умолчанию = %q/%q/%q/%d/%d, want unspecified/''/''/1/0",
			gender, phone, middle, isActive, isDelete)
	}

	t.Run("уникальность email", func(t *testing.T) {
		err := insertUser(ctx, conn, database.DialectSQLite,
			"22222222-2222-4222-8222-222222222222", "ivan@example.com")
		if err == nil {
			t.Error("insert дубликата email: ошибки нет, want нарушение UNIQUE")
		}
	})

	t.Run("частичная уникальность email", func(t *testing.T) {
		// Soft-deleted записи email не занимают: у адреса может быть сколько
		// угодно удалённых строк и ровно одна активная — индекс частичный.
		for _, id := range []string{
			"aaaa1111-1111-4111-8111-111111111111",
			"bbbb2222-2222-4222-8222-222222222222",
		} {
			if err := insertInactiveUser(ctx, conn, database.DialectSQLite, id, "reuse@example.com"); err != nil {
				t.Fatalf("insertInactiveUser() unexpected error = %v", err)
			}
		}

		if err := insertUser(ctx, conn, database.DialectSQLite,
			"cccc3333-3333-4333-8333-333333333333", "reuse@example.com"); err != nil {
			t.Fatalf("insert активного email после удалённых: %v", err)
		}

		err := insertUser(ctx, conn, database.DialectSQLite,
			"dddd4444-4444-4444-8444-444444444444", "reuse@example.com")
		if err == nil {
			t.Error("insert второго активного email: ошибки нет, want нарушение UNIQUE")
		}
	})

	t.Run("уникальность id", func(t *testing.T) {
		err := insertUser(ctx, conn, database.DialectSQLite,
			"11111111-1111-4111-8111-111111111111", "petr2@example.com")
		if err == nil {
			t.Error("insert дубликата id: ошибки нет, want нарушение UNIQUE")
		}
	})

	t.Run("уникальность телефона", func(t *testing.T) {
		// Пустой телефон — это «телефон не задан»: пользователей без телефона
		// может быть сколько угодно (первый вставлен выше через DEFAULT).
		insertPhoneUser(t, ctx, conn, database.DialectSQLite,
			"55555555-5555-4555-8555-555555555555", "no-phone@example.com", "")

		insertPhoneUser(t, ctx, conn, database.DialectSQLite,
			"66666666-6666-4666-8666-666666666666", "phone1@example.com", "+79990000001")

		err := insertUserWithPhone(ctx, conn, database.DialectSQLite,
			"77777777-7777-4777-8777-777777777777", "phone2@example.com", "+79990000001")
		if err == nil {
			t.Error("insert дубликата телефона: ошибки нет, want нарушение уникальности")
		}
	})

	t.Run("CHECK пола", func(t *testing.T) {
		_, err := conn.ExecContext(ctx,
			"INSERT INTO users (id, email, first_name, last_name, gender, created_at, updated_at)"+
				" VALUES (?, ?, ?, ?, ?, ?, ?)",
			"33333333-3333-4333-8333-333333333333", "petr@example.com", "Пётр", "Петров", "wat",
			time.Now().UTC(), time.Now().UTC())
		if err == nil {
			t.Error("insert неизвестного gender: ошибки нет, want нарушение CHECK")
		}
	})

	t.Run("NOT NULL email", func(t *testing.T) {
		_, err := conn.ExecContext(ctx,
			"INSERT INTO users (id, first_name, last_name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
			"44444444-4444-4444-8444-444444444444", "Анна", "Аннова", time.Now().UTC(), time.Now().UTC())
		if err == nil {
			t.Error("insert без email: ошибки нет, want нарушение NOT NULL")
		}
	})

	// Каждый откат снимает ровно одну миграцию — с последней (00007_notifications)
	// до первой (00001_foundation). Таблица, которую миграция создавала, должна
	// исчезнуть вместе с ней.
	var count int

	for _, tc := range []struct {
		version string
		table   string
	}{
		{"00007_notifications", "notifications"},
		{"00006_bookings", "bookings"},
		{"00005_objects", "rental_objects"},
		{"00004_auth", "user_tokens"},
		{"00003_tenants", "tenant_members"},
		{"00002_users", "users"},
		{"00001_foundation", "roles"},
	} {
		if err := database.MigrateDown(ctx, conn, database.DialectSQLite); err != nil {
			t.Fatalf("MigrateDown(%s) unexpected error = %v", tc.version, err)
		}

		if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+tc.table).Scan(&count); err == nil {
			t.Errorf("SELECT %s после отката %s: ошибки нет, want отсутствие таблицы", tc.table, tc.version)
		}
	}

	// Откат, когда миграций не осталось, ошибкой не считается.
	if err := database.MigrateDown(ctx, conn, database.DialectSQLite); err != nil {
		t.Fatalf("MigrateDown() без миграций unexpected error = %v", err)
	}
}

func TestMigrateUnsupportedDialect(t *testing.T) {
	conn := newSQLiteConnection(t)

	err := database.Migrate(context.Background(), conn, "mysql")
	if !errors.Is(err, database.ErrUnsupportedDialect) {
		t.Fatalf("Migrate() error = %v, want %v", err, database.ErrUnsupportedDialect)
	}
}

func TestMigrateWithoutStdDB(t *testing.T) {
	conn, err := database.New(fakePool{})
	if err != nil {
		t.Fatalf("database.New() unexpected error = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	err = database.Migrate(context.Background(), conn, database.DialectSQLite)
	if !errors.Is(err, database.ErrMigrationsUnavailable) {
		t.Fatalf("Migrate() error = %v, want %v", err, database.ErrMigrationsUnavailable)
	}
}

// TestMigrationFiles проверяет goose-конвенцию: один файл на версию, в файле
// есть обе секции (Up и Down), а набор версий одинаков у обоих диалектов.
//
// Пары 00001_x.up.sql + 00001_x.down.sql goose не поддерживает: он считает
// каждый файл отдельной миграцией и падает на дубликате версии.
func TestMigrationFiles(t *testing.T) {
	versionSets := map[string][]string{}

	for _, dir := range []string{"postgres", "sqlite"} {
		entries, err := fs.ReadDir(migrations.FS, dir)
		if err != nil {
			t.Fatalf("fs.ReadDir(%s): %v", dir, err)
		}

		versions := make([]string, 0, len(entries))

		for _, entry := range entries {
			name := entry.Name()

			version, err := gooseVersion(name)
			if err != nil {
				t.Errorf("%s/%s: %v", dir, name, err)

				continue
			}

			if slices.Contains(versions, version) {
				t.Errorf("%s/%s: версия %s уже встречалась", dir, name, version)
			}

			versions = append(versions, version)

			content, err := fs.ReadFile(migrations.FS, dir+"/"+name)
			if err != nil {
				t.Fatalf("fs.ReadFile(%s/%s): %v", dir, name, err)
			}

			for _, section := range []string{"-- +goose Up", "-- +goose Down"} {
				if !strings.Contains(string(content), section) {
					t.Errorf("%s/%s: нет секции %q", dir, name, section)
				}
			}
		}

		if len(versions) == 0 {
			t.Fatalf("%s: нет ни одной миграции", dir)
		}

		sort.Strings(versions)

		versionSets[dir] = versions
	}

	if !slices.Equal(versionSets["postgres"], versionSets["sqlite"]) {
		t.Errorf("наборы версий различаются: postgres=%v, sqlite=%v",
			versionSets["postgres"], versionSets["sqlite"])
	}
}

// gooseVersion проверяет имя файла миграции и возвращает её версию.
func gooseVersion(name string) (string, error) {
	if !strings.HasSuffix(name, ".sql") {
		return "", errors.New("имя миграции должно заканчиваться на .sql")
	}

	version, _, found := strings.Cut(strings.TrimSuffix(name, ".sql"), "_")
	if !found {
		return "", errors.New("в имени миграции нет разделителя '_'")
	}

	if _, err := strconv.ParseInt(version, 10, 64); err != nil {
		return "", fmt.Errorf("версия %q не является числом", version)
	}

	return version, nil
}

// TestMigratePostgres — интеграционный тест. Работает в отдельной схеме:
// DSN может указывать на общую базу (как у разработчика), поэтому таблицы
// приложения в public не трогаются, а тест остаётся повторяемым.
//
// Раньше тест дропал users и goose_db_version в current_schema — на DSN без
// поиска это стирало схему приложения.
func TestMigratePostgres(t *testing.T) {
	dsn := os.Getenv("PG")
	if dsn == "" {
		t.Skip("PG не задан: интеграционный тест миграций PostgreSQL пропущен")
	}

	ctx := context.Background()

	// Схема теста: создаётся до миграций, удаляется вместе с содержимым.
	const schema = "bb_migrate_test"

	conn, err := postgres.New(ctx, config.DatabaseConfig{
		DSN:          dsnWithSearchPath(dsn, schema),
		MaxDBConnect: 2,
	})
	if err != nil {
		t.Fatalf("postgres.New() unexpected error = %v", err)
	}

	t.Cleanup(func() { _ = conn.Close() })

	if _, err := conn.ExecContext(ctx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE"); err != nil {
		t.Fatalf("DROP SCHEMA IF EXISTS unexpected error = %v", err)
	}

	if _, err := conn.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("CREATE SCHEMA unexpected error = %v", err)
	}

	t.Cleanup(func() {
		if _, dropErr := conn.ExecContext(ctx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE"); dropErr != nil {
			t.Errorf("DROP SCHEMA unexpected error = %v", dropErr)
		}
	})

	if err := database.Migrate(ctx, conn, database.DialectPostgres); err != nil {
		t.Fatalf("Migrate() unexpected error = %v", err)
	}

	if got := countRows(t, ctx, conn, "users"); got != 0 {
		t.Errorf("users rows = %d, want 0", got)
	}

	if got := columns(t, ctx, conn, database.DialectPostgres); !slices.Equal(got, wantUserColumns) {
		t.Errorf("колонки users = %v, want %v", got, wantUserColumns)
	}

	// gender — нативный ENUM: тип создан миграцией и привязан к колонке.
	var genderType string
	if err := conn.QueryRowContext(ctx, `SELECT udt_name FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'users' AND column_name = 'gender'`).
		Scan(&genderType); err != nil {
		t.Fatalf("тип колонки gender: %v", err)
	}

	if genderType != "user_gender" {
		t.Errorf("тип gender = %q, want user_gender", genderType)
	}

	t.Run("индексы", func(t *testing.T) {
		got := indexNames(t, ctx, conn, database.DialectPostgres)

		for _, name := range []string{"users_email_idx", "users_phone_number_idx"} {
			if !slices.Contains(got, name) {
				t.Errorf("индексы users = %v, want %s", got, name)
			}
		}

		assertUniqueIndex(t, ctx, conn, database.DialectPostgres, "users_email_idx", true)
		assertUniqueIndex(t, ctx, conn, database.DialectPostgres, "users_phone_number_idx", true)
	})

	t.Run("уникальность телефона", func(t *testing.T) {
		// «Телефон не задан» может быть у многих пользователей: индекс частичный.
		insertPhoneUser(t, ctx, conn, database.DialectPostgres,
			"77777777-7777-4777-8777-777777777777", "no-phone@example.com", "")

		insertPhoneUser(t, ctx, conn, database.DialectPostgres,
			"88888888-8888-4888-8888-888888888888", "phone1@example.com", "+79990000001")

		err := insertUserWithPhone(ctx, conn, database.DialectPostgres,
			"99999999-9999-4999-8999-999999999999", "phone2@example.com", "+79990000001")
		if err == nil {
			t.Error("insert дубликата телефона: ошибки нет, want нарушение уникальности")
		}
	})

	t.Run("частичная уникальность email", func(t *testing.T) {
		// Soft-deleted записи email не занимают: частичный индекс (WHERE is_active)
		// допускает любое число удалённых строк и одну активную на адрес.
		for _, id := range []string{
			"aaaa1111-1111-4111-8111-111111111111",
			"bbbb2222-2222-4222-8222-222222222222",
		} {
			if err := insertInactiveUser(ctx, conn, database.DialectPostgres, id, "reuse@example.com"); err != nil {
				t.Fatalf("insertInactiveUser() unexpected error = %v", err)
			}
		}

		if err := insertUser(ctx, conn, database.DialectPostgres,
			"cccc3333-3333-4333-8333-333333333333", "reuse@example.com"); err != nil {
			t.Fatalf("insert активного email после удалённых: %v", err)
		}

		err := insertUser(ctx, conn, database.DialectPostgres,
			"dddd4444-4444-4444-8444-444444444444", "reuse@example.com")
		if err == nil {
			t.Error("insert второго активного email: ошибки нет, want нарушение UNIQUE")
		}
	})

	if err := insertUser(ctx, conn, database.DialectPostgres,
		"11111111-1111-4111-8111-111111111111", "ivan@example.com"); err != nil {
		t.Fatalf("insertUser() unexpected error = %v", err)
	}

	// Ограничения схемы работают и в PostgreSQL.
	err = insertUser(ctx, conn, database.DialectPostgres,
		"22222222-2222-4222-8222-222222222222", "ivan@example.com")
	if err == nil {
		t.Error("insert дубликата email: ошибки нет, want нарушение UNIQUE")
	}

	// Значение вне ENUM и не-UUID отклоняет сама СУБД (SQLSTATE 22P02).
	_, err = conn.ExecContext(ctx,
		"INSERT INTO users (id, email, first_name, last_name, gender, created_at, updated_at)"+
			" VALUES ($1, $2, $3, $4, $5, $6, $7)",
		"44444444-4444-4444-8444-444444444444", "anna@example.com", "Анна", "Аннова", "wat",
		time.Now().UTC(), time.Now().UTC())
	if err == nil {
		t.Error("insert неизвестного gender: ошибки нет, want нарушение ENUM")
	}

	if err := insertUser(ctx, conn, database.DialectPostgres, "not-a-uuid", "nikita@example.com"); err == nil {
		t.Error("insert не-UUID в id: ошибки нет, want нарушение типа uuid")
	}

	// Каждый откат снимает ровно одну миграцию — с 00007_notifications до
	// 00001_foundation. Таблица, которую миграция создавала, исчезает вместе с ней.
	var count int

	for _, tc := range []struct {
		version string
		table   string
	}{
		{"00007_notifications", "notifications"},
		{"00006_bookings", "bookings"},
		{"00005_objects", "rental_objects"},
		{"00004_auth", "user_tokens"},
		{"00003_tenants", "tenant_members"},
		{"00002_users", "users"},
		{"00001_foundation", "roles"},
	} {
		if err := database.MigrateDown(ctx, conn, database.DialectPostgres); err != nil {
			t.Fatalf("MigrateDown(%s) unexpected error = %v", tc.version, err)
		}

		if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+tc.table).Scan(&count); err == nil {
			t.Errorf("SELECT %s после отката %s: ошибки нет, want отсутствие таблицы", tc.table, tc.version)
		}
	}

	// Откат 00002_users убирает и собственный тип ENUM: тип в PostgreSQL живёт
	// независимо от таблицы, поэтому повторная раскатка должна создавать его
	// заново. to_regtype ищет тип по search_path — то есть в схеме теста, а не в public.
	var genderTypeMissing bool
	if err := conn.QueryRowContext(ctx, "SELECT to_regtype('user_gender') IS NULL").Scan(&genderTypeMissing); err != nil {
		t.Fatalf("поиск типа user_gender: %v", err)
	}

	if !genderTypeMissing {
		t.Error("после отката тип user_gender ещё существует, want удаление типом миграции")
	}
}

// dsnWithSearchPath дописывает search_path к DSN: тест работает в своей схеме
// и не задевает таблицы приложения в public.
func dsnWithSearchPath(dsn, schema string) string {
	if strings.Contains(dsn, "?") {
		return dsn + "&search_path=" + schema
	}

	return dsn + "?search_path=" + schema
}
