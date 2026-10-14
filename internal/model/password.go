package model

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Требования к паролю и параметры хеширования.
//
// Пароли хеширует Argon2id — современный алгоритм, победитель Password Hashing
// Competition. Он намеренно медленный и «жадный» до памяти: подбор пароля
// требует заметной памяти на каждую попытку, поэтому перебор на GPU/ASIC
// становится невыгодным. Устаревшие алгоритмы (MD5, SHA1/SHA256 без растяжения
// ключа) не используются вовсе.
//
// Алгоритм и его параметры хранятся в самой строке хеша, поэтому их изменение
// не ломает уже сохранённые пароли: старые хеши продолжают проверяться, а при
// следующем успешном входе пересчитываются новыми параметрами (см. NeedsRehash).
const (
	// PasswordMinLen — минимальная длина пароля в символах (рунах).
	PasswordMinLen = 8
	// PasswordMaxLen — максимальная длина пароля в байтах: ограничивает работу,
	// которую злоумышленник может заставить сервер выполнить одним запросом.
	PasswordMaxLen = 128
	// passwordSaltLen — длина соли в байтах: своя для каждого пароля, поэтому
	// одинаковые пароли дают разные хеши.
	passwordSaltLen = 16
	// passwordKeyLen — длина ключа (хеша) в байтах.
	passwordKeyLen = 32
	// hashAlgo — имя алгоритма хеширования пароля в закодированном хеше.
	hashAlgo = "argon2id"
	// argon2Version — версия Argon2. Хранится в хеше вместе с параметрами,
	// поэтому обновление библиотеки не ломает уже сохранённые пароли.
	argon2Version = argon2.Version
	// argon2Memory — сколько KiB памяти использует один расчёт (19 MiB —
	// рекомендация OWASP для Argon2id).
	argon2Memory = 19 * 1024
	// argon2Time — число проходов (iterations).
	argon2Time = 2
	// argon2Threads — число потоков, которыми считается хеш.
	argon2Threads = 1
	// legacyHashAlgo — алгоритм прежних версий сервиса (PBKDF2-HMAC-SHA256).
	// Поддерживается только для проверки уже сохранённых хешей: новые пароли
	// всегда считаются Argon2id.
	legacyHashAlgo = "pbkdf2_sha256"
)

// Ошибки пароля.
var (
	// ErrInvalidPassword — пароль не проходит требования модели.
	ErrInvalidPassword = errors.New("user password is invalid")
	// ErrPasswordMismatch — пароль не совпадает с сохранённым хешем.
	ErrPasswordMismatch = errors.New("user password does not match")
	// ErrInvalidPasswordHash — хранимый хеш повреждён или посчитан неизвестным
	// алгоритмом: проверить по нему пароль нельзя.
	ErrInvalidPasswordHash = errors.New("stored password hash is invalid")
)

// ValidatePassword проверяет требования к паролю: не короче PasswordMinLen
// символов и не длиннее PasswordMaxLen байт.
//
// Правила намеренно простые: сложность пароля проверяет не сервис, а разумная
// длина — рекомендация NIST SP 800-63B. Пустой пароль невалиден.
func ValidatePassword(password string) error {
	if runes := len([]rune(password)); runes < PasswordMinLen {
		return fmt.Errorf("%w: %d characters is less than %d", ErrInvalidPassword, runes, PasswordMinLen)
	}

	if len(password) > PasswordMaxLen {
		return fmt.Errorf("%w: %d bytes is more than %d", ErrInvalidPassword, len(password), PasswordMaxLen)
	}

	return nil
}

// HashPassword считает хеш пароля Argon2id в формате
//
//	argon2id$v=19$m=19456,t=2,p=1$<соль>$<ключ>
//
// Алгоритм и его параметры хранятся вместе с хешем: их смена не ломает уже
// сохранённые пароли. Соль случайная для каждого пароля, поэтому одинаковые
// пароли разных пользователей дают разные хеши. Открытый пароль нигде не
// сохраняется.
func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}

	salt := make([]byte, passwordSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, argon2Time, argon2Memory, argon2Threads, passwordKeyLen)

	return encodeArgon2Hash(salt, key), nil
}

// VerifyPassword сравнивает пароль с закодированным хешем и возвращает nil,
// если пароль подходит.
//
// Понимаются оба формата: текущий Argon2id и прежний PBKDF2-HMAC-SHA256
// (legacyHashAlgo) — иначе после перехода на новый алгоритм уже сохранённые
// пароли перестали бы проверяться. Возвращает ErrPasswordMismatch, если пароль
// не подходит, и ErrInvalidPasswordHash, если хеш нельзя разобрать. Сравнение
// ключей — за постоянное время (crypto/subtle): по времени ответа нельзя
// узнать, сколько байт хеша совпало.
func VerifyPassword(password, encoded string) error {
	switch {
	case strings.HasPrefix(encoded, hashAlgo+"$"):
		return verifyArgon2(password, encoded)
	case strings.HasPrefix(encoded, legacyHashAlgo+"$"):
		return verifyPBKDF2(password, encoded)
	default:
		return fmt.Errorf("%w: unknown algorithm in %q", ErrInvalidPasswordHash, prefix(encoded))
	}
}

// NeedsRehash сообщает, что хеш посчитан устаревшим алгоритмом или с
// параметрами, отличными от текущих, и его стоит пересчитать.
//
// Вызывается после успешной проверки пароля: открытый пароль в этот момент
// известен, поэтому хеш можно обновить, не заставляя пользователя менять
// пароль (см. AuthService — «прозрачное» обновление хеша при входе).
func NeedsRehash(encoded string) bool {
	if !strings.HasPrefix(encoded, hashAlgo+"$") {
		return true
	}

	params, _, key, err := decodeArgon2Hash(encoded)
	if err != nil {
		return true
	}

	return params != currentArgon2Params() || uint32(len(key)) != passwordKeyLen
}

// verifyArgon2 проверяет пароль по хешу Argon2id.
func verifyArgon2(password, encoded string) error {
	params, salt, key, err := decodeArgon2Hash(encoded)
	if err != nil {
		return err
	}

	actual := argon2.IDKey([]byte(password), salt, params.time, params.memory, params.threads, uint32(len(key)))

	if subtle.ConstantTimeCompare(actual, key) != 1 {
		return ErrPasswordMismatch
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
		return ErrPasswordMismatch
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

// decodeArgon2Hash разбирает строку хеша Argon2id на параметры, соль и ключ.
func decodeArgon2Hash(encoded string) (argon2Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 {
		return argon2Params{}, nil, nil, fmt.Errorf("%w: %d parts, want 5", ErrInvalidPasswordHash, len(parts))
	}

	if parts[0] != hashAlgo {
		return argon2Params{}, nil, nil, fmt.Errorf("%w: unknown algorithm %q", ErrInvalidPasswordHash, parts[0])
	}

	version, err := strconv.Atoi(strings.TrimPrefix(parts[1], "v="))
	if err != nil || version != argon2Version {
		return argon2Params{}, nil, nil, fmt.Errorf("%w: version %q", ErrInvalidPasswordHash, parts[1])
	}

	params, err := parseArgon2Params(parts[2])
	if err != nil {
		return argon2Params{}, nil, nil, err
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(salt) == 0 {
		return argon2Params{}, nil, nil, fmt.Errorf("%w: salt %q", ErrInvalidPasswordHash, parts[3])
	}

	key, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(key) == 0 {
		return argon2Params{}, nil, nil, fmt.Errorf("%w: key %q", ErrInvalidPasswordHash, parts[4])
	}

	return params, salt, key, nil
}

// parseArgon2Params разбирает сегмент "m=...,t=...,p=...".
func parseArgon2Params(segment string) (argon2Params, error) {
	params := argon2Params{}

	for _, field := range strings.Split(segment, ",") {
		key, value, found := strings.Cut(field, "=")
		if !found {
			return argon2Params{}, fmt.Errorf("%w: params %q", ErrInvalidPasswordHash, segment)
		}

		parsed, err := strconv.ParseUint(value, 10, 32)
		if err != nil || parsed == 0 {
			return argon2Params{}, fmt.Errorf("%w: param %q", ErrInvalidPasswordHash, field)
		}

		switch key {
		case "m":
			params.memory = uint32(parsed)
		case "t":
			params.time = uint32(parsed)
		case "p":
			if parsed > 255 {
				return argon2Params{}, fmt.Errorf("%w: param %q", ErrInvalidPasswordHash, field)
			}

			params.threads = uint8(parsed)
		default:
			return argon2Params{}, fmt.Errorf("%w: param %q", ErrInvalidPasswordHash, field)
		}
	}

	if params.memory == 0 || params.time == 0 || params.threads == 0 {
		return argon2Params{}, fmt.Errorf("%w: params %q", ErrInvalidPasswordHash, segment)
	}

	return params, nil
}

// decodePBKDF2Hash разбирает строку хеша прежнего формата.
func decodePBKDF2Hash(encoded string) (int, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 {
		return 0, nil, nil, fmt.Errorf("%w: %d parts, want 4", ErrInvalidPasswordHash, len(parts))
	}

	if parts[0] != legacyHashAlgo {
		return 0, nil, nil, fmt.Errorf("%w: unknown algorithm %q", ErrInvalidPasswordHash, parts[0])
	}

	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations <= 0 {
		return 0, nil, nil, fmt.Errorf("%w: iterations %q", ErrInvalidPasswordHash, parts[1])
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil || len(salt) == 0 {
		return 0, nil, nil, fmt.Errorf("%w: salt %q", ErrInvalidPasswordHash, parts[2])
	}

	key, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(key) == 0 {
		return 0, nil, nil, fmt.Errorf("%w: key %q", ErrInvalidPasswordHash, parts[3])
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
