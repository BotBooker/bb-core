package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/model"
)

// testCredentialHash — сохранённый хеш пароля в формате pkg/password.
// Репозиторий хеш не считает и не разбирает: он лишь хранит строку, поэтому
// тестам достаточно фиксированного значения (хеширование проверяет
// pkg/password).
const testCredentialHash = "argon2id$v=19$m=19456,t=2,p=1$c2FsdA$aGFzaA"

// testCredentials собирает учётные данные пользователя.
//
// Срок действия задаётся явно: он попадает в expires_at.
func testCredentials(t *testing.T, userID, email string) *model.Credentials {
	t.Helper()

	credentials, err := model.NewCredentials(userID, email, testCredentialHash,
		90*24*time.Hour, time.Now().UTC().Truncate(time.Microsecond))
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
			if got := NewCredentialRepo(context.Background(), nil, tt.dialect); got == nil {
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

	userRepo := NewUserRepo(context.Background(), conn, dialect)
	credentialRepo := NewCredentialRepo(context.Background(), conn, dialect)

	user := testUser(t, "credentials@example.com", "+79990000126")
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("UserRepo.Create() unexpected error = %v", err)
	}

	credentials := testCredentials(t, user.ID, user.Email)

	t.Run("создание и чтение", func(t *testing.T) {
		if err := credentialRepo.Create(ctx, credentials); err != nil {
			t.Fatalf("Create() unexpected error = %v", err)
		}

		got, err := credentialRepo.GetByUserID(ctx, user.ID)
		if err != nil {
			t.Fatalf("GetByUserID() unexpected error = %v", err)
		}

		if got.ID != credentials.ID {
			t.Errorf("ID = %q, want %q", got.ID, credentials.ID)
		}
		if got.UserID != user.ID {
			t.Errorf("UserID = %q, want %q", got.UserID, user.ID)
		}
		if got.Provider != model.CredentialProviderEmail || got.ProviderKey != user.Email {
			t.Errorf("Provider/ProviderKey = %q/%q, want %q/%q",
				got.Provider, got.ProviderKey, model.CredentialProviderEmail, user.Email)
		}
		// Хеш возвращается ровно таким, каким был сохранён: репозиторий его
		// не пересчитывает и не «поправляет».
		if got.Hash != credentials.Hash {
			t.Errorf("Hash = %q, want %q", got.Hash, credentials.Hash)
		}
		if !got.CreatedAt.Equal(credentials.CreatedAt) {
			t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, credentials.CreatedAt)
		}
		if !got.ExpiresAt.Equal(credentials.ExpiresAt) {
			t.Errorf("ExpiresAt = %v, want %v", got.ExpiresAt, credentials.ExpiresAt)
		}
		if got.LastLoginAt != nil || got.RevokedAt != nil {
			t.Errorf("LastLoginAt/RevokedAt = %v/%v, want nil/nil", got.LastLoginAt, got.RevokedAt)
		}
		if !got.IsActive || got.IsDelete {
			t.Errorf("IsActive/IsDelete = %t/%t, want true/false", got.IsActive, got.IsDelete)
		}
	})

	t.Run("поиск по провайдеру и ключу", func(t *testing.T) {
		got, err := credentialRepo.GetByProvider(ctx, string(model.CredentialProviderEmail), user.Email)
		if err != nil {
			t.Fatalf("GetByProvider() unexpected error = %v", err)
		}

		if got.ID != credentials.ID {
			t.Errorf("ID = %q, want %q", got.ID, credentials.ID)
		}
	})

	t.Run("неизвестный провайдер — ErrNotFound", func(t *testing.T) {
		if _, err := credentialRepo.GetByProvider(ctx, "telegram", "12345"); !errors.Is(err, database.ErrNotFound) {
			t.Errorf("GetByProvider() error = %v, want %v", err, database.ErrNotFound)
		}
	})

	t.Run("пустые аргументы поиска — ErrInvalidCredential", func(t *testing.T) {
		if _, err := credentialRepo.GetByProvider(ctx, "", ""); !errors.Is(err, model.ErrInvalidCredential) {
			t.Errorf("GetByProvider() error = %v, want %v", err, model.ErrInvalidCredential)
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

	t.Run("обновление хеша и срока", func(t *testing.T) {
		// Смена пароля и пересчёт устаревшего хеша сохраняются одним UPDATE:
		// меняются хеш и срок действия.
		updated := *credentials

		changedAt := credentials.CreatedAt.Add(time.Hour)
		if err := updated.SetPasswordHash(testCredentialHash+"-updated", 30*24*time.Hour, changedAt); err != nil {
			t.Fatalf("SetPasswordHash() unexpected error = %v", err)
		}

		if err := credentialRepo.Update(ctx, &updated); err != nil {
			t.Fatalf("Update() unexpected error = %v", err)
		}

		got, err := credentialRepo.GetByUserID(ctx, user.ID)
		if err != nil {
			t.Fatalf("GetByUserID() unexpected error = %v", err)
		}

		if got.Hash != updated.Hash {
			t.Errorf("Hash = %q, want %q", got.Hash, updated.Hash)
		}
		if !got.ExpiresAt.Equal(updated.ExpiresAt) {
			t.Errorf("ExpiresAt = %v, want %v", got.ExpiresAt, updated.ExpiresAt)
		}
	})

	t.Run("метаданные провайдера сохраняются как JSON", func(t *testing.T) {
		updated := *credentials
		updated.Meta = []byte(`{"locale":"ru"}`)

		if err := credentialRepo.Update(ctx, &updated); err != nil {
			t.Fatalf("Update() unexpected error = %v", err)
		}

		got, err := credentialRepo.GetByUserID(ctx, user.ID)
		if err != nil {
			t.Fatalf("GetByUserID() unexpected error = %v", err)
		}

		assertJSONEqual(t, got.Meta, `{"locale":"ru"}`)
	})

	t.Run("отметка последнего входа", func(t *testing.T) {
		loginAt := credentials.CreatedAt.Add(2 * time.Hour)

		if err := credentialRepo.TouchLastLogin(ctx, credentials.ID, loginAt); err != nil {
			t.Fatalf("TouchLastLogin() unexpected error = %v", err)
		}

		got, err := credentialRepo.GetByUserID(ctx, user.ID)
		if err != nil {
			t.Fatalf("GetByUserID() unexpected error = %v", err)
		}

		if got.LastLoginAt == nil || !got.LastLoginAt.Equal(loginAt) {
			t.Errorf("LastLoginAt = %v, want %v", got.LastLoginAt, loginAt)
		}
	})

	t.Run("отметка неизвестных учётных данных — ErrNotFound", func(t *testing.T) {
		unknown := "3f0c8f0e-9f60-4e0e-9d5f-1f6f1f6f1f6f"

		err := credentialRepo.TouchLastLogin(ctx, unknown, time.Now().UTC().Truncate(time.Microsecond))
		if !errors.Is(err, database.ErrNotFound) {
			t.Errorf("TouchLastLogin() error = %v, want %v", err, database.ErrNotFound)
		}
	})

	t.Run("обновление неизвестных учётных данных — ErrNotFound", func(t *testing.T) {
		orphan := testCredentials(t, "3f0c8f0e-9f60-4e0e-9d5f-1f6f1f6f1f6f", "orphan@example.com")

		if err := credentialRepo.Update(ctx, orphan); !errors.Is(err, database.ErrNotFound) {
			t.Errorf("Update() error = %v, want %v", err, database.ErrNotFound)
		}
	})

	t.Run("пароль неизвестного пользователя не сохраняется", func(t *testing.T) {
		// Внешний ключ на users не даёт завести учётные данные «в никуда»:
		// иначе появился бы пароль, к которому нет пользователя.
		orphan := testCredentials(t, "3f0c8f0e-9f60-4e0e-9d5f-1f6f1f6f1f6f", "orphan@example.com")

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
	userRepo := NewUserRepo(context.Background(), conn, database.DialectSQLite)
	credentialRepo := NewCredentialRepo(context.Background(), conn, database.DialectSQLite)

	user := testUser(t, "credentials-cascade@example.com", "+79990000127")
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("UserRepo.Create() unexpected error = %v", err)
	}

	if err := credentialRepo.Create(ctx, testCredentials(t, user.ID, user.Email)); err != nil {
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
