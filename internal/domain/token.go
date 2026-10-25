package domain

import (
	"context"
	"time"

	"github.com/botbooker/bb-core/internal/model"
)

// TokenRepository — порт репозитория сессий (user_tokens).
type TokenRepository interface {
	// Create вставляет выданный токен; конфликт уникальности — ErrDuplicate.
	Create(ctx context.Context, token *model.Token) error
	// GetByID возвращает токен по идентификатору (jti) или ErrNotFound.
	GetByID(ctx context.Context, id string) (*model.Token, error)
	// TouchLastUsed отмечает, что токен использовали (аудит активности).
	TouchLastUsed(ctx context.Context, id string, at time.Time) error
	// Revoke помечает токен отозванным. Повторный отзыв не ошибка.
	Revoke(ctx context.Context, id string, at time.Time) error
	// RevokeByUser отзывает все действующие сессии пользователя во всех
	// тенантах: logout-all, смена пароля и удаление аккаунта.
	RevokeByUser(ctx context.Context, userID string, at time.Time) (int64, error)
	// RevokeByTenant отзывает все действующие сессии тенанта.
	RevokeByTenant(ctx context.Context, tenantID string, at time.Time) (int64, error)
	// RevokeByUserTenant отзывает сессии пользователя в одном тенанте.
	RevokeByUserTenant(ctx context.Context, userID, tenantID string, at time.Time) (int64, error)
	// DeleteExpired удаляет записи, истёкшие до указанного момента.
	DeleteExpired(ctx context.Context, userID string, before time.Time) (int64, error)
}
