package model

import (
	"errors"
	"testing"
	"time"
)

// tokenNow — фиксированное «сейчас» для тестов токенов.
var tokenNow = time.Date(2026, time.October, 12, 10, 0, 0, 0, time.UTC)

// testUserForToken возвращает валидного пользователя с заданной ролью.
func testUserForToken(t *testing.T, role Role) *User {
	t.Helper()

	user, err := NewUser(CreateUserInput{
		Email:       "ivan@example.com",
		FirstName:   "Иван",
		LastName:    "Иванов",
		PhoneNumber: "+79991234567",
	}, tokenNow)
	if err != nil {
		t.Fatalf("model.NewUser() unexpected error = %v", err)
	}

	user.Role = role

	return user
}

func TestNewToken(t *testing.T) {
	const ttl = 24 * time.Hour

	t.Run("выдаёт токен владельцу", func(t *testing.T) {
		user := testUserForToken(t, RoleClient)

		token, err := NewToken(user, ttl, tokenNow)
		if err != nil {
			t.Fatalf("NewToken() unexpected error = %v", err)
		}

		if err := ValidateUUID(token.ID); err != nil {
			t.Errorf("token.ID = %q: %v", token.ID, err)
		}
		if token.UserID != user.ID {
			t.Errorf("UserID = %q, want %q", token.UserID, user.ID)
		}
		if token.Role != RoleClient {
			t.Errorf("Role = %q, want %q", token.Role, RoleClient)
		}
		if !token.IssuedAt.Equal(tokenNow) {
			t.Errorf("IssuedAt = %v, want %v", token.IssuedAt, tokenNow)
		}
		if want := tokenNow.Add(ttl); !token.ExpiresAt.Equal(want) {
			t.Errorf("ExpiresAt = %v, want %v", token.ExpiresAt, want)
		}
		if token.Revoked() {
			t.Error("свежий токен не должен быть отозван")
		}
		if token.LastUsedAt != nil {
			t.Errorf("LastUsedAt = %v, want nil", token.LastUsedAt)
		}
	})

	t.Run("идентификаторы разных токенов не совпадают", func(t *testing.T) {
		user := testUserForToken(t, RoleClient)

		first, err := NewToken(user, ttl, tokenNow)
		if err != nil {
			t.Fatalf("NewToken() unexpected error = %v", err)
		}

		second, err := NewToken(user, ttl, tokenNow)
		if err != nil {
			t.Fatalf("NewToken() unexpected error = %v", err)
		}

		if first.ID == second.ID {
			t.Errorf("ID обоих токенов = %q, want разные", first.ID)
		}
	})

	t.Run("время приводится к UTC", func(t *testing.T) {
		user := testUserForToken(t, RoleClient)
		zone := time.FixedZone("MSK", 3*60*60)

		token, err := NewToken(user, ttl, tokenNow.In(zone))
		if err != nil {
			t.Fatalf("NewToken() unexpected error = %v", err)
		}

		if token.IssuedAt.Location() != time.UTC || token.ExpiresAt.Location() != time.UTC {
			t.Errorf("время не в UTC: issued %v, expires %v", token.IssuedAt, token.ExpiresAt)
		}
	})

	t.Run("отказы", func(t *testing.T) {
		valid := testUserForToken(t, RoleAdmin)

		broken := *valid
		broken.Role = Role("wat")

		tests := []struct {
			name string
			user *User
			ttl  time.Duration
			want error
		}{
			{name: "нет пользователя", user: nil, ttl: ttl, want: ErrInvalidToken},
			{name: "нулевой срок", user: valid, ttl: 0, want: ErrInvalidToken},
			{name: "отрицательный срок", user: valid, ttl: -time.Minute, want: ErrInvalidToken},
			{name: "невалидный пользователь", user: &broken, ttl: ttl, want: ErrInvalidRole},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := NewToken(tt.user, tt.ttl, tokenNow)
				if !errors.Is(err, tt.want) {
					t.Errorf("NewToken() error = %v, want %v", err, tt.want)
				}
			})
		}
	})
}

func TestToken_Validate(t *testing.T) {
	valid := Token{
		ID:        "3f0c8f0e-9f60-4e0e-9d5f-1f6f1f6f1f6f",
		UserID:    "2f0c8f0e-9f60-4e0e-9d5f-1f6f1f6f1f6f",
		Role:      RoleClient,
		IssuedAt:  tokenNow,
		ExpiresAt: tokenNow.Add(time.Hour),
	}

	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error = %v", err)
	}

	tests := []struct {
		name  string
		alter func(token *Token)
		want  error
	}{
		{name: "пустой id", alter: func(t *Token) { t.ID = "" }, want: ErrInvalidToken},
		{name: "id не UUID", alter: func(t *Token) { t.ID = "not-a-uuid" }, want: ErrInvalidToken},
		{name: "пустой user_id", alter: func(t *Token) { t.UserID = "" }, want: ErrInvalidToken},
		{name: "неизвестная роль", alter: func(t *Token) { t.Role = Role("wat") }, want: ErrInvalidRole},
		{name: "нет времени выдачи", alter: func(t *Token) { t.IssuedAt = time.Time{} }, want: ErrInvalidToken},
		{
			name:  "срок не позже выдачи",
			alter: func(t *Token) { t.ExpiresAt = t.IssuedAt },
			want:  ErrInvalidToken,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := valid
			tt.alter(&token)

			if err := token.Validate(); !errors.Is(err, tt.want) {
				t.Errorf("Validate() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestToken_Valid(t *testing.T) {
	base := Token{
		ID:        "3f0c8f0e-9f60-4e0e-9d5f-1f6f1f6f1f6f",
		UserID:    "2f0c8f0e-9f60-4e0e-9d5f-1f6f1f6f1f6f",
		Role:      RoleClient,
		IssuedAt:  tokenNow,
		ExpiresAt: tokenNow.Add(time.Hour),
	}

	t.Run("действующий токен", func(t *testing.T) {
		if err := base.Valid(tokenNow.Add(30 * time.Minute)); err != nil {
			t.Errorf("Valid() error = %v, want nil", err)
		}
	})

	t.Run("момент истечения считается истёкшим", func(t *testing.T) {
		if err := base.Valid(base.ExpiresAt); !errors.Is(err, ErrTokenExpired) {
			t.Errorf("Valid() error = %v, want %v", err, ErrTokenExpired)
		}
	})

	t.Run("просроченный токен", func(t *testing.T) {
		if err := base.Valid(base.ExpiresAt.Add(time.Second)); !errors.Is(err, ErrTokenExpired) {
			t.Errorf("Valid() error = %v, want %v", err, ErrTokenExpired)
		}
	})

	t.Run("отозванный токен", func(t *testing.T) {
		token := base
		token.Revoke(tokenNow.Add(time.Minute))

		if !token.Revoked() {
			t.Fatal("Revoke() не отметил токен отозванным")
		}

		if err := token.Valid(tokenNow.Add(2 * time.Minute)); !errors.Is(err, ErrTokenRevoked) {
			t.Errorf("Valid() error = %v, want %v", err, ErrTokenRevoked)
		}
	})

	t.Run("повторный отзыв не меняет время", func(t *testing.T) {
		token := base
		token.Revoke(tokenNow.Add(time.Minute))
		first := *token.RevokedAt

		token.Revoke(tokenNow.Add(time.Hour))

		if !token.RevokedAt.Equal(first) {
			t.Errorf("RevokedAt = %v, want %v", token.RevokedAt, first)
		}
	})

	t.Run("Touch отмечает активность в UTC", func(t *testing.T) {
		token := base
		zone := time.FixedZone("MSK", 3*60*60)
		token.Touch(tokenNow.In(zone))

		if token.LastUsedAt == nil {
			t.Fatal("Touch() не проставил LastUsedAt")
		}

		if token.LastUsedAt.Location() != time.UTC || !token.LastUsedAt.Equal(tokenNow) {
			t.Errorf("LastUsedAt = %v, want %v в UTC", token.LastUsedAt, tokenNow)
		}
	})
}
