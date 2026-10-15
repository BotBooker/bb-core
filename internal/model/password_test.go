package model

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{name: "минимальная длина в символах", password: "пароль12", wantErr: false},
		{name: "длинный пароль", password: strings.Repeat("a", PasswordMaxLen), wantErr: false},
		{name: "пустой", password: "", wantErr: true},
		{name: "короче минимума", password: "1234567", wantErr: true},
		{name: "короче минимума в руках", password: "пароль1", wantErr: true},
		{name: "длиннее максимума", password: strings.Repeat("a", PasswordMaxLen+1), wantErr: true},
		{name: "максимум в байтах, но кириллицей", password: strings.Repeat("я", 130), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.password)

			if tt.wantErr && !errors.Is(err, ErrInvalidPassword) {
				t.Errorf("ValidatePassword() error = %v, want %v", err, ErrInvalidPassword)
			}

			if !tt.wantErr && err != nil {
				t.Errorf("ValidatePassword() unexpected error = %v", err)
			}
		})
	}
}

func TestHashPassword(t *testing.T) {
	const password = "correct horse battery staple"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() unexpected error = %v", err)
	}

	parts := strings.Split(hash, "$")
	if len(parts) != 5 {
		t.Fatalf("HashPassword() = %q, want 5 частей, разделённых $", hash)
	}

	if parts[0] != hashAlgo {
		t.Errorf("алгоритм = %q, want %q", parts[0], hashAlgo)
	}

	if parts[1] != "v=19" {
		t.Errorf("версия = %q, want %q", parts[1], "v=19")
	}

	if parts[2] != "m=19456,t=2,p=1" {
		t.Errorf("параметры = %q, want %q", parts[2], "m=19456,t=2,p=1")
	}

	if parts[3] == "" || parts[4] == "" {
		t.Errorf("HashPassword() = %q, want непустые соль и ключ", hash)
	}

	if strings.Contains(hash, password) {
		t.Errorf("хеш %q содержит открытый пароль", hash)
	}

	// Соль случайная: одинаковые пароли дают разные хеши.
	second, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() unexpected error = %v", err)
	}

	if hash == second {
		t.Error("два хеша одного пароля совпали: соль не случайна")
	}

	t.Run("пароль не проходит требования", func(t *testing.T) {
		if _, err := HashPassword("short"); !errors.Is(err, ErrInvalidPassword) {
			t.Errorf("HashPassword() error = %v, want %v", err, ErrInvalidPassword)
		}
	})
}

func TestVerifyPassword(t *testing.T) {
	const password = "correct horse battery staple"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() unexpected error = %v", err)
	}

	t.Run("верный пароль", func(t *testing.T) {
		if err := VerifyPassword(password, hash); err != nil {
			t.Errorf("VerifyPassword() unexpected error = %v", err)
		}
	})

	t.Run("неверный пароль", func(t *testing.T) {
		err := VerifyPassword("correct horse battery stapl", hash)
		if !errors.Is(err, ErrPasswordMismatch) {
			t.Errorf("VerifyPassword() error = %v, want %v", err, ErrPasswordMismatch)
		}
	})
}

func TestVerifyPassword_InvalidHash(t *testing.T) {
	tests := []struct {
		name string
		hash string
	}{
		{name: "пустой", hash: ""},
		{name: "не хватает частей", hash: "pbkdf2_sha256$210000$c2FsdA"},
		{name: "лишние части", hash: "pbkdf2_sha256$210000$c2FsdA$a2V5$extra"},
		{name: "неизвестный алгоритм", hash: "bcrypt$210000$c2FsdA$a2V5"},
		{name: "итерации не число", hash: "pbkdf2_sha256$many$c2FsdA$a2V5"},
		{name: "итерации отрицательные", hash: "pbkdf2_sha256$-1$c2FsdA$a2V5"},
		{name: "соль не base64", hash: "pbkdf2_sha256$210000$***$a2V5"},
		{name: "пустая соль", hash: "pbkdf2_sha256$210000$$a2V5"},
		{name: "ключ не base64", hash: "pbkdf2_sha256$210000$c2FsdA$***"},
		{name: "пустой ключ", hash: "pbkdf2_sha256$210000$c2FsdA$"},
		{name: "argon2 без частей", hash: "argon2id$v=19$m=19456,t=2,p=1$c2FsdA"},
		{name: "argon2 неизвестная версия", hash: "argon2id$v=18$m=19456,t=2,p=1$c2FsdA$a2V5"},
		{name: "argon2 нулевая память", hash: "argon2id$v=19$m=0,t=2,p=1$c2FsdA$a2V5"},
		{name: "argon2 неизвестный параметр", hash: "argon2id$v=19$m=19456,x=2,p=1$c2FsdA$a2V5"},
		{name: "argon2 соль не base64", hash: "argon2id$v=19$m=19456,t=2,p=1$***$a2V5"},
		{name: "argon2 ключ не base64", hash: "argon2id$v=19$m=19456,t=2,p=1$c2FsdA$***"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyPassword("correct horse battery staple", tt.hash)
			if !errors.Is(err, ErrInvalidPasswordHash) {
				t.Errorf("VerifyPassword() error = %v, want %v", err, ErrInvalidPasswordHash)
			}
		})
	}
}

func TestNeedsRehash(t *testing.T) {
	current, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword() unexpected error = %v", err)
	}

	tests := []struct {
		name string
		hash string
		want bool
	}{
		{name: "текущий Argon2id", hash: current, want: false},
		{
			name: "прежний PBKDF2",
			hash: "pbkdf2_sha256$210000$dQIjm0wTireOlDo8WomRug$+qvx5cWbyYARbq061iEy9aedaWfstobFn25SQIDvZbc",
			want: true,
		},
		{name: "Argon2id с другими параметрами", hash: "argon2id$v=19$m=1024,t=1,p=1$c2FsdA$a2V5", want: true},
		{name: "повреждённый Argon2id", hash: "argon2id$broken", want: true},
		{name: "неизвестный алгоритм", hash: "bcrypt$whatever", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NeedsRehash(tt.hash); got != tt.want {
				t.Errorf("NeedsRehash(%q) = %v, want %v", tt.hash, got, tt.want)
			}
		})
	}
}

func TestCredentials(t *testing.T) {
	const (
		password = "correct horse battery staple"
		email    = "ivan@example.com"
		userID   = "3f0c8f0e-9f60-4e0e-9d5f-1f6f1f6f1f6f"
		ttl      = 90 * 24 * time.Hour
	)

	now := time.Date(2026, time.October, 12, 10, 0, 0, 0, time.UTC)

	credentials, err := NewCredentials(userID, email, password, ttl, now)
	if err != nil {
		t.Fatalf("NewCredentials() unexpected error = %v", err)
	}

	if credentials.UserID != userID {
		t.Errorf("UserID = %q, want %q", credentials.UserID, userID)
	}
	if !credentials.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", credentials.CreatedAt, now)
	}
	if credentials.ProviderKey != email || credentials.Provider != CredentialProviderEmail {
		t.Errorf("Provider/ProviderKey = %q/%q, want %q/%q",
			credentials.Provider, credentials.ProviderKey, CredentialProviderEmail, email)
	}
	if strings.Contains(credentials.Hash, password) {
		t.Error("Hash содержит открытый пароль")
	}

	if err := credentials.Verify(password); err != nil {
		t.Errorf("Verify() unexpected error = %v", err)
	}

	if err := credentials.Verify("another password"); !errors.Is(err, ErrPasswordMismatch) {
		t.Errorf("Verify() error = %v, want %v", err, ErrPasswordMismatch)
	}

	t.Run("смена пароля", func(t *testing.T) {
		before := credentials.Hash
		later := now.Add(time.Hour)

		if err := credentials.SetPassword("new correct horse", ttl, later); err != nil {
			t.Fatalf("SetPassword() unexpected error = %v", err)
		}

		if credentials.Hash == before {
			t.Error("хеш не изменился")
		}
		if want := later.Add(ttl); !credentials.ExpiresAt.Equal(want) {
			t.Errorf("ExpiresAt = %v, want %v", credentials.ExpiresAt, want)
		}
		if err := credentials.Verify("new correct horse"); err != nil {
			t.Errorf("Verify() нового пароля unexpected error = %v", err)
		}
		if err := credentials.Verify(password); !errors.Is(err, ErrPasswordMismatch) {
			t.Errorf("старый пароль принят: error = %v", err)
		}
	})

	t.Run("отказы", func(t *testing.T) {
		if _, err := NewCredentials("not-a-uuid", email, password, ttl, now); !errors.Is(err, ErrInvalidUUID) {
			t.Errorf("NewCredentials() error = %v, want %v", err, ErrInvalidUUID)
		}

		if _, err := NewCredentials(userID, email, "short", ttl, now); !errors.Is(err, ErrInvalidPassword) {
			t.Errorf("NewCredentials() error = %v, want %v", err, ErrInvalidPassword)
		}

		if err := credentials.SetPassword("short", ttl, now); !errors.Is(err, ErrInvalidPassword) {
			t.Errorf("SetPassword() error = %v, want %v", err, ErrInvalidPassword)
		}
	})
}
