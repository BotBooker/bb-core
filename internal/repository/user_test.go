package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/model"
)

// mockUserDB is a mock implementation of UserDB for testing.
type mockUserDB struct {
	queryFunc    func(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	queryRowFunc func(ctx context.Context, query string, args ...any) *sql.Row
	execFunc     func(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (m *mockUserDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, query, args...)
	}
	return nil, nil
}

func (m *mockUserDB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	if m.queryRowFunc != nil {
		return m.queryRowFunc(ctx, query, args...)
	}
	return nil
}

func (m *mockUserDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if m.execFunc != nil {
		return m.execFunc(ctx, query, args...)
	}
	return nil, nil
}

func TestNewUserRepo(t *testing.T) {
	tests := []struct {
		name    string
		db      UserDB
		dialect string
	}{
		{name: "valid db", db: &mockUserDB{}, dialect: database.DialectSQLite},
		{name: "postgres", db: &mockUserDB{}, dialect: database.DialectPostgres},
		{name: "nil db", db: nil, dialect: database.DialectSQLite},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewUserRepo(context.Background(), tt.db, tt.dialect); got == nil {
				t.Error("NewUserRepo() returned nil")
			}
		})
	}
}

func TestCodec_Rebind(t *testing.T) {
	const query = "SELECT a FROM t WHERE b = ? AND c = ?"

	tests := []struct {
		name    string
		dialect string
		want    string
	}{
		{name: "sqlite оставляет «?»", dialect: database.DialectSQLite, want: query},
		{
			name:    "postgres нумерует плейсхолдеры",
			dialect: database.DialectPostgres,
			want:    "SELECT a FROM t WHERE b = $1 AND c = $2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := codec{dialect: tt.dialect}

			if got := c.rebind(query); got != tt.want {
				t.Errorf("rebind() = %q, want %q", got, tt.want)
			}
		})
	}
}

// newTestUserRepo создаёт репозиторий поверх настоящей SQLite во временном
// каталоге: схема накатывается теми же миграциями, что и в приложении.
func newTestUserRepo(t *testing.T) UserRepo {
	t.Helper()

	return NewUserRepo(context.Background(), newSQLiteTestDB(t), database.DialectSQLite)
}

func TestUserRepoSQLite(t *testing.T) {
	runUserRepoScenario(t, newTestUserRepo(t))
}

// TestUserRepoPostgres проверяет репозиторий на PostgreSQL: у диалекта другие
// плейсхолдеры и типы времени (TIMESTAMPTZ и DATE вместо строк SQLite).
func TestUserRepoPostgres(t *testing.T) {
	runUserRepoScenario(t, NewUserRepo(context.Background(), newPostgresTestDB(t), database.DialectPostgres))
}

func TestUserRepo_DeleteQuery(t *testing.T) {
	const id = "22222222-2222-4222-8222-222222222222"

	tests := []struct {
		name    string
		dialect string
		want    string
	}{
		{
			name:    "sqlite",
			dialect: database.DialectSQLite,
			want: "UPDATE users SET is_active = false, is_delete = true" +
				" WHERE id = ? AND is_active = true",
		},
		{
			name:    "postgres",
			dialect: database.DialectPostgres,
			want: "UPDATE users SET is_active = false, is_delete = true" +
				" WHERE id = $1 AND is_active = true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				gotQuery string
				gotArgs  []any
			)

			db := &mockUserDB{execFunc: func(_ context.Context, query string, args ...any) (sql.Result, error) {
				gotQuery = query
				gotArgs = args

				return resultStub{affected: 1}, nil
			}}

			if err := NewUserRepo(context.Background(), db, tt.dialect).Delete(context.Background(), id); err != nil {
				t.Fatalf("Delete() unexpected error = %v", err)
			}

			if gotQuery != tt.want {
				t.Errorf("query = %q, want %q", gotQuery, tt.want)
			}

			if strings.Count(gotQuery, "?")+strings.Count(gotQuery, "$1") != 1 {
				t.Errorf("в запросе %q want ровно один плейсхолдер", gotQuery)
			}

			if len(gotArgs) != 1 || gotArgs[0] != id {
				t.Errorf("args = %v, want [%q]", gotArgs, id)
			}
		})
	}
}

// TestUserRepo_DeleteInvalidUUID проверяет, что невалидный идентификатор
// отклоняется до запроса: пользовательская строка не попадает в базу даже как
// аргумент, а возвращается ошибка валидации.
func TestUserRepo_DeleteInvalidUUID(t *testing.T) {
	db := &mockUserDB{execFunc: func(_ context.Context, _ string, _ ...any) (sql.Result, error) {
		t.Error("ExecContext вызван с невалидным uuid, want отказ до запроса")

		return resultStub{affected: 1}, nil
	}}

	err := NewUserRepo(context.Background(), db, database.DialectSQLite).Delete(context.Background(), "1'; DROP TABLE users; --")
	if !errors.Is(err, model.ErrInvalidUUID) {
		t.Errorf("Delete() error = %v, want %v", err, model.ErrInvalidUUID)
	}
}

// testUser собирает валидного пользователя со всеми заполненными полями:
// так проверяется и кодирование времени, и необязательные значения.
// Телефон передаётся явно: индекс по phone_number уникален, поэтому разные
// пользователи в тестах должны иметь разные номера.
func testUser(t *testing.T, email, phoneNumber string) *model.User {
	t.Helper()

	// Микросекунды — предел точности TIMESTAMPTZ: с ними результат одинаков
	// у обоих диалектов.
	now := time.Now().UTC().Truncate(time.Microsecond)

	birthDate, err := model.ParseDate("1990-05-17")
	if err != nil {
		t.Fatalf("model.ParseDate() unexpected error = %v", err)
	}

	user, err := model.NewUser(model.CreateUserInput{
		Email:       email,
		FirstName:   "Иван",
		LastName:    "Иванов",
		MiddleName:  "Иванович",
		PhoneNumber: phoneNumber,
		Gender:      model.GenderMale,
		BirthDate:   &birthDate,
	}, now)
	if err != nil {
		t.Fatalf("model.NewUser() unexpected error = %v", err)
	}

	user.SetLastLogin(now.Add(-time.Hour))

	return user
}

// assertUser сравнивает прочитанную запись с ожидаемой.
func assertUser(t *testing.T, want, got *model.User) {
	t.Helper()

	if got == nil {
		t.Fatal("пользователь не прочитан, want запись")
	}

	if got.ID != want.ID {
		t.Errorf("ID = %q, want %q", got.ID, want.ID)
	}
	if got.Email != want.Email {
		t.Errorf("Email = %q, want %q", got.Email, want.Email)
	}
	if got.FirstName != want.FirstName || got.LastName != want.LastName || got.MiddleName != want.MiddleName {
		t.Errorf("ФИО = %q %q %q, want %q %q %q",
			got.LastName, got.FirstName, got.MiddleName, want.LastName, want.FirstName, want.MiddleName)
	}
	if got.PhoneNumber != want.PhoneNumber {
		t.Errorf("PhoneNumber = %q, want %q", got.PhoneNumber, want.PhoneNumber)
	}
	if got.Gender != want.Gender {
		t.Errorf("Gender = %q, want %q", got.Gender, want.Gender)
	}
	// Роли у профиля нет: она принадлежит членству в тенанте.
	if got.IsDelete != want.IsDelete {
		t.Errorf("IsDelete = %v, want %v", got.IsDelete, want.IsDelete)
	}
	if got.BirthDate == nil || want.BirthDate == nil || got.BirthDate.String() != want.BirthDate.String() {
		t.Errorf("BirthDate = %v, want %v", got.BirthDate, want.BirthDate)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) || got.CreatedAt.Location() != time.UTC {
		t.Errorf("CreatedAt = %v, want %v (UTC)", got.CreatedAt, want.CreatedAt)
	}
	if !got.UpdatedAt.Equal(want.UpdatedAt) || got.UpdatedAt.Location() != time.UTC {
		t.Errorf("UpdatedAt = %v, want %v (UTC)", got.UpdatedAt, want.UpdatedAt)
	}
	if got.LastLoginAt == nil || want.LastLoginAt == nil || !got.LastLoginAt.Equal(*want.LastLoginAt) {
		t.Errorf("LastLoginAt = %v, want %v", got.LastLoginAt, want.LastLoginAt)
	}
	if got.IsActive != want.IsActive {
		t.Errorf("IsActive = %v, want %v", got.IsActive, want.IsActive)
	}
}

// runUserRepoScenario прогоняет один и тот же сценарий на любом диалекте:
// так SQLite и PostgreSQL проверяются одинаково.
func runUserRepoScenario(t *testing.T, repo UserRepo) {
	t.Helper()

	ctx := context.Background()

	t.Run("Create и GetByID", func(t *testing.T) {
		want := testUser(t, "ivan@example.com", "+79990000001")

		if err := repo.Create(ctx, want); err != nil {
			t.Fatalf("Create() unexpected error = %v", err)
		}

		got, err := repo.GetByID(ctx, want.ID)
		if err != nil {
			t.Fatalf("GetByID() unexpected error = %v", err)
		}

		assertUser(t, want, got)
	})

	t.Run("GetByEmail", func(t *testing.T) {
		want := testUser(t, "petr@example.com", "+79990000002")

		if err := repo.Create(ctx, want); err != nil {
			t.Fatalf("Create() unexpected error = %v", err)
		}

		got, err := repo.GetByEmail(ctx, want.Email)
		if err != nil {
			t.Fatalf("GetByEmail() unexpected error = %v", err)
		}

		assertUser(t, want, got)
	})

	t.Run("дубликат email", func(t *testing.T) {
		// Тот же email, другой телефон: конфликт должен прийти по email.
		if err := repo.Create(ctx, testUser(t, "ivan@example.com", "+79990000003")); !errors.Is(err, database.ErrDuplicate) {
			t.Errorf("Create() error = %v, want %v", err, database.ErrDuplicate)
		}
	})

	t.Run("дубликат телефона", func(t *testing.T) {
		// Другой email, тот же телефон: конфликт должен прийти по номеру.
		if err := repo.Create(ctx, testUser(t, "anna@example.com", "+79990000001")); !errors.Is(err, database.ErrDuplicate) {
			t.Errorf("Create() error = %v, want %v", err, database.ErrDuplicate)
		}
	})

	t.Run("Update", func(t *testing.T) {
		user, err := repo.GetByEmail(ctx, "petr@example.com")
		if err != nil {
			t.Fatalf("GetByEmail() unexpected error = %v", err)
		}

		firstName := "Пётр"
		gender := model.GenderFemale
		phoneNumber := ""

		birthDate, err := model.ParseDate("2000-01-02")
		if err != nil {
			t.Fatalf("model.ParseDate() unexpected error = %v", err)
		}

		err = user.ApplyUpdate(model.UpdateUserInput{
			FirstName:   &firstName,
			Gender:      &gender,
			PhoneNumber: &phoneNumber,
			BirthDate:   &birthDate,
		}, time.Now().UTC().Truncate(time.Microsecond))
		if err != nil {
			t.Fatalf("ApplyUpdate() unexpected error = %v", err)
		}

		if err := repo.Update(ctx, user); err != nil {
			t.Fatalf("Update() unexpected error = %v", err)
		}

		got, err := repo.GetByID(ctx, user.ID)
		if err != nil {
			t.Fatalf("GetByID() unexpected error = %v", err)
		}

		assertUser(t, user, got)

		if got.PhoneNumber != "" {
			t.Errorf("PhoneNumber = %q, want пустую строку", got.PhoneNumber)
		}
	})

	t.Run("Update отсутствующего пользователя", func(t *testing.T) {
		if err := repo.Update(ctx, testUser(t, "nobody@example.com", "+79990000009")); !errors.Is(err, database.ErrNotFound) {
			t.Errorf("Update() error = %v, want %v", err, database.ErrNotFound)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		user := testUser(t, "maria@example.com", "+79990000004")

		if err := repo.Create(ctx, user); err != nil {
			t.Fatalf("Create() unexpected error = %v", err)
		}

		if err := repo.Delete(ctx, user.ID); err != nil {
			t.Fatalf("Delete() unexpected error = %v", err)
		}

		if _, err := repo.GetByID(ctx, user.ID); !errors.Is(err, database.ErrNotFound) {
			t.Errorf("GetByID() error = %v, want %v", err, database.ErrNotFound)
		}

		if err := repo.Delete(ctx, user.ID); !errors.Is(err, database.ErrNotFound) {
			t.Errorf("повторный Delete() error = %v, want %v", err, database.ErrNotFound)
		}
	})

	t.Run("soft delete освобождает email", func(t *testing.T) {
		// Удаление — это is_active = false: строка остаётся, но адрес снова
		// свободен (частичный уникальный индекс users_email_idx), а GetByEmail
		// находит только активную запись, не удалённую.
		removed := testUser(t, "reuse@example.com", "+79990000005")

		if err := repo.Create(ctx, removed); err != nil {
			t.Fatalf("Create() unexpected error = %v", err)
		}

		if err := repo.Delete(ctx, removed.ID); err != nil {
			t.Fatalf("Delete() unexpected error = %v", err)
		}

		if _, err := repo.GetByEmail(ctx, removed.Email); !errors.Is(err, database.ErrNotFound) {
			t.Errorf("GetByEmail() удалённого error = %v, want %v", err, database.ErrNotFound)
		}

		current := testUser(t, "reuse@example.com", "+79990000006")
		if err := repo.Create(ctx, current); err != nil {
			t.Fatalf("повторный Create() unexpected error = %v", err)
		}

		got, err := repo.GetByEmail(ctx, current.Email)
		if err != nil {
			t.Fatalf("GetByEmail() unexpected error = %v", err)
		}

		if got.ID != current.ID {
			t.Errorf("GetByEmail() вернул %q, want активного %q", got.ID, current.ID)
		}
	})

	t.Run("Get отсутствующего пользователя", func(t *testing.T) {
		const missingID = "11111111-1111-4111-8111-111111111111"

		if _, err := repo.GetByID(ctx, missingID); !errors.Is(err, database.ErrNotFound) {
			t.Errorf("GetByID() error = %v, want %v", err, database.ErrNotFound)
		}

		if _, err := repo.GetByEmail(ctx, "nobody@example.com"); !errors.Is(err, database.ErrNotFound) {
			t.Errorf("GetByEmail() error = %v, want %v", err, database.ErrNotFound)
		}
	})
}

// TestUserRepo_SoftDeleteMarksBothFlags проверяет, что удаление помечает строку
// целиком: is_active = false и is_delete = true (как в ERDB для пользователей).
// Строка остаётся в таблице — её читают напрямую, минуя фильтр репозитория.
func TestUserRepo_SoftDeleteMarksBothFlags(t *testing.T) {
	ctx := context.Background()

	conn := newSQLiteTestDB(t)
	repo := NewUserRepo(context.Background(), conn, database.DialectSQLite)

	user := testUser(t, "flags@example.com", "+79990000010")
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create() unexpected error = %v", err)
	}

	if err := repo.Delete(ctx, user.ID); err != nil {
		t.Fatalf("Delete() unexpected error = %v", err)
	}

	var (
		isActive bool
		isDelete bool
	)

	err := conn.QueryRowContext(ctx,
		"SELECT is_active, is_delete FROM users WHERE id = ?", user.ID).Scan(&isActive, &isDelete)
	if err != nil {
		t.Fatalf("SELECT is_active, is_delete unexpected error = %v", err)
	}

	if isActive {
		t.Error("is_active = true после удаления, want false")
	}
	if !isDelete {
		t.Error("is_delete = false после удаления, want true")
	}
}

// resultStub — sql.Result с заданным числом строк и ошибкой: проверяет
// обработку «ничего не изменено» без настоящей базы.
type resultStub struct {
	affected int64
	err      error
}

func (r resultStub) LastInsertId() (int64, error) { return 0, nil }

func (r resultStub) RowsAffected() (int64, error) { return r.affected, r.err }

// errTestResult — ошибка драйвера при получении числа изменённых строк.
var errTestResult = errors.New("rows affected is unavailable")

func TestRequireAffected(t *testing.T) {
	t.Run("строка изменена", func(t *testing.T) {
		if err := requireAffected(resultStub{affected: 1}, "update user"); err != nil {
			t.Errorf("requireAffected() unexpected error = %v", err)
		}
	})

	t.Run("ничего не изменено — ErrNotFound", func(t *testing.T) {
		if err := requireAffected(resultStub{}, "update user"); !errors.Is(err, database.ErrNotFound) {
			t.Errorf("requireAffected() error = %v, want %v", err, database.ErrNotFound)
		}
	})

	t.Run("драйвер не сообщил число строк", func(t *testing.T) {
		if err := requireAffected(resultStub{err: errTestResult}, "update user"); !errors.Is(err, errTestResult) {
			t.Errorf("requireAffected() error = %v, want %v", err, errTestResult)
		}
	})
}

// TestCodec_ArgEncoding проверяет кодирование времени и даты по диалектам:
// PostgreSQL принимает time.Time, SQLite — строку в заданном репозиторием формате.
func TestCodec_ArgEncoding(t *testing.T) {
	moment := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	date, err := model.ParseDate("1990-05-17")
	if err != nil {
		t.Fatalf("model.ParseDate() unexpected error = %v", err)
	}

	tests := []struct {
		name     string
		dialect  string
		wantTime any
		wantDate any
	}{
		{
			name:     "sqlite хранит время и дату строкой",
			dialect:  database.DialectSQLite,
			wantTime: "2026-01-02T03:04:05Z",
			wantDate: "1990-05-17",
		},
		{
			name:     "postgres принимает time.Time",
			dialect:  database.DialectPostgres,
			wantTime: moment,
			wantDate: date.Time(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := codec{dialect: tt.dialect}

			if got := c.timeArg(moment); got != tt.wantTime {
				t.Errorf("timeArg() = %#v, want %#v", got, tt.wantTime)
			}

			if got := c.nullTimeArg(&moment); got != tt.wantTime {
				t.Errorf("nullTimeArg() = %#v, want %#v", got, tt.wantTime)
			}

			if got := c.nullTimeArg(nil); got != nil {
				t.Errorf("nullTimeArg(nil) = %#v, want nil", got)
			}

			if got := c.dateArg(&date); got != tt.wantDate {
				t.Errorf("dateArg() = %#v, want %#v", got, tt.wantDate)
			}

			if got := c.dateArg(nil); got != nil {
				t.Errorf("dateArg(nil) = %#v, want nil", got)
			}
		})
	}
}

// TestNullTime_Scan проверяет разбор времени из обоих диалектов и отказ на
// данных, которые репозиторий не ожидает: молчаливое «нулевое время» скрыло бы
// проблему с форматом в базе.
func TestNullTime_Scan(t *testing.T) {
	const parsed = "2026-01-02T03:04:05Z"

	tests := []struct {
		name      string
		src       any
		wantValid bool
		want      string
		wantErr   bool
	}{
		{name: "NULL", src: nil},
		{
			name:      "time.Time из PostgreSQL",
			src:       time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
			wantValid: true,
			want:      parsed,
		},
		{name: "строка из SQLite", src: parsed, wantValid: true, want: parsed},
		{name: "байты из SQLite", src: []byte(parsed), wantValid: true, want: parsed},
		{name: "битый формат", src: "вчера", wantErr: true},
		{name: "неподдерживаемый тип", src: int64(42), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got nullTime

			err := got.Scan(tt.src)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Scan() error = nil, want ошибку")
				}

				return
			}

			if err != nil {
				t.Fatalf("Scan() unexpected error = %v", err)
			}

			if got.valid != tt.wantValid {
				t.Errorf("valid = %v, want %v", got.valid, tt.wantValid)
			}

			if tt.wantValid {
				if got.time().Format(time.RFC3339) != tt.want {
					t.Errorf("time() = %v, want %v", got.time(), tt.want)
				}

				if got.ptr() == nil || !got.ptr().Equal(got.time()) {
					t.Errorf("ptr() = %v, want %v", got.ptr(), got.time())
				}

				return
			}

			if got.ptr() != nil || !got.time().IsZero() {
				t.Errorf("для NULL получено %v / %v, want нулевые значения", got.ptr(), got.time())
			}
		})
	}
}

// TestNullDate_Scan проверяет разбор календарной даты из обоих диалектов.
func TestNullDate_Scan(t *testing.T) {
	tests := []struct {
		name    string
		src     any
		want    string
		wantNil bool
		wantErr bool
	}{
		{name: "NULL", src: nil, wantNil: true},
		{
			name: "time.Time из PostgreSQL",
			src:  time.Date(1990, 5, 17, 0, 0, 0, 0, time.UTC),
			want: "1990-05-17",
		},
		{name: "строка из SQLite", src: "1990-05-17", want: "1990-05-17"},
		{name: "байты из SQLite", src: []byte("1990-05-17"), want: "1990-05-17"},
		{name: "битый формат", src: "17.05.1990", wantErr: true},
		{name: "неподдерживаемый тип", src: int64(19900517), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got nullDate

			err := got.Scan(tt.src)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Scan() error = nil, want ошибку")
				}

				return
			}

			if err != nil {
				t.Fatalf("Scan() unexpected error = %v", err)
			}

			if tt.wantNil {
				if got.date != nil {
					t.Errorf("date = %v, want nil", got.date)
				}

				return
			}

			if got.date == nil {
				t.Fatal("date = nil, want разобранную дату")
			}

			if got.date.String() != tt.want {
				t.Errorf("date = %q, want %q", got.date.String(), tt.want)
			}
		})
	}
}
