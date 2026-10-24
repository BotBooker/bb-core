package password

import (
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// testPassword — пароль, на котором проверяются хеширование и проверка.
const testPassword = "correct horse battery staple"

// TestHasher_HashAndVerify проверяет основной сценарий порта: пароль
// хешируется Argon2id, свой хеш проверяется успешно, чужой — отвергается.
func TestHasher_HashAndVerify(t *testing.T) {
	hasher := NewHasher()

	encoded, err := hasher.Hash(testPassword)
	if err != nil {
		t.Fatalf("Hash() unexpected error = %v", err)
	}

	t.Run("формат хеша", func(t *testing.T) {
		if !strings.HasPrefix(encoded, hashAlgo+"$v=") {
			t.Errorf("Hash() = %q, want префикс %q", encoded, hashAlgo+"$v=")
		}

		if _, _, _, err := decodeArgon2Hash(encoded); err != nil {
			t.Errorf("Hash() = %q не разбирается: %v", encoded, err)
		}
	})

	t.Run("соль случайна", func(t *testing.T) {
		other, err := hasher.Hash(testPassword)
		if err != nil {
			t.Fatalf("Hash() unexpected error = %v", err)
		}

		if other == encoded {
			t.Error("два хеша одного пароля совпали: соль не случайна")
		}
	})

	t.Run("верный пароль проходит", func(t *testing.T) {
		if err := hasher.Verify(testPassword, encoded); err != nil {
			t.Errorf("Verify() unexpected error = %v", err)
		}
	})

	t.Run("неверный пароль отвергается", func(t *testing.T) {
		if err := hasher.Verify("another correct horse", encoded); !errors.Is(err, ErrMismatch) {
			t.Errorf("Verify() error = %v, want %v", err, ErrMismatch)
		}
	})

	t.Run("свежий хеш не требует пересчёта", func(t *testing.T) {
		if hasher.NeedsRehash(encoded) {
			t.Error("NeedsRehash() = true для свежего хеша, want false")
		}
	})
}

// TestHasher_HashRejects проверяет технические отказы хеширования: домен
// проверяет политику пароля, а здесь остаются пустой и слишком длинный вход.
func TestHasher_HashRejects(t *testing.T) {
	hasher := NewHasher()

	if _, err := hasher.Hash(""); !errors.Is(err, ErrEmptyPassword) {
		t.Errorf("Hash(\"\") error = %v, want %v", err, ErrEmptyPassword)
	}

	tooLong := strings.Repeat("a", maxPasswordLen+1)
	if _, err := hasher.Hash(tooLong); !errors.Is(err, ErrPasswordTooLong) {
		t.Errorf("Hash(длинный пароль) error = %v, want %v", err, ErrPasswordTooLong)
	}
}

// testPBKDF2Hash собирает хеш прежнего формата: так выглядят пароли,
// сохранённые до перехода на Argon2id.
func testPBKDF2Hash(t *testing.T, password string, iterations int) string {
	t.Helper()

	salt := []byte("legacy-salt-0001")

	key, err := pbkdf2.Key(sha256.New, password, salt, iterations, passwordKeyLen)
	if err != nil {
		t.Fatalf("pbkdf2.Key() unexpected error = %v", err)
	}

	return fmt.Sprintf("%s$%d$%s$%s", legacyHashAlgo, iterations,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key))
}

// TestHasher_VerifyLegacy проверяет совместимость: хеши PBKDF2, сохранённые
// прежними версиями, продолжают проверяться и помечаются для пересчёта.
func TestHasher_VerifyLegacy(t *testing.T) {
	hasher := NewHasher()
	encoded := testPBKDF2Hash(t, testPassword, 1000)

	t.Run("верный пароль проходит", func(t *testing.T) {
		if err := hasher.Verify(testPassword, encoded); err != nil {
			t.Errorf("Verify() unexpected error = %v", err)
		}
	})

	t.Run("неверный пароль отвергается", func(t *testing.T) {
		if err := hasher.Verify("another correct horse", encoded); !errors.Is(err, ErrMismatch) {
			t.Errorf("Verify() error = %v, want %v", err, ErrMismatch)
		}
	})

	t.Run("хеш требует пересчёта", func(t *testing.T) {
		if !hasher.NeedsRehash(encoded) {
			t.Error("NeedsRehash() = false для PBKDF2-хеша, want true")
		}
	})
}

// TestHasher_VerifyInvalidHash проверяет отказы на повреждённых и неизвестных
// хешах: такой хеш нельзя проверить, и это отдельная ошибка (ErrInvalidHash),
// а не «пароль не подошёл».
func TestHasher_VerifyInvalidHash(t *testing.T) {
	hasher := NewHasher()

	tests := []struct {
		name    string
		encoded string
	}{
		{name: "неизвестный алгоритм", encoded: "md5$deadbeef"},
		{name: "мало частей", encoded: "argon2id$v=19$m=19456,t=2,p=1$c2FsdA"},
		{name: "другая версия", encoded: "argon2id$v=18$m=19456,t=2,p=1$c2FsdA$aGFzaA"},
		{name: "нулевой параметр", encoded: "argon2id$v=19$m=0,t=2,p=1$c2FsdA$aGFzaA"},
		{name: "неизвестный параметр", encoded: "argon2id$v=19$m=19456,t=2,x=1$c2FsdA$aGFzaA"},
		{name: "соль не base64", encoded: "argon2id$v=19$m=19456,t=2,p=1$!!!$aGFzaA"},
		{name: "пустой ключ", encoded: "argon2id$v=19$m=19456,t=2,p=1$c2FsdA$"},
		{name: "пустые итерации", encoded: "pbkdf2_sha256$0$c2FsdA$aGFzaA"},
		{name: "мало частей у PBKDF2", encoded: "pbkdf2_sha256$1000$c2FsdA"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := hasher.Verify(testPassword, tt.encoded); !errors.Is(err, ErrInvalidHash) {
				t.Errorf("Verify() error = %v, want %v", err, ErrInvalidHash)
			}

			if !hasher.NeedsRehash(tt.encoded) {
				t.Error("NeedsRehash() = false для повреждённого хеша, want true")
			}
		})
	}
}

// TestHasher_NeedsRehash проверяет, что хеш с устаревшими параметрами Argon2id
// всё ещё проверяется, но помечается для пересчёта: параметры берутся из самого
// хеша, поэтому старые пароли не ломаются.
func TestHasher_NeedsRehash(t *testing.T) {
	hasher := NewHasher()

	salt := []byte("outdated-salt-01")
	key := argon2IDKey(testPassword, salt, 1, 1024, 1)

	encoded := fmt.Sprintf("%s$v=%d$m=%d,t=%d,p=%d$%s$%s", hashAlgo, argon2Version, 1024, 1, 1,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key))

	if err := hasher.Verify(testPassword, encoded); err != nil {
		t.Fatalf("Verify() unexpected error = %v", err)
	}

	if !hasher.NeedsRehash(encoded) {
		t.Error("NeedsRehash() = false для хеша с устаревшими параметрами, want true")
	}
}
