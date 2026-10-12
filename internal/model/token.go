package model

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Ошибки токенов доступа.
var (
	// ErrInvalidToken — токен не проходит проверку модели.
	ErrInvalidToken = errors.New("auth token is invalid")
	// ErrTokenExpired — срок действия токена истёк.
	ErrTokenExpired = errors.New("auth token is expired")
	// ErrTokenRevoked — токен отозван (выход из системы).
	ErrTokenRevoked = errors.New("auth token is revoked")
)

// Token — выданный пользователю токен доступа.
//
// Сам токен подписан и отдаётся клиенту целиком, а в БД лежит его описание.
// Нужно оно, чтобы токен можно было отозвать: одной подписи для этого мало —
// иначе выход из системы не работал бы до истечения срока. Идентификатор
// записи (ID) — это claim jti: по нему проверка находит токен и узнаёт,
// не отозван ли он.
type Token struct {
	// ID — идентификатор токена (jti), UUIDv4. Первичный ключ user_tokens.id.
	ID string
	// UserID — владелец токена: идентификатор пользователя (users.id).
	UserID string
	// Role — роль пользователя на момент выдачи: попадает в claim role.
	Role Role
	// IssuedAt — когда токен выдан (UTC).
	IssuedAt time.Time
	// ExpiresAt — когда токен перестаёт действовать (UTC).
	ExpiresAt time.Time
	// RevokedAt — когда токен отозвали; nil — действует.
	RevokedAt *time.Time
	// LastUsedAt — когда токен использовали в последний раз; nil — не использовали.
	LastUsedAt *time.Time
}

// NewToken выдаёт токен пользователю на срок ttl.
//
// Идентификатор (UUIDv4) и время выдачи проставляет модель — так же, как
// NewUser делает для пользователя. Роль берётся у пользователя и попадает в
// claim role: по ней транспорт проверяет права, не ходя в базу за пользователем.
func NewToken(user *User, ttl time.Duration, now time.Time) (*Token, error) {
	if user == nil {
		return nil, fmt.Errorf("%w: user is nil", ErrInvalidToken)
	}

	if err := user.Validate(); err != nil {
		return nil, fmt.Errorf("issue token: %w", err)
	}

	if ttl <= 0 {
		return nil, fmt.Errorf("%w: ttl %s is not positive", ErrInvalidToken, ttl)
	}

	issuedAt := now.UTC()

	return &Token{
		ID:        uuid.NewString(),
		UserID:    user.ID,
		Role:      user.Role,
		IssuedAt:  issuedAt,
		ExpiresAt: issuedAt.Add(ttl),
	}, nil
}

// Validate проверяет инварианты записи токена.
//
// Срок действия здесь не проверяется: он зависит от текущего момента,
// поэтому за него отвечает Valid.
func (t Token) Validate() error {
	var errs []error

	if err := validateUUID(t.ID); err != nil {
		errs = append(errs, fmt.Errorf("%w: token id: %w", ErrInvalidToken, err))
	}

	if err := validateUUID(t.UserID); err != nil {
		errs = append(errs, fmt.Errorf("%w: user id: %w", ErrInvalidToken, err))
	}

	if !t.Role.Valid() {
		errs = append(errs, fmt.Errorf("%w: role %q: %w", ErrInvalidToken, t.Role, ErrInvalidRole))
	}

	if t.IssuedAt.IsZero() {
		errs = append(errs, fmt.Errorf("%w: issued_at is not set", ErrInvalidToken))
	}

	if !t.ExpiresAt.After(t.IssuedAt) {
		errs = append(errs, fmt.Errorf("%w: expires_at %s is not after issued_at %s",
			ErrInvalidToken, t.ExpiresAt.UTC().Format(time.RFC3339), t.IssuedAt.UTC().Format(time.RFC3339)))
	}

	return errors.Join(errs...)
}

// Valid сообщает, что токен можно использовать в момент now: он не отозван и
// не истёк. Возвращает nil, ErrTokenRevoked или ErrTokenExpired — так причину
// отказа видно в логе и в ответе транспорта.
func (t Token) Valid(now time.Time) error {
	if t.RevokedAt != nil {
		return fmt.Errorf("%w: revoked at %s", ErrTokenRevoked, t.RevokedAt.UTC().Format(time.RFC3339))
	}

	if !now.UTC().Before(t.ExpiresAt) {
		return fmt.Errorf("%w: expired at %s", ErrTokenExpired, t.ExpiresAt.UTC().Format(time.RFC3339))
	}

	return nil
}

// Revoke отзывает токен: после этого Valid вернёт ErrTokenRevoked.
// Повторный отзыв время не меняет — отзыв идемпотентен.
func (t *Token) Revoke(now time.Time) {
	if t.RevokedAt != nil {
		return
	}

	ts := now.UTC()
	t.RevokedAt = &ts
}

// Revoked сообщает, что токен отозван.
func (t Token) Revoked() bool { return t.RevokedAt != nil }

// Touch отмечает использование токена (аудит последней активности).
func (t *Token) Touch(now time.Time) {
	ts := now.UTC()
	t.LastUsedAt = &ts
}
