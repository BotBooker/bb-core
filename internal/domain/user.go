package domain

import (
	"context"

	"github.com/botbooker/bb-core/internal/model"
)

// UserRepository — порт репозитория пользователей.
//
// Репозиторий работает с агрегатом model.User: курсоры, SQL и диалекты
// остаются в реализации (internal/repository). Отсутствие активной записи —
// ErrNotFound, конфликт уникальности email/телефона — ErrDuplicate.
type UserRepository interface {
	// Create вставляет пользователя; конфликт уникальности — ErrDuplicate.
	Create(ctx context.Context, user *model.User) error
	// GetByID возвращает активного пользователя по идентификатору или ErrNotFound.
	GetByID(ctx context.Context, id string) (*model.User, error)
	// GetByEmail возвращает активного пользователя по email или ErrNotFound.
	GetByEmail(ctx context.Context, email string) (*model.User, error)
	// Update перезаписывает изменяемые поля активного пользователя;
	// отсутствие активной строки — ErrNotFound.
	Update(ctx context.Context, user *model.User) error
	// Delete помечает пользователя удалённым (soft delete);
	// отсутствие активной строки — ErrNotFound.
	Delete(ctx context.Context, id string) error
	// ListAll возвращает активных пользователей с пагинацией: для глобальных
	// администраторов.
	ListAll(ctx context.Context, limit, offset int) ([]*model.User, error)
	// ListVisible возвращает пользователей, видимых staff тенанта.
	ListVisible(ctx context.Context, tenantID string, limit, offset int) ([]*model.User, error)
	// GetVisible возвращает пользователя, видимого staff тенанта, или ErrNotFound.
	GetVisible(ctx context.Context, tenantID, id string) (*model.User, error)
}
