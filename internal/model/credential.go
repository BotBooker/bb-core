package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrPasswordExpired — срок действия пароля истёк. Пользователь обязан сменить
// пароль, прежде чем продолжать работу: до этого доступ закрыт.
var ErrPasswordExpired = errors.New("user password is expired")

// ErrInvalidCredential — запись учётных данных не проходит валидацию модели.
var ErrInvalidCredential = errors.New("credentials are invalid")

// CredentialProviderEmail — провайдер локальной авторизации (HTTP/gRPC) по
// email: это значение попадает в user_credentials.provider, когда провайдер
// не указан явно (ERDB 2.4).
const CredentialProviderEmail = "email"

// Credentials — учётные данные пользователя (таблица user_credentials).
//
// Хеш пароля хранится отдельно от профиля (users): профиль читается почти
// каждым запросом, а хеш нужен только при входе, поэтому вынесение его из
// users уменьшает объём читаемых данных и риск утечки хеша в логи ответов
// профиля. Связь с users — по user_id (в БД это внешний ключ с ON DELETE
// CASCADE), причём одна запись соответствует одному провайдеру: у профиля
// учётных данных может быть несколько ('email', 'telegram', ...).
type Credentials struct {
	// ID — идентификатор записи (PK user_credentials.id, UUIDv4).
	ID string
	// UserID — владелец: идентификатор пользователя (users.id).
	UserID string
	// Provider — провайдер авторизации: 'email', 'telegram', 'google', ...
	Provider string
	// ProviderKey — ключ внутри провайдера: email, телефон, числовой Telegram
	// ID. Именно по паре (provider, provider_key) ищется запись при входе
	// (индекс idx_credentials_lookup).
	ProviderKey string
	// Hash — хеш пароля в формате argon2id$v=19$m=...,t=...,p=...$<соль>$<ключ>.
	Hash string
	// Meta — метаданные провайдера (JSONB в PostgreSQL, JSON-строка в SQLite);
	// nil означает NULL.
	Meta []byte
	// CreatedAt — когда учётные данные заведены (UTC).
	CreatedAt time.Time
	// LastLoginAt — когда по ним входили; nil — не входили.
	LastLoginAt *time.Time
	// ExpiresAt — когда учётные данные перестают действовать. Для пароля это
	// «пароль устарел»: сервис задаёт момент как время смены пароля плюс
	// PASSWORD_TTL. Нулевой момент означает «не истекает» (TTL отключён).
	ExpiresAt time.Time
	// RevokedAt — когда учётные данные отозваны; nil — действуют.
	RevokedAt *time.Time
	// IsActive — признак «запись живая»: false означает удаление (soft delete).
	IsActive bool
	// IsDelete — признак удаления записи (мягкое удаление).
	IsDelete bool
}

// NewCredentials создаёт учётные данные для локальной авторизации по email.
//
// Пароль сразу превращается в хеш: открытый пароль не сохраняется нигде — ни в
// модели, ни в БД, ни в событиях. ttl — срок жизни пароля
// (config.Auth().PasswordTTL); 0 или отрицательное значение означает «пароль
// не истекает».
func NewCredentials(userID, email, password string, ttl time.Duration, now time.Time) (*Credentials, error) {
	if err := validateUUID(userID); err != nil {
		return nil, fmt.Errorf("create credentials: %w", err)
	}

	providerKey := NormalizeEmail(email)
	if err := validateEmail(providerKey); err != nil {
		return nil, fmt.Errorf("create credentials: %w", err)
	}

	hash, err := HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("create credentials: %w", err)
	}

	ts := now.UTC()

	return &Credentials{
		ID:          uuid.NewString(),
		UserID:      userID,
		Provider:    CredentialProviderEmail,
		ProviderKey: providerKey,
		Hash:        hash,
		CreatedAt:   ts,
		ExpiresAt:   passwordExpiresAt(ts, ttl),
		IsActive:    true,
	}, nil
}

// Validate проверяет инварианты хранимой записи учётных данных.
//
// Хеш не проверяется на формат: он может быть посчитан предыдущей версией
// алгоритма, и такой хеш — повод для прозрачного пересчёта при входе
// (NeedsRehash), а не для отказа в чтении записи.
func (c *Credentials) Validate() error {
	var errs []error

	if err := validateUUID(c.ID); err != nil {
		errs = append(errs, fmt.Errorf("%w: id: %w", ErrInvalidCredential, err))
	}

	if err := validateUUID(c.UserID); err != nil {
		errs = append(errs, fmt.Errorf("%w: user_id: %w", ErrInvalidCredential, err))
	}

	if strings.TrimSpace(c.Provider) == "" {
		errs = append(errs, fmt.Errorf("%w: provider is empty", ErrInvalidCredential))
	}

	if strings.TrimSpace(c.ProviderKey) == "" {
		errs = append(errs, fmt.Errorf("%w: provider_key is empty", ErrInvalidCredential))
	}

	if c.Hash == "" {
		errs = append(errs, fmt.Errorf("%w: password hash is empty", ErrInvalidCredential))
	}

	if c.CreatedAt.IsZero() {
		errs = append(errs, fmt.Errorf("%w: created_at is not set", ErrInvalidCredential))
	}

	return errors.Join(errs...)
}

// SetPassword меняет пароль: считается новый хеш, срок действия начинается
// заново (время смены пароля плюс ttl).
func (c *Credentials) SetPassword(password string, ttl time.Duration, now time.Time) error {
	hash, err := HashPassword(password)
	if err != nil {
		return fmt.Errorf("set password: %w", err)
	}

	c.Hash = hash
	c.ExpiresAt = passwordExpiresAt(now.UTC(), ttl)
	c.RevokedAt = nil

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

// Expired сообщает, что срок действия пароля истёк к моменту now.
//
// Нулевой ExpiresAt означает «не истекает»: так отключённый PASSWORD_TTL не
// превращается в «пароль просрочен всегда». Сравнение идёт в UTC — модель
// работает только с UTC, а ExpiresAt приходит из репозитория нормализованным.
func (c Credentials) Expired(now time.Time) bool {
	if c.ExpiresAt.IsZero() {
		return false
	}

	return !now.UTC().Before(c.ExpiresAt)
}

// Revoked сообщает, что учётные данные отозваны.
func (c Credentials) Revoked() bool { return c.RevokedAt != nil }

// TouchLastLogin фиксирует вход по этим учётным данным (аудит).
func (c *Credentials) TouchLastLogin(now time.Time) {
	ts := now.UTC()
	c.LastLoginAt = &ts
}

// NeedsRehash сообщает, что сохранённый хеш посчитан устаревшим алгоритмом
// или с устаревшими параметрами: при следующем успешном входе его стоит
// пересчитать.
func (c Credentials) NeedsRehash() bool {
	return NeedsRehash(c.Hash)
}

// Rehash пересчитывает хеш пароля, не меняя срок действия: обновление
// алгоритма — не смена пароля и не должно сбрасывать срок его действия.
func (c *Credentials) Rehash(password string) error {
	hash, err := HashPassword(password)
	if err != nil {
		return fmt.Errorf("rehash password: %w", err)
	}

	c.Hash = hash

	return nil
}

// passwordExpiresAt считает момент истечения пароля: время now плюс ttl.
// ttl <= 0 отключает проверку срока — возвращается нулевой момент
// («не истекает»), а не «истёк сразу».
func passwordExpiresAt(now time.Time, ttl time.Duration) time.Time {
	if ttl <= 0 {
		return time.Time{}
	}

	return now.Add(ttl)
}
