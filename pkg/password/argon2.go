package password

import (
	"crypto/pbkdf2"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

// argon2IDKey считает ключ Argon2id с указанными параметрами.
func argon2IDKey(password string, salt []byte, time, memory uint32, threads uint8) []byte {
	return argon2.IDKey([]byte(password), salt, time, memory, threads, passwordKeyLen)
}

// verifyArgon2 проверяет пароль по хешу Argon2id.
func verifyArgon2(password, encoded string) error {
	params, salt, key, err := decodeArgon2Hash(encoded)
	if err != nil {
		return err
	}

	actual := argon2.IDKey([]byte(password), salt, params.time, params.memory, params.threads, uint32(len(key)))

	if subtle.ConstantTimeCompare(actual, key) != 1 {
		return ErrMismatch
	}

	return nil
}

// verifyPBKDF2 проверяет пароль по хешу прежнего формата.
//
// Параметры (число итераций) берутся из самого хеша, поэтому старые пароли
// проверяются ровно так, как были посчитаны.
func verifyPBKDF2(password, encoded string) error {
	iterations, salt, key, err := decodePBKDF2Hash(encoded)
	if err != nil {
		return err
	}

	actual, err := pbkdf2.Key(sha256.New, password, salt, iterations, len(key))
	if err != nil {
		return fmt.Errorf("verify password: %w", err)
	}

	if subtle.ConstantTimeCompare(actual, key) != 1 {
		return ErrMismatch
	}

	return nil
}

// argon2Params — параметры Argon2id, с которыми посчитан хеш.
type argon2Params struct {
	memory  uint32
	time    uint32
	threads uint8
}

// currentArgon2Params возвращает параметры, заданные константами пакета.
func currentArgon2Params() argon2Params {
	return argon2Params{memory: argon2Memory, time: argon2Time, threads: argon2Threads}
}

// encodeArgon2Hash собирает строку хеша Argon2id (формат PHC).
func encodeArgon2Hash(salt, key []byte) string {
	params := currentArgon2Params()
	encode := base64.RawStdEncoding.EncodeToString

	return fmt.Sprintf("%s$v=%d$m=%d,t=%d,p=%d$%s$%s",
		hashAlgo, argon2Version, params.memory, params.time, params.threads, encode(salt), encode(key))
}

// decodeArgon2Hash разбирает строку хеша Argon2id.
func decodeArgon2Hash(encoded string) (argon2Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 {
		return argon2Params{}, nil, nil, fmt.Errorf("%w: %d parts, want 5", ErrInvalidHash, len(parts))
	}

	if parts[0] != hashAlgo {
		return argon2Params{}, nil, nil, fmt.Errorf("%w: unknown algorithm %q", ErrInvalidHash, parts[0])
	}

	version, err := strconv.Atoi(strings.TrimPrefix(parts[1], "v="))
	if err != nil || version != argon2Version {
		return argon2Params{}, nil, nil, fmt.Errorf("%w: version %q", ErrInvalidHash, parts[1])
	}

	params, err := parseArgon2Params(parts[2])
	if err != nil {
		return argon2Params{}, nil, nil, err
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(salt) == 0 {
		return argon2Params{}, nil, nil, fmt.Errorf("%w: salt %q", ErrInvalidHash, parts[3])
	}

	key, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(key) == 0 {
		return argon2Params{}, nil, nil, fmt.Errorf("%w: key %q", ErrInvalidHash, parts[4])
	}

	return params, salt, key, nil
}

// parseArgon2Params разбирает сегмент "m=...,t=...,p=...".
func parseArgon2Params(segment string) (argon2Params, error) {
	params := argon2Params{}

	for _, field := range strings.Split(segment, ",") {
		key, value, found := strings.Cut(field, "=")
		if !found {
			return argon2Params{}, fmt.Errorf("%w: params %q", ErrInvalidHash, segment)
		}

		parsed, err := strconv.ParseUint(value, 10, 32)
		if err != nil || parsed == 0 {
			return argon2Params{}, fmt.Errorf("%w: param %q", ErrInvalidHash, field)
		}

		switch key {
		case "m":
			params.memory = uint32(parsed)
		case "t":
			params.time = uint32(parsed)
		case "p":
			if parsed > 255 {
				return argon2Params{}, fmt.Errorf("%w: param %q", ErrInvalidHash, field)
			}

			params.threads = uint8(parsed)
		default:
			return argon2Params{}, fmt.Errorf("%w: param %q", ErrInvalidHash, field)
		}
	}

	if params.memory == 0 || params.time == 0 || params.threads == 0 {
		return argon2Params{}, fmt.Errorf("%w: params %q", ErrInvalidHash, segment)
	}

	return params, nil
}

// decodePBKDF2Hash разбирает строку хеша прежнего формата.
func decodePBKDF2Hash(encoded string) (int, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 {
		return 0, nil, nil, fmt.Errorf("%w: %d parts, want 4", ErrInvalidHash, len(parts))
	}

	if parts[0] != legacyHashAlgo {
		return 0, nil, nil, fmt.Errorf("%w: unknown algorithm %q", ErrInvalidHash, parts[0])
	}

	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations <= 0 {
		return 0, nil, nil, fmt.Errorf("%w: iterations %q", ErrInvalidHash, parts[1])
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil || len(salt) == 0 {
		return 0, nil, nil, fmt.Errorf("%w: salt %q", ErrInvalidHash, parts[2])
	}

	key, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(key) == 0 {
		return 0, nil, nil, fmt.Errorf("%w: key %q", ErrInvalidHash, parts[3])
	}

	return iterations, salt, key, nil
}

// prefix возвращает начало строки для сообщения об ошибке: сам хеш в лог
// целиком не попадает.
func prefix(encoded string) string {
	const limit = 16

	if len(encoded) <= limit {
		return encoded
	}

	return encoded[:limit] + "..."
}
