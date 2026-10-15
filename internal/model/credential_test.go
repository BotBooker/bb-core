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

// TestCredentials_Expiry проверяет срок жизни пароля: он записывается в
// учётные данные (expires_at) при создании и сбрасывается при смене пароля,
// ноль отключает проверку, а пересчёт хеша (Rehash) срок не меняет.
func TestCredentials_Expiry(t *testing.T) {
	const password = "correct horse battery staple"

	now := time.Date(2026, time.October, 12, 10, 0, 0, 0, time.UTC)
	ttl := 90 * 24 * time.Hour

	credentials, err := NewCredentials(testCredentialUserID, testCredentialEmail, password, ttl, now)
	if err != nil {
		t.Fatalf("NewCredentials() unexpected error = %v", err)
	}

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

		endless, err := NewCredentials(testCredentialUserID, testCredentialEmail, password, 0, now)
		if err != nil {
			t.Fatalf("NewCredentials() unexpected error = %v", err)
		}

		if !endless.ExpiresAt.IsZero() {
			t.Errorf("ExpiresAt = %v при ttl = 0, want нулевой момент", endless.ExpiresAt)
		}

		if endless.Expired(far) {
			t.Error("Expired() = true при ttl = 0, want false")
		}

		negative, err := NewCredentials(testCredentialUserID, testCredentialEmail, password, -time.Hour, now)
		if err != nil {
			t.Fatalf("NewCredentials() unexpected error = %v", err)
		}

		if negative.Expired(far) {
			t.Error("Expired() = true при отрицательном ttl, want false")
		}
	})

	t.Run("смена пароля сбрасывает срок", func(t *testing.T) {
		later := now.Add(30 * 24 * time.Hour)

		if err := credentials.SetPassword("new correct horse", ttl, later); err != nil {
			t.Fatalf("SetPassword() unexpected error = %v", err)
		}

		if want := later.Add(ttl); !credentials.ExpiresAt.Equal(want) {
			t.Errorf("ExpiresAt = %v, want %v", credentials.ExpiresAt, want)
		}

		if credentials.Expired(later.Add(ttl - time.Second)) {
			t.Error("после смены пароля Expired() = true, want false")
		}
	})
}

// TestCredentials_Password проверяет работу с хешем пароля: пересчёт устаревшего
// хеша, проверку пароля и отказы конструктора.
func TestCredentials_Password(t *testing.T) {
	const password = "correct horse battery staple"

	now := time.Date(2026, time.October, 12, 10, 0, 0, 0, time.UTC)
	ttl := 90 * 24 * time.Hour

	credentials, err := NewCredentials(testCredentialUserID, testCredentialEmail, password, ttl, now)
	if err != nil {
		t.Fatalf("NewCredentials() unexpected error = %v", err)
	}

	t.Run("NeedsRehash на свежем Argon2id = false", func(t *testing.T) {
		if credentials.NeedsRehash() {
			t.Error("NeedsRehash() = true для свежего Argon2id-хеша, want false")
		}
	})

	t.Run("NeedsRehash на прежнем формате = true", func(t *testing.T) {
		legacy := Credentials{
			ID:          "4f0c8f0e-9f60-4e0e-9d5f-1f6f1f6f1f6f",
			UserID:      testCredentialUserID,
			Provider:    CredentialProviderEmail,
			ProviderKey: testCredentialEmail,
			Hash:        "pbkdf2_sha256$210000$dQIjm0wTireOlDo8WomRug$+qvx5cWbyYARbq061iEy9aedaWfstobFn25SQIDvZbc",
			CreatedAt:   now,
			ExpiresAt:   now.Add(ttl),
			IsActive:    true,
		}

		if !legacy.NeedsRehash() {
			t.Error("NeedsRehash() = false для PBKDF2-хеша, want true")
		}

		if err := legacy.Validate(); err != nil {
			t.Errorf("Validate() unexpected error = %v", err)
		}
	})

	t.Run("Rehash меняет хеш и не сбрасывает срок", func(t *testing.T) {
		before, expiresAt := credentials.Hash, credentials.ExpiresAt

		if err := credentials.Rehash("new correct horse"); err != nil {
			t.Fatalf("Rehash() unexpected error = %v", err)
		}

		if credentials.Hash == before {
			t.Error("Rehash() не изменил хеш")
		}

		if !credentials.ExpiresAt.Equal(expiresAt) {
			t.Errorf("Rehash() ExpiresAt = %v, want %v", credentials.ExpiresAt, expiresAt)
		}

		if err := credentials.Verify("new correct horse"); err != nil {
			t.Errorf("Verify() после Rehash() unexpected error = %v", err)
		}
	})

	t.Run("Verify отвергает неверный пароль", func(t *testing.T) {
		if err := credentials.Verify("another correct horse"); !errors.Is(err, ErrPasswordMismatch) {
			t.Errorf("Verify() error = %v, want %v", err, ErrPasswordMismatch)
		}
	})

	t.Run("TouchLastLogin отмечает вход в UTC", func(t *testing.T) {
		credentials.TouchLastLogin(now)

		if credentials.LastLoginAt == nil || !credentials.LastLoginAt.Equal(now) {
			t.Errorf("LastLoginAt = %v, want %v", credentials.LastLoginAt, now)
		}
	})

	t.Run("Rehash отвергает слабый пароль", func(t *testing.T) {
		if err := credentials.Rehash("short"); !errors.Is(err, ErrInvalidPassword) {
			t.Errorf("Rehash() error = %v, want %v", err, ErrInvalidPassword)
		}
	})

	t.Run("отказы создания", func(t *testing.T) {
		if _, err := NewCredentials("not-a-uuid", testCredentialEmail, password, ttl, now); !errors.Is(err, ErrInvalidUUID) {
			t.Errorf("NewCredentials() error = %v, want %v", err, ErrInvalidUUID)
		}

		if _, err := NewCredentials(testCredentialUserID, "not-an-email", password, ttl, now); !errors.Is(err, ErrInvalidEmail) {
			t.Errorf("NewCredentials() error = %v, want %v", err, ErrInvalidEmail)
		}

		if _, err := NewCredentials(testCredentialUserID, testCredentialEmail, "short", ttl, now); !errors.Is(err, ErrInvalidPassword) {
			t.Errorf("NewCredentials() error = %v, want %v", err, ErrInvalidPassword)
		}
	})
}

// TestCredentials_Validate проверяет инварианты хранимой записи учётных данных.
func TestCredentials_Validate(t *testing.T) {
	valid := Credentials{
		ID:          "4f0c8f0e-9f60-4e0e-9d5f-1f6f1f6f1f6f",
		UserID:      testCredentialUserID,
		Provider:    CredentialProviderEmail,
		ProviderKey: testCredentialEmail,
		Hash:        "argon2id$v=19$m=19456,t=2,p=1$c2FsdA$aGFzaA",
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
