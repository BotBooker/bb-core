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
)

// Требования к паролю и параметры хеширования.
const (
	// PasswordMinLen — минимальная длина пароля в символах (рунах).
	PasswordMinLen = 8
	// PasswordMaxLen — максимальная длина пароля в байтах: ограничивает работу,
	// которую злоумышленник может заставить сервер выполнить одним запросом.
	PasswordMaxLen = 128
	// passwordSaltLen — длина соли в байтах.
	passwordSaltLen = 16
	// passwordKeyLen — длина ключа (хеша) в байтах.
	passwordKeyLen = 32
	// passwordIterations — число итераций PBKDF2-HMAC-SHA256 по умолчанию:
	// компромисс между стойкостью и временем входа (≈0,2 с на этом железе).
	// Значение хранится в самом хеше, поэтому его можно поднять без сброса
	// паролей (текущая рекомендация OWASP — 600 000 итераций).
	passwordIterations = 210_000
	// hashAlgo — имя алгоритма в закодированном хеше.
	hashAlgo = "pbkdf2_sha256"
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

// HashPassword считает хеш пароля в формате
//
//	pbkdf2_sha256$<итерации>$<соль>$<ключ>
//
// Алгоритм и число итераций хранятся вместе с хешем: их смена не ломает уже
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

	key, err := pbkdf2.Key(sha256.New, password, salt, passwordIterations, passwordKeyLen)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}

	return encodePasswordHash(passwordIterations, salt, key), nil
}

// VerifyPassword сравнивает пароль с закодированным хешем.
//
// Возвращает ErrPasswordMismatch, если пароль не подходит, и
// ErrInvalidPasswordHash, если хеш нельзя разобрать. Сравнение ключей —
// за постоянное время (crypto/subtle): по времени ответа нельзя узнать,
// сколько байт хеша совпало.
func VerifyPassword(password, encoded string) error {
	iterations, salt, key, err := decodePasswordHash(encoded)
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

// encodePasswordHash собирает строку хеша.
func encodePasswordHash(iterations int, salt, key []byte) string {
	encode := base64.RawStdEncoding.EncodeToString

	return fmt.Sprintf("%s$%d$%s$%s", hashAlgo, iterations, encode(salt), encode(key))
}

// decodePasswordHash разбирает строку хеша на итерации, соль и ключ.
func decodePasswordHash(encoded string) (int, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 {
		return 0, nil, nil, fmt.Errorf("%w: %d parts, want 4", ErrInvalidPasswordHash, len(parts))
	}

	if parts[0] != hashAlgo {
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
