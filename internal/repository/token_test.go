package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/model"
)

// testToken собирает токен с заданным сроком жизни.
//
// Сессия действует в рамках тенанта, поэтому он передаётся явно: у
// user_tokens.tenant_id внешний ключ на tenants, и тенант должен существовать.
// Роль — роль пользователя в этом тенанте на момент выдачи.
//
// Время усечено до микросекунд — предел точности TIMESTAMPTZ: с ним результат
// одинаков у обоих диалектов, а сравнение в тестах не зависит от округления.
func testToken(t *testing.T, tenantID string, user *model.User, ttl time.Duration) *model.Token {
	t.Helper()

	token, err := model.NewToken(user, tenantID, model.RoleClient, ttl, time.Now().UTC().Truncate(time.Microsecond))
	if err != nil {
		t.Fatalf("model.NewToken() unexpected error = %v", err)
	}

	return token
}

func TestNewTokenRepo(t *testing.T) {
	tests := []struct {
		name    string
		dialect string
	}{
		{name: "sqlite", dialect: database.DialectSQLite},
		{name: "postgres", dialect: database.DialectPostgres},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewTokenRepo(nil, tt.dialect); got == nil {
				t.Error("NewTokenRepo() returned nil")
			}
		})
	}
}

func TestTokenRepoSQLite(t *testing.T) {
	runTokenRepoScenario(t, newSQLiteTestDB(t), database.DialectSQLite)
}

// TestTokenRepoPostgres проверяет репозиторий токенов на PostgreSQL: там другие
// плейсхолдеры и тип времени (TIMESTAMPTZ вместо строки SQLite).
func TestTokenRepoPostgres(t *testing.T) {
	runTokenRepoScenario(t, newPostgresTestDB(t), database.DialectPostgres)
}

// runTokenRepoScenario — общий сценарий для обоих диалектов: он проверяет
// работу с настоящей схемой, поэтому одинаковый набор проверок прогоняется
// и на SQLite, и на PostgreSQL.
func runTokenRepoScenario(t *testing.T, conn database.DB, dialect string) {
	t.Helper()

	ctx := context.Background()

	userRepo := NewUserRepo(conn, dialect)
	tokenRepo := NewTokenRepo(conn, dialect)
	tenantRepo := NewTenantRepo(conn, dialect)

	user := testUser(t, "token-owner@example.com", "+79990000123")
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("UserRepo.Create() unexpected error = %v", err)
	}

	// Сессия действует в тенанте, поэтому он должен существовать до токена.
	tenant := testTenant(t, tenantRepo, "token-owner")

	t.Run("создание и чтение", func(t *testing.T) {
		token := testToken(t, tenant.ID, user, time.Hour)

		if err := tokenRepo.Create(ctx, token); err != nil {
			t.Fatalf("Create() unexpected error = %v", err)
		}

		got, err := tokenRepo.GetByID(ctx, token.ID)
		if err != nil {
			t.Fatalf("GetByID() unexpected error = %v", err)
		}

		if got.ID != token.ID || got.UserID != user.ID || got.TenantID != tenant.ID || got.Role != token.Role {
			t.Errorf("GetByID() = %+v, want id=%q user=%q tenant=%q role=%q",
				got, token.ID, user.ID, tenant.ID, token.Role)
		}
		if !got.IssuedAt.Equal(token.IssuedAt) || !got.ExpiresAt.Equal(token.ExpiresAt) {
			t.Errorf("время = %v..%v, want %v..%v",
				got.IssuedAt, got.ExpiresAt, token.IssuedAt, token.ExpiresAt)
		}
		if got.Revoked() {
			t.Errorf("RevokedAt = %v, want nil", got.RevokedAt)
		}
		if got.LastUsedAt != nil {
			t.Errorf("LastUsedAt = %v, want nil", got.LastUsedAt)
		}
	})

	t.Run("повторная вставка — ErrDuplicate", func(t *testing.T) {
		token := testToken(t, tenant.ID, user, time.Hour)

		if err := tokenRepo.Create(ctx, token); err != nil {
			t.Fatalf("Create() unexpected error = %v", err)
		}

		if err := tokenRepo.Create(ctx, token); !errors.Is(err, database.ErrDuplicate) {
			t.Errorf("Create() error = %v, want %v", err, database.ErrDuplicate)
		}
	})

	t.Run("неизвестный токен — ErrNotFound", func(t *testing.T) {
		unknown := "3f0c8f0e-9f60-4e0e-9d5f-1f6f1f6f1f6f"

		if _, err := tokenRepo.GetByID(ctx, unknown); !errors.Is(err, database.ErrNotFound) {
			t.Errorf("GetByID() error = %v, want %v", err, database.ErrNotFound)
		}

		now := time.Now().UTC().Truncate(time.Microsecond)

		if err := tokenRepo.TouchLastUsed(ctx, unknown, now); !errors.Is(err, database.ErrNotFound) {
			t.Errorf("TouchLastUsed() error = %v, want %v", err, database.ErrNotFound)
		}

		if err := tokenRepo.Revoke(ctx, unknown, now); !errors.Is(err, database.ErrNotFound) {
			t.Errorf("Revoke() error = %v, want %v", err, database.ErrNotFound)
		}
	})

	t.Run("активность и отзыв", func(t *testing.T) {
		token := testToken(t, tenant.ID, user, time.Hour)

		if err := tokenRepo.Create(ctx, token); err != nil {
			t.Fatalf("Create() unexpected error = %v", err)
		}

		usedAt := token.IssuedAt.Add(10 * time.Minute)
		if err := tokenRepo.TouchLastUsed(ctx, token.ID, usedAt); err != nil {
			t.Fatalf("TouchLastUsed() unexpected error = %v", err)
		}

		got, err := tokenRepo.GetByID(ctx, token.ID)
		if err != nil {
			t.Fatalf("GetByID() unexpected error = %v", err)
		}

		if got.LastUsedAt == nil || !got.LastUsedAt.Equal(usedAt) {
			t.Errorf("LastUsedAt = %v, want %v", got.LastUsedAt, usedAt)
		}

		revokedAt := token.IssuedAt.Add(20 * time.Minute)
		if err := tokenRepo.Revoke(ctx, token.ID, revokedAt); err != nil {
			t.Fatalf("Revoke() unexpected error = %v", err)
		}

		got, err = tokenRepo.GetByID(ctx, token.ID)
		if err != nil {
			t.Fatalf("GetByID() unexpected error = %v", err)
		}

		if !got.Revoked() {
			t.Fatal("RevokedAt = nil, want время отзыва")
		}

		if !got.RevokedAt.Equal(revokedAt) {
			t.Errorf("RevokedAt = %v, want %v", got.RevokedAt, revokedAt)
		}

		// Повторный выход из системы ошибкой не считается: клиент мог потерять
		// ответ на первый запрос.
		if err := tokenRepo.Revoke(ctx, token.ID, revokedAt.Add(time.Hour)); err != nil {
			t.Errorf("повторный Revoke() unexpected error = %v", err)
		}

		got, err = tokenRepo.GetByID(ctx, token.ID)
		if err != nil {
			t.Fatalf("GetByID() unexpected error = %v", err)
		}

		if !got.RevokedAt.Equal(revokedAt) {
			t.Errorf("RevokedAt = %v после повторного отзыва, want %v", got.RevokedAt, revokedAt)
		}
	})
}

// TestTokenRepo_DeleteExpired проверяет чистку просроченных сессий: таблица не
// должна расти с каждым входом в систему.
func TestTokenRepo_DeleteExpired(t *testing.T) {
	ctx := context.Background()

	conn := newSQLiteTestDB(t)
	userRepo := NewUserRepo(conn, database.DialectSQLite)
	tokenRepo := NewTokenRepo(conn, database.DialectSQLite)
	tenant := testTenant(t, NewTenantRepo(conn, database.DialectSQLite), "expired-tokens")

	user := testUser(t, "expired-tokens@example.com", "+79990000124")
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("UserRepo.Create() unexpected error = %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)

	// Просроченный токен: выдан два часа назад со сроком жизни час.
	expired, err := model.NewToken(user, tenant.ID, model.RoleClient, time.Hour, now.Add(-2*time.Hour))
	if err != nil {
		t.Fatalf("model.NewToken() unexpected error = %v", err)
	}

	fresh := testToken(t, tenant.ID, user, time.Hour)

	for _, token := range []*model.Token{expired, fresh} {
		if err := tokenRepo.Create(ctx, token); err != nil {
			t.Fatalf("Create() unexpected error = %v", err)
		}
	}

	deleted, err := tokenRepo.DeleteExpired(ctx, user.ID, now)
	if err != nil {
		t.Fatalf("DeleteExpired() unexpected error = %v", err)
	}

	if deleted != 1 {
		t.Errorf("DeleteExpired() = %d, want 1", deleted)
	}

	if _, err := tokenRepo.GetByID(ctx, expired.ID); !errors.Is(err, database.ErrNotFound) {
		t.Errorf("просроченный токен error = %v, want %v", err, database.ErrNotFound)
	}

	if _, err := tokenRepo.GetByID(ctx, fresh.ID); err != nil {
		t.Errorf("действующий токен unexpected error = %v", err)
	}
}

// TestTokenRepo_UserSoftDelete проверяет удаление пользователя: это soft delete
// (is_active = false), поэтому сессии не уносятся каскадом (ON DELETE CASCADE
// относится к физическому DELETE), а просто перестают работать: пользователь
// больше не читается как активный, и аутентификация по такому токену невозможна.
func TestTokenRepo_UserSoftDelete(t *testing.T) {
	ctx := context.Background()

	conn := newSQLiteTestDB(t)
	userRepo := NewUserRepo(conn, database.DialectSQLite)
	tokenRepo := NewTokenRepo(conn, database.DialectSQLite)
	tenant := testTenant(t, NewTenantRepo(conn, database.DialectSQLite), "cascade")

	user := testUser(t, "cascade@example.com", "+79990000125")
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("UserRepo.Create() unexpected error = %v", err)
	}

	token := testToken(t, tenant.ID, user, time.Hour)
	if err := tokenRepo.Create(ctx, token); err != nil {
		t.Fatalf("Create() unexpected error = %v", err)
	}

	if err := userRepo.Delete(ctx, user.ID); err != nil {
		t.Fatalf("UserRepo.Delete() unexpected error = %v", err)
	}

	if _, err := tokenRepo.GetByID(ctx, token.ID); err != nil {
		t.Errorf("токен удалённого пользователя unexpected error = %v", err)
	}

	if _, err := userRepo.GetByID(ctx, user.ID); !errors.Is(err, database.ErrNotFound) {
		t.Errorf("удалённый пользователь error = %v, want %v", err, database.ErrNotFound)
	}
}
