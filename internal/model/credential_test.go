package model

import (
	"errors"
	"testing"
	"time"
)

// testCredentialEmail — адрес, под которым заводится локальная учётная запись.
const testCredentialEmail = "ivan@example.com"

// testCredentialUserID — владелец учётных данных в тестах.
const testCredentialUserID = "3f0c8f0e-9f60-4e0e-9d5f-1f6f1f6f1f6f"

// testPasswordHash — строка хеша в формате pkg/password. Модель хранит хеш как
// есть: разбирает и проверяет его порт domain.PasswordHasher, поэтому для
// тестов достаточно корректной на вид строки.
const testPasswordHash = "argon2id$v=19$m=19456,t=2,p=1$c2FsdA$aGFzaA"

// newTestCredentials создаёт учётные данные для тестов.
func newTestCredentials(t *testing.T, ttl time.Duration, now time.Time) *Credentials {
	t.Helper()

	credentials, err := NewCredentials(testCredentialUserID, testCredentialEmail, testPasswordHash, ttl, now)
	if err != nil {
		t.Fatalf("NewCredentials() unexpected error = %v", err)
	}

	return credentials
}

// TestCredentials_Expiry проверяет срок жизни пароля: он записывается в
// учётные данные (expires_at) при создании и перезапускается при смене пароля,
// ноль отключает проверку, а замена хеша (SetHash) срок не меняет.
func TestCredentials_Expiry(t *testing.T) {
	now := time.Date(2026, time.October, 12, 10, 0, 0, 0, time.UTC)
	ttl := 90 * 24 * time.Hour

	credentials := newTestCredentials(t, ttl, now)

	t.Run("ExpiresAt считается от времени создания", func(t *testing.T) {
		if got, want := credentials.ExpiresAt, now.Add(ttl); !got.Equal(want) {
			t.Errorf("ExpiresAt = %v, want %v", got, want)
		}
	})

	t.Run("провайдер и ключ провайдера", func(t *testing.T) {
		if credentials.Provider != CredentialProviderEmail {
			t.Errorf("Provider = %q, want %q", credentials.Provider, CredentialProviderEmail)
		}
		if credentials.ProviderKey != testCredentialEmail {
			t.Errorf("ProviderKey = %q, want %q", credentials.ProviderKey, testCredentialEmail)
		}
		if err := ValidateUUID(credentials.ID); err != nil {
			t.Errorf("ID = %q: %v", credentials.ID, err)
		}
		if credentials.Hash != testPasswordHash {
			t.Errorf("Hash = %q, want %q", credentials.Hash, testPasswordHash)
		}
		if !credentials.IsActive || credentials.IsDelete {
			t.Errorf("IsActive/IsDelete = %t/%t, want true/false", credentials.IsActive, credentials.IsDelete)
		}
	})

	t.Run("до истечения срока пароль действует", func(t *testing.T) {
		if credentials.Expired(now.Add(ttl - time.Second)) {
			t.Error("Expired() = true за секунду до истечения, want false")
		}
	})

	t.Run("в момент истечения пароль уже не действует", func(t *testing.T) {
		if !credentials.Expired(now.Add(ttl)) {
			t.Error("Expired() = false в момент истечения, want true")
		}
	})

	t.Run("нулевой и отрицательный срок отключают проверку", func(t *testing.T) {
		far := now.Add(100 * 365 * 24 * time.Hour)

		endless := newTestCredentials(t, 0, now)
		if !endless.ExpiresAt.IsZero() {
			t.Errorf("ExpiresAt = %v при ttl = 0, want нулевой момент", endless.ExpiresAt)
		}
		if endless.Expired(far) {
			t.Error("Expired() = true при ttl = 0, want false")
		}

		negative := newTestCredentials(t, -time.Hour, now)
		if negative.Expired(far) {
			t.Error("Expired() = true при отрицательном ttl, want false")
		}
	})

	t.Run("смена пароля сбрасывает срок", func(t *testing.T) {
		later := now.Add(30 * 24 * time.Hour)

		if err := credentials.SetPasswordHash(testPasswordHash+"new", ttl, later); err != nil {
			t.Fatalf("SetPasswordHash() unexpected error = %v", err)
		}

		if want := later.Add(ttl); !credentials.ExpiresAt.Equal(want) {
			t.Errorf("ExpiresAt = %v, want %v", credentials.ExpiresAt, want)
		}
		if credentials.Expired(later.Add(ttl - time.Second)) {
			t.Error("после смены пароля Expired() = true до истечения, want false")
		}
		if !credentials.Expired(later.Add(ttl)) {
			t.Error("после смены пароля Expired() = false в момент истечения, want true")
		}
	})

	t.Run("смена пароля снимает отзыв", func(t *testing.T) {
		revoked := newTestCredentials(t, ttl, now)
		revokedAt := now.Add(time.Hour)
		revoked.RevokedAt = &revokedAt

		if !revoked.Revoked() {
			t.Fatal("Revoked() = false, want true")
		}

		if err := revoked.SetPasswordHash(testPasswordHash+"revoked", ttl, now); err != nil {
			t.Fatalf("SetPasswordHash() unexpected error = %v", err)
		}

		if revoked.Revoked() {
			t.Error("после смены пароля Revoked() = true, want false")
		}
	})
}

// TestCredentials_SetHash проверяет замену хеша без смены пароля: срок действия
// не сбрасывается, пустой хеш отвергается.
func TestCredentials_SetHash(t *testing.T) {
	now := time.Date(2026, time.October, 12, 10, 0, 0, 0, time.UTC)
	ttl := 90 * 24 * time.Hour

	credentials := newTestCredentials(t, ttl, now)
	expiresAt := credentials.ExpiresAt

	t.Run("новый хеш сохраняется, срок не меняется", func(t *testing.T) {
		updated := testPasswordHash + "upgraded"

		if err := credentials.SetHash(updated); err != nil {
			t.Fatalf("SetHash() unexpected error = %v", err)
		}

		if credentials.Hash != updated {
			t.Errorf("Hash = %q, want %q", credentials.Hash, updated)
		}
		if !credentials.ExpiresAt.Equal(expiresAt) {
			t.Errorf("SetHash() ExpiresAt = %v, want %v", credentials.ExpiresAt, expiresAt)
		}
	})

	t.Run("пустой хеш отвергается", func(t *testing.T) {
		if err := credentials.SetHash(" "); !errors.Is(err, ErrInvalidCredential) {
			t.Errorf("SetHash() error = %v, want %v", err, ErrInvalidCredential)
		}
	})

	t.Run("смена пароля отвергает пустой хеш", func(t *testing.T) {
		if err := credentials.SetPasswordHash("", ttl, now); !errors.Is(err, ErrInvalidCredential) {
			t.Errorf("SetPasswordHash() error = %v, want %v", err, ErrInvalidCredential)
		}
	})
}

// TestCredentials_TouchLastLogin проверяет отметку входа: время сохраняется в
// UTC, чтобы аудит не зависел от часового пояса процесса.
func TestCredentials_TouchLastLogin(t *testing.T) {
	credentials := newTestCredentials(t, 0, time.Now())
	loginAt := time.Date(2026, time.October, 12, 10, 0, 0, 0, time.FixedZone("MSK", 3*60*60))

	credentials.TouchLastLogin(loginAt)

	if credentials.LastLoginAt == nil {
		t.Fatal("LastLoginAt = nil, want время входа")
	}
	if want := loginAt.UTC(); !credentials.LastLoginAt.Equal(want) {
		t.Errorf("LastLoginAt = %v, want %v", credentials.LastLoginAt, want)
	}
}

// TestNewCredentials_Rejects проверяет отказы конструктора: без валидного
// владельца, email и хеша учётные данные не создаются.
func TestNewCredentials_Rejects(t *testing.T) {
	now := time.Date(2026, time.October, 12, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		userID string
		email  string
		hash   string
		want   error
	}{
		{
			name:   "id не UUID",
			userID: "not-a-uuid",
			email:  testCredentialEmail,
			hash:   testPasswordHash,
			want:   ErrInvalidUUID,
		},
		{
			name:   "email невалиден",
			userID: testCredentialUserID,
			email:  "not-an-email",
			hash:   testPasswordHash,
			want:   ErrInvalidEmail,
		},
		{
			name:   "пустой хеш",
			userID: testCredentialUserID,
			email:  testCredentialEmail,
			hash:   " ",
			want:   ErrInvalidCredential,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewCredentials(tt.userID, tt.email, tt.hash, 0, now)
			if !errors.Is(err, tt.want) {
				t.Errorf("NewCredentials() error = %v, want %v", err, tt.want)
			}
		})
	}
}

// TestCredentials_Validate проверяет инварианты хранимой записи учётных данных.
func TestCredentials_Validate(t *testing.T) {
	valid := Credentials{
		ID:          "4f0c8f0e-9f60-4e0e-9d5f-1f6f1f6f1f6f",
		UserID:      testCredentialUserID,
		Provider:    CredentialProviderEmail,
		ProviderKey: testCredentialEmail,
		Hash:        testPasswordHash,
		CreatedAt:   time.Date(2026, time.October, 12, 10, 0, 0, 0, time.UTC),
		IsActive:    true,
	}

	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error = %v", err)
	}

	tests := []struct {
		name  string
		alter func(credentials *Credentials)
		want  error
	}{
		{name: "пустой id", alter: func(c *Credentials) { c.ID = "" }, want: ErrInvalidCredential},
		{name: "id не UUID", alter: func(c *Credentials) { c.ID = "not-a-uuid" }, want: ErrInvalidCredential},
		{name: "пустой user_id", alter: func(c *Credentials) { c.UserID = "" }, want: ErrInvalidCredential},
		{name: "пустой провайдер", alter: func(c *Credentials) { c.Provider = " " }, want: ErrInvalidCredential},
		{name: "пустой ключ провайдера", alter: func(c *Credentials) { c.ProviderKey = "" }, want: ErrInvalidCredential},
		{name: "пустой хеш", alter: func(c *Credentials) { c.Hash = "" }, want: ErrInvalidCredential},
		{name: "нет времени создания", alter: func(c *Credentials) { c.CreatedAt = time.Time{} }, want: ErrInvalidCredential},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			credentials := valid
			tt.alter(&credentials)

			if err := credentials.Validate(); !errors.Is(err, tt.want) {
				t.Errorf("Validate() error = %v, want %v", err, tt.want)
			}
		})
	}
}
