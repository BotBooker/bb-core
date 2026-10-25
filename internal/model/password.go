package model

import (
	"errors"
	"fmt"
)

// Требования к паролю — правило домена.
//
// Хеширование и проверку выполняет порт domain.PasswordHasher (реализация —
// pkg/password): domain остаётся на stdlib Go (spec §1.1), а параметры
// Argon2id можно менять, не затрагивая модель.
const (
	// PasswordMinLen — минимальная длина пароля в символах (рунах).
	PasswordMinLen = 8
	// PasswordMaxLen — максимальная длина пароля в байтах: ограничивает работу,
	// которую злоумышленник может заставить сервер выполнить одним запросом.
	PasswordMaxLen = 128
)

// ErrInvalidPassword — пароль не проходит требования модели.
var ErrInvalidPassword = errors.New("user password is invalid")

// ValidatePassword проверяет требования к паролю: не короче PasswordMinLen
// символов и не длиннее PasswordMaxLen байт.
//
// Правила намеренно простые: сложность пароля проверяет не сервис, а разумная
// длина — рекомендация NIST SP 800-63B. Пустой пароль невалиден. Вызывается до
// хеширования: в pkg/password попадает уже проверенный пароль.
func ValidatePassword(password string) error {
	if runes := len([]rune(password)); runes < PasswordMinLen {
		return fmt.Errorf("%w: %d characters is less than %d", ErrInvalidPassword, runes, PasswordMinLen)
	}

	if len(password) > PasswordMaxLen {
		return fmt.Errorf("%w: %d bytes is more than %d", ErrInvalidPassword, len(password), PasswordMaxLen)
	}

	return nil
}
