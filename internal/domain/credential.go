package domain

import (
	"context"
	"time"

	"github.com/botbooker/bb-core/internal/model"
)

// CredentialRepository — порт репозитория учётных данных (user_credentials).
//
// У профиля может быть несколько записей — по одной на провайдера
// авторизации; действующая локальная запись ищется по провайдеру email.
type CredentialRepository interface {
	// Create сохраняет учётные данные; конфликт уникальности — ErrDuplicate.
	Create(ctx context.Context, credentials *model.Credentials) error
	// GetByUserID возвращает действующие учётные данные пользователя для
	// локальной авторизации (провайдер email) или ErrNotFound.
	GetByUserID(ctx context.Context, userID string) (*model.Credentials, error)
	// GetByProvider находит учётные данные по паре «провайдер + ключ
	// провайдера»; отсутствие записи — ErrNotFound.
	GetByProvider(ctx context.Context, provider, providerKey string) (*model.Credentials, error)
	// Update перезаписывает изменяемые поля учётных данных; отсутствие записи —
	// ErrNotFound.
	Update(ctx context.Context, credentials *model.Credentials) error
	// TouchLastLogin отмечает вход по учётным данным (аудит).
	TouchLastLogin(ctx context.Context, id string, at time.Time) error
	// DeactivateByUser мягко удаляет все учётные данные пользователя: при
	// удалении аккаунта пароль не должен оставаться действующим. Операция
	// идемпотентна и не возвращает ErrNotFound, если записей нет.
	DeactivateByUser(ctx context.Context, userID string, at time.Time) error
}
