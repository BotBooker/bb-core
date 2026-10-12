package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/model"
)

// testCredentials собирает учётные данные пользователя.
//
// Пароль хешируется моделью (PBKDF2, ~0,2 с на вызов), поэтому тесты вызывают
// этот помощник экономно: один хеш на сценарий, а не на каждую проверку.
func testCredentials(t *testing.T, userID string) *model.Credentials {
	t.Helper()

	credentials, err := model.NewCredentials(userID, "correct horse battery staple",
		time.Now().UTC().Truncate(time.Microsecond))
	if err != nil {
		t.Fatalf("model.NewCredentials() unexpected error = %v", err)
	}

	return credentials
}

func TestNewCredentialRepo(t *testing.T) {
	tests := []struct {
		name    string
		dialect string
	}{
		{name: "sqlite", dialect: database.DialectSQLite},
		{name: "postgres", dialect: database.DialectPostgres},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewCredentialRepo(nil, tt.dialect); got == nil {
				t.Error("NewCredentialRepo() returned nil")
			}
		})
	}
}

func TestCredentialRepoSQLite(t *testing.T) {
	runCredentialRepoScenario(t, newSQLiteTestDB(t), database.DialectSQLite)
}

func TestCredentialRepoPostgres(t *testing.T) {
	runCredentialRepoScenario(t, newPostgresTestDB(t), database.DialectPostgres)
}

// runCredentialRepoScenario — общий сценарий для обоих диалектов.
func runCredentialRepoScenario(t *testing.T, conn database.DB, dialect string) {
	t.Helper()

	ctx := context.Background()

	userRepo := NewUserRepo(conn, dialect)
	credentialRepo := NewCredentialRepo(conn, dialect)

	user := testUser(t, "credentials@example.com", "+79990000126")
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("UserRepo.Create() unexpected error = %v", err)
	}

	credentials := testCredentials(t, user.ID)

	t.Run("создание и чтение", func(t *testing.T) {
		if err := credentialRepo.Create(ctx, credentials); err != nil {
			t.Fatalf("Create() unexpected error = %v", err)
		}

		got, err := credentialRepo.GetByUserID(ctx, user.ID)
		if err != nil {
			t.Fatalf("GetByUserID() unexpected error = %v", err)
		}

		if got.UserID != user.ID {
			t.Errorf("UserID = %q, want %q", got.UserID, user.ID)
		}
		// Хеш возвращается ровно таким, каким был сохранён: репозиторий его
		// не пересчитывает и не «поправляет».
		if got.Hash != credentials.Hash {
			t.Errorf("Hash = %q, want %q", got.Hash, credentials.Hash)
		}
		if !got.CreatedAt.Equal(credentials.CreatedAt) || !got.UpdatedAt.Equal(credentials.UpdatedAt) {
			t.Errorf("CreatedAt/UpdatedAt = %v/%v, want %v/%v",
				got.CreatedAt, got.UpdatedAt, credentials.CreatedAt, credentials.UpdatedAt)
		}
	})

	t.Run("повторная вставка — ErrDuplicate", func(t *testing.T) {
		if err := credentialRepo.Create(ctx, credentials); !errors.Is(err, database.ErrDuplicate) {
			t.Errorf("Create() error = %v, want %v", err, database.ErrDuplicate)
		}
	})

	t.Run("нет учётных данных — ErrNotFound", func(t *testing.T) {
		unknown := "3f0c8f0e-9f60-4e0e-9d5f-1f6f1f6f1f6f"

		if _, err := credentialRepo.GetByUserID(ctx, unknown); !errors.Is(err, database.ErrNotFound) {
			t.Errorf("GetByUserID() error = %v, want %v", err, database.ErrNotFound)
		}
	})

	t.Run("пароль неизвестного пользователя не сохраняется", func(t *testing.T) {
		// Внешний ключ на users не даёт завести учётные данные «в никуда»:
		// иначе появился бы пароль, к которому нет пользователя.
		orphan := testCredentials(t, "3f0c8f0e-9f60-4e0e-9d5f-1f6f1f6f1f6f")

		if err := credentialRepo.Create(ctx, orphan); err == nil {
			t.Error("Create() error = nil, want нарушение внешнего ключа")
		}
	})
}

// TestCredentialRepo_UserSoftDelete проверяет удаление пользователя: это soft
// delete (is_active = false), а не физический DELETE. Строка users остаётся
// ради истории, поэтому ON DELETE CASCADE не срабатывает и учётные данные
// сохраняются; при этом пользователь пропадает из активных и вход по нему
// невозможен (GetByID возвращает ErrNotFound).
func TestCredentialRepo_UserSoftDelete(t *testing.T) {
	ctx := context.Background()

	conn := newSQLiteTestDB(t)
	userRepo := NewUserRepo(conn, database.DialectSQLite)
	credentialRepo := NewCredentialRepo(conn, database.DialectSQLite)

	user := testUser(t, "credentials-cascade@example.com", "+79990000127")
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("UserRepo.Create() unexpected error = %v", err)
	}

	if err := credentialRepo.Create(ctx, testCredentials(t, user.ID)); err != nil {
		t.Fatalf("Create() unexpected error = %v", err)
	}

	if err := userRepo.Delete(ctx, user.ID); err != nil {
		t.Fatalf("UserRepo.Delete() unexpected error = %v", err)
	}

	if _, err := userRepo.GetByID(ctx, user.ID); !errors.Is(err, database.ErrNotFound) {
		t.Errorf("удалённый пользователь error = %v, want %v", err, database.ErrNotFound)
	}

	credentials, err := credentialRepo.GetByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("учётные данные удалённого пользователя error = %v, want запись", err)
	}

	if credentials.UserID != user.ID {
		t.Errorf("UserID = %q, want %q", credentials.UserID, user.ID)
	}
}
