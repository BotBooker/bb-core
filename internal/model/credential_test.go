package model

import (
	"errors"
	"testing"
	"time"
)

// TestCredentials_Expiry проверяет динамический срок жизни пароля: момент
// истечения считается от времени последнего изменения, ноль отключает
// проверку, а пересчёт хеша (Rehash) срок не сбрасывает.
func TestCredentials_Expiry(t *testing.T) {
	const (
		password = "correct horse battery staple"
		userID   = "3f0c8f0e-9f60-4e0e-9d5f-1f6f1f6f1f6f"
	)

	now := time.Date(2026, time.October, 12, 10, 0, 0, 0, time.UTC)
	ttl := 90 * 24 * time.Hour

	credentials, err := NewCredentials(userID, password, now)
	if err != nil {
		t.Fatalf("NewCredentials() unexpected error = %v", err)
	}

	t.Run("ExpiresAt считается от времени изменения пароля", func(t *testing.T) {
		if got, want := credentials.ExpiresAt(ttl), now.Add(ttl); !got.Equal(want) {
			t.Errorf("ExpiresAt() = %v, want %v", got, want)
		}
	})

	t.Run("до истечения срока пароль действует", func(t *testing.T) {
		if credentials.Expired(now.Add(ttl-time.Second), ttl) {
			t.Error("Expired() = true за секунду до истечения, want false")
		}
	})

	t.Run("в момент истечения пароль уже не действует", func(t *testing.T) {
		if !credentials.Expired(now.Add(ttl), ttl) {
			t.Error("Expired() = false в момент истечения, want true")
		}
	})

	t.Run("нулевой и отрицательный срок отключают проверку", func(t *testing.T) {
		far := now.Add(100 * 365 * 24 * time.Hour)

		if credentials.Expired(far, 0) {
			t.Error("Expired() = true при ttl = 0, want false")
		}

		if credentials.Expired(far, -time.Hour) {
			t.Error("Expired() = true при отрицательном ttl, want false")
		}
	})

	t.Run("смена пароля сбрасывает срок", func(t *testing.T) {
		later := now.Add(30 * 24 * time.Hour)

		if err := credentials.SetPassword("new correct horse", later); err != nil {
			t.Fatalf("SetPassword() unexpected error = %v", err)
		}

		if credentials.Expired(later.Add(ttl-time.Second), ttl) {
			t.Error("после смены пароля Expired() = true, want false")
		}
	})

	t.Run("Rehash не сбрасывает срок и меняет хеш", func(t *testing.T) {
		before, updatedAt := credentials.Hash, credentials.UpdatedAt

		if err := credentials.Rehash("new correct horse"); err != nil {
			t.Fatalf("Rehash() unexpected error = %v", err)
		}

		if credentials.Hash == before {
			t.Error("Rehash() не изменил хеш")
		}

		if !credentials.UpdatedAt.Equal(updatedAt) {
			t.Errorf("Rehash() UpdatedAt = %v, want %v", credentials.UpdatedAt, updatedAt)
		}

		if err := credentials.Verify("new correct horse"); err != nil {
			t.Errorf("Verify() после Rehash() unexpected error = %v", err)
		}
	})

	t.Run("NeedsRehash на свежем Argon2id = false", func(t *testing.T) {
		fresh, err := NewCredentials(userID, password, now)
		if err != nil {
			t.Fatalf("NewCredentials() unexpected error = %v", err)
		}

		if fresh.NeedsRehash() {
			t.Error("NeedsRehash() = true для свежего Argon2id-хеша, want false")
		}
	})

	t.Run("NeedsRehash на прежнем формате = true", func(t *testing.T) {
		legacy := Credentials{
			UserID: userID,
			Hash:   "pbkdf2_sha256$210000$dQIjm0wTireOlDo8WomRug$+qvx5cWbyYARbq061iEy9aedaWfstobFn25SQIDvZbc",
		}

		if !legacy.NeedsRehash() {
			t.Error("NeedsRehash() = false для PBKDF2-хеша, want true")
		}
	})

	t.Run("Rehash отвергает слабый пароль", func(t *testing.T) {
		if err := credentials.Rehash("short"); !errors.Is(err, ErrInvalidPassword) {
			t.Errorf("Rehash() error = %v, want %v", err, ErrInvalidPassword)
		}
	})
}
