package domain

import (
	"context"
	"time"

	"github.com/botbooker/bb-core/internal/model"
)

// TenantRepository — порт репозитория тенантов и членства в них.
//
// Роль пользователя — атрибут членства (tenant_members.role_id), а сессия
// действует в рамках одного тенанта, поэтому тенанты и членство описывает один
// порт.
type TenantRepository interface {
	// Create создаёт тенант.
	Create(ctx context.Context, tenant *model.Tenant) error
	// CreateWithOwner вставляет тенант и членство владельца в одной транзакции.
	// Внутри Unit of Work операция выполняется в уже открытой транзакции.
	CreateWithOwner(ctx context.Context, tenant *model.Tenant, member *model.TenantMember) error
	// GetByID возвращает тенант по идентификатору или ErrNotFound.
	GetByID(ctx context.Context, id string) (*model.Tenant, error)
	// GetBySlug возвращает тенант по slug или ErrNotFound.
	GetBySlug(ctx context.Context, slug string) (*model.Tenant, error)
	// List возвращает тенанты (кроме удалённых) с пагинацией.
	List(ctx context.Context, limit, offset int) ([]*model.Tenant, error)
	// ListActive возвращает действующие тенанты с пагинацией.
	ListActive(ctx context.Context, limit, offset int) ([]*model.Tenant, error)
	// UpdateStatus меняет статус тенанта; отсутствие строки — ErrNotFound.
	UpdateStatus(ctx context.Context, id string, status model.TenantStatus, now time.Time) error
	// AddMember добавляет пользователя в тенант; повторное членство —
	// ErrDuplicate, несуществующий тенант — нарушение внешнего ключа.
	AddMember(ctx context.Context, member *model.TenantMember) error
	// GetMember возвращает членство пользователя в тенанте или ErrNotFound.
	GetMember(ctx context.Context, tenantID, userID string) (*model.TenantMember, error)
	// GetMemberBySlug возвращает членство в действующем тенанте по slug или
	// ErrNotFound.
	GetMemberBySlug(ctx context.Context, slug, userID string) (*model.TenantMembership, error)
	// MemberOfUser возвращает членство пользователя в действующем тенанте или
	// ErrNotFound — это тенант по умолчанию для новой сессии.
	MemberOfUser(ctx context.Context, userID string) (*model.TenantMember, error)
	// ListMemberships возвращает действующие членства пользователя вместе с
	// тенантами.
	ListMemberships(ctx context.Context, userID string) ([]*model.TenantMembership, error)
	// ListMembers возвращает участников тенанта с профилями пользователей.
	// Удалённые пользователи (soft delete) в список не попадают.
	ListMembers(ctx context.Context, tenantID string, limit, offset int) ([]*model.TenantMemberView, error)
	// GetMemberView возвращает участника тенанта вместе с профилем; удалённый
	// пользователь не виден (ErrNotFound).
	GetMemberView(ctx context.Context, tenantID, userID string) (*model.TenantMemberView, error)
	// UpdateMemberRole меняет роль участника; отсутствие строки — ErrNotFound.
	UpdateMemberRole(ctx context.Context, tenantID, userID string, role model.Role) error
	// RemoveMember удаляет членство; отсутствие строки — ErrNotFound.
	RemoveMember(ctx context.Context, tenantID, userID string) error
	// TransferOwnership передаёт владение тенантом участнику в одной транзакции.
	TransferOwnership(ctx context.Context, tenantID, userID string) error
}
