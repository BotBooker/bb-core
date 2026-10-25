package domain

import (
	"context"

	"github.com/botbooker/bb-core/internal/model"
)

// PublicTenantReader — узкий порт чтения тенантов для публичного каталога.
//
// Отдельный от TenantRepository: публичному каталогу нужны только чтение по
// slug и список действующих тенантов, поэтому здесь нет методов записи и
// членства. Реализует его тот же адаптер — repository.TenantRepo.
type PublicTenantReader interface {
	// GetBySlug возвращает тенант по slug или ErrNotFound.
	GetBySlug(ctx context.Context, slug string) (*model.Tenant, error)
	// ListActive возвращает действующие тенанты с пагинацией.
	ListActive(ctx context.Context, limit, offset int) ([]*model.Tenant, error)
}

// CatalogRepository — порт read-only репозитория публичного каталога.
//
// Возвращаются только поля, разрешённые к публикации (ADR 0001 §8): без
// внутренних идентификаторов, владельца и служебных признаков.
type CatalogRepository interface {
	// ListPublicObjects возвращает опубликованные объекты тенанта с пагинацией.
	// Удалённые объекты в каталог не попадают.
	ListPublicObjects(ctx context.Context, tenantID string, limit, offset int) ([]*model.PublicObject, error)
}
