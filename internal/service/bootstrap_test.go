package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/model"
)

// newTestBootstrapService создаёт сервис bootstrap с моками и фиксированными
// часами: так проверки не зависят от реального времени.
func newTestBootstrapService(
	t *testing.T,
	users *mockAuthUserRepository,
	credentials *mockAuthCredentialRepository,
) *bootstrapService {
	t.Helper()

	created := NewBootstrapService(users, credentials)

	svc, ok := created.(*bootstrapService)
	if !ok {
		t.Fatalf("NewBootstrapService() returned %T, want *bootstrapService", created)
	}

	svc.now = func() time.Time { return testNow }

	return svc
}

// testBootstrapInput — данные первого суперпользователя.
func testBootstrapInput() BootstrapInput {
	return BootstrapInput{
		Email:     "Admin@Example.com",
		Password:  "super-secret-password",
		FirstName: "Super",
		LastName:  "Admin",
	}
}

func TestBootstrapService_EnsureSuperuser(t *testing.T) {
	t.Run("создаёт суперпользователя с ролью admin", func(t *testing.T) {
		var (
			storedUser        *model.User
			storedCredentials *model.Credentials
		)

		users := &mockAuthUserRepository{
			getByEmailFunc: func(context.Context, string) (*model.User, error) {
				return nil, database.ErrNotFound
			},
			createFunc: func(_ context.Context, user *model.User) error {
				storedUser = user

				return nil
			},
		}
		credentials := &mockAuthCredentialRepository{
			createFunc: func(_ context.Context, got *model.Credentials) error {
				storedCredentials = got

				return nil
			},
		}

		svc := newTestBootstrapService(t, users, credentials)

		user, created, err := svc.EnsureSuperuser(context.Background(), testBootstrapInput())
		if err != nil {
			t.Fatalf("EnsureSuperuser() unexpected error = %v", err)
		}

		if !created {
			t.Error("EnsureSuperuser() created = false, want true")
		}
		if user == nil {
			t.Fatal("EnsureSuperuser() user = nil")
		}
		if user.Role != model.RoleAdmin {
			t.Errorf("Role = %q, want %q", user.Role, model.RoleAdmin)
		}
		if user.Email != "admin@example.com" {
			t.Errorf("Email = %q, want %q (адрес нормализуется)", user.Email, "admin@example.com")
		}
		if storedUser != user {
			t.Error("EnsureSuperuser() не сохранил пользователя")
		}
		if storedCredentials == nil || storedCredentials.UserID != user.ID {
			t.Fatalf("учётные данные = %+v, want для пользователя %q", storedCredentials, user.ID)
		}
		if err := storedCredentials.Verify("super-secret-password"); err != nil {
			t.Errorf("сохранённый пароль не проверяется: %v", err)
		}
		if !storedCredentials.CreatedAt.Equal(testNow) {
			t.Errorf("CreatedAt = %v, want %v", storedCredentials.CreatedAt, testNow)
		}
	})

	t.Run("повторный запуск не создаёт второго", func(t *testing.T) {
		existing := testAuthUser(t, model.RoleAdmin)

		createCalled := false

		users := &mockAuthUserRepository{
			getByEmailFunc: func(_ context.Context, email string) (*model.User, error) {
				if email != "admin@example.com" {
					t.Errorf("GetByEmail() email = %q, want %q", email, "admin@example.com")
				}

				return existing, nil
			},
			createFunc: func(context.Context, *model.User) error {
				createCalled = true

				return nil
			},
		}

		svc := newTestBootstrapService(t, users, &mockAuthCredentialRepository{})

		user, created, err := svc.EnsureSuperuser(context.Background(), testBootstrapInput())
		if err != nil {
			t.Fatalf("EnsureSuperuser() unexpected error = %v", err)
		}

		if created {
			t.Error("EnsureSuperuser() created = true для существующего пользователя")
		}
		if user != existing {
			t.Error("EnsureSuperuser() вернул не существующего пользователя")
		}
		if createCalled {
			t.Error("EnsureSuperuser() создал второго суперпользователя")
		}
	})
}

// TestBootstrapService_EnsureSuperuser_Race проверяет гонку двух экземпляров:
// проигравший вставку считает суперпользователя созданным, а не падает.
func TestBootstrapService_EnsureSuperuser_Race(t *testing.T) {
	existing := testAuthUser(t, model.RoleAdmin)

	calls := 0

	users := &mockAuthUserRepository{
		getByEmailFunc: func(context.Context, string) (*model.User, error) {
			calls++
			if calls == 1 {
				return nil, database.ErrNotFound
			}

			return existing, nil
		},
		createFunc: func(context.Context, *model.User) error {
			return database.ErrDuplicate
		},
	}

	svc := newTestBootstrapService(t, users, &mockAuthCredentialRepository{})

	user, created, err := svc.EnsureSuperuser(context.Background(), testBootstrapInput())
	if err != nil {
		t.Fatalf("EnsureSuperuser() unexpected error = %v", err)
	}
	if created {
		t.Error("EnsureSuperuser() created = true при гонке")
	}
	if user != existing {
		t.Error("EnsureSuperuser() вернул не существующего пользователя")
	}
}

// TestBootstrapService_EnsureSuperuser_Rollback проверяет откат: пользователь
// без пароля не должен остаться в базе и занять email.
func TestBootstrapService_EnsureSuperuser_Rollback(t *testing.T) {
	var deletedID string

	users := &mockAuthUserRepository{
		getByEmailFunc: func(context.Context, string) (*model.User, error) {
			return nil, database.ErrNotFound
		},
		deleteFunc: func(_ context.Context, id string) error {
			deletedID = id

			return nil
		},
	}
	credentials := &mockAuthCredentialRepository{
		createFunc: func(context.Context, *model.Credentials) error {
			return errors.New("insert credentials failed")
		},
	}

	svc := newTestBootstrapService(t, users, credentials)

	user, created, err := svc.EnsureSuperuser(context.Background(), testBootstrapInput())
	if err == nil {
		t.Fatal("EnsureSuperuser() error = nil, want ошибку")
	}
	if created || user != nil {
		t.Error("EnsureSuperuser() вернул результат при ошибке")
	}
	if deletedID == "" {
		t.Error("EnsureSuperuser() не откатил пользователя")
	}
}

// TestBootstrapService_EnsureSuperuser_InvalidConfig проверяет отсутствие
// обязательных данных: это ошибка конфигурации, а не запроса.
func TestBootstrapService_EnsureSuperuser_InvalidConfig(t *testing.T) {
	svc := newTestBootstrapService(t, &mockAuthUserRepository{}, &mockAuthCredentialRepository{})

	tests := []struct {
		name string
		in   BootstrapInput
	}{
		{name: "нет email", in: BootstrapInput{Password: "super-secret-password"}},
		{name: "email из пробелов", in: BootstrapInput{Email: "   ", Password: "super-secret-password"}},
		{name: "нет пароля", in: BootstrapInput{Email: "admin@example.com"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := svc.EnsureSuperuser(context.Background(), tt.in); !errors.Is(err, ErrInvalidBootstrap) {
				t.Errorf("EnsureSuperuser() error = %v, want %v", err, ErrInvalidBootstrap)
			}
		})
	}
}

// TestBootstrapService_EnsureSuperuser_RepositoryError проверяет, что ошибка
// чтения пользователя не маскируется.
func TestBootstrapService_EnsureSuperuser_RepositoryError(t *testing.T) {
	users := &mockAuthUserRepository{
		getByEmailFunc: func(context.Context, string) (*model.User, error) {
			return nil, errors.New("select user failed")
		},
	}

	svc := newTestBootstrapService(t, users, &mockAuthCredentialRepository{})

	if _, _, err := svc.EnsureSuperuser(context.Background(), testBootstrapInput()); err == nil {
		t.Error("EnsureSuperuser() error = nil, want ошибку")
	}
}
