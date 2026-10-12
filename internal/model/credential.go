package model

import (
	"fmt"
	"time"
)

// Credentials — учётные данные пользователя: хеш пароля.
//
// Пароль хранится отдельно от профиля (таблица user_credentials): профиль
// читается почти каждым запросом, а хеш пароля нужен только при входе, и
// вынесение его из users уменьшает объём читаемых данных и риск утечки
// хеша в логи ответов профиля. Связь с users — по идентификатору
// (в БД это внешний ключ с ON DELETE CASCADE).
type Credentials struct {
	// UserID — владелец: идентификатор пользователя (users.id).
	UserID string
	// Hash — хеш пароля в формате pbkdf2_sha256$<итерации>$<соль>$<ключ>.
	Hash string
	// CreatedAt — когда пароль задан (UTC).
	CreatedAt time.Time
	// UpdatedAt — когда пароль изменён (UTC).
	UpdatedAt time.Time
}

// NewCredentials создаёт учётные данные пользователя.
//
// Пароль сразу превращается в хеш: открытый пароль не сохраняется нигде —
// ни в модели, ни в БД, ни в событиях.
func NewCredentials(userID, password string, now time.Time) (*Credentials, error) {
	if err := validateUUID(userID); err != nil {
		return nil, fmt.Errorf("create credentials: %w", err)
	}

	hash, err := HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("create credentials: %w", err)
	}

	ts := now.UTC()

	return &Credentials{UserID: userID, Hash: hash, CreatedAt: ts, UpdatedAt: ts}, nil
}

// SetPassword меняет пароль: считается новый хеш и время изменения.
func (c *Credentials) SetPassword(password string, now time.Time) error {
	hash, err := HashPassword(password)
	if err != nil {
		return fmt.Errorf("set password: %w", err)
	}

	c.Hash = hash
	c.UpdatedAt = now.UTC()

	return nil
}

// Verify проверяет пароль пользователя.
//
// Возвращает ErrPasswordMismatch, если пароль не подходит, и
// ErrInvalidPasswordHash, если сохранённый хеш повреждён. Причина отказа не
// должна уходить клиенту: иначе по ответам можно перебирать пароли по одному.
func (c Credentials) Verify(password string) error {
	if err := VerifyPassword(password, c.Hash); err != nil {
		return fmt.Errorf("verify password: %w", err)
	}

	return nil
}
