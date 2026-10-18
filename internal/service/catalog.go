package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/model"
	"github.com/botbooker/bb-core/pkg/observability"
)

// PublicTenantRepository — интерфейс чтения тенантов для публичного каталога.
type PublicTenantRepository interface {
	// GetBySlug возвращает тенант по slug или database.ErrNotFound.
	GetBySlug(ctx context.Context, slug string) (*model.Tenant, error)
	// ListActive возвращает действующие тенанты с пагинацией.
	ListActive(ctx context.Context, limit, offset int) ([]*model.Tenant, error)
}

// PublicCatalogRepository — интерфейс чтения публичных объектов.
type PublicCatalogRepository interface {
	// ListPublicObjects возвращает опубликованные объекты тенанта.
	ListPublicObjects(ctx context.Context, tenantID string, limit, offset int) ([]*model.PublicObject, error)
}

// PublicCatalogService — публичный каталог тенантов и объектов.
//
// Ручки не требуют аутентификации и членства, поэтому сервис не принимает
// Principal: он явно tenant-agnostic. Наружу отдаются только публичные поля
// (ADR 0001 §8), а приостановленные и удалённые тенанты не видны.
type PublicCatalogService interface {
	// GetTenant возвращает публичный профиль действующего тенанта по slug.
	GetTenant(ctx context.Context, slug string) (*model.PublicTenant, error)
	// ListTenants возвращает публичный каталог действующих тенантов.
	ListTenants(ctx context.Context, page model.Page) ([]*model.PublicTenant, string, error)
	// ListObjects возвращает публичный каталог объектов действующего тенанта.
	ListObjects(ctx context.Context, slug string, page model.Page) ([]*model.PublicObject, string, error)
}

// publicCatalogService — конкретная реализация, скрыта от внешних пакетов.
type publicCatalogService struct {
	tenants PublicTenantRepository
	objects PublicCatalogRepository
}

// NewPublicCatalogService создаёт сервис публичного каталога.
func NewPublicCatalogService(
	ctx context.Context,
	tenants PublicTenantRepository,
	objects PublicCatalogRepository,
) PublicCatalogService {
	slog.DebugContext(ctx, "public catalog service created")

	return &publicCatalogService{tenants: tenants, objects: objects}
}

// GetTenant возвращает публичный профиль тенанта по slug.
func (s *publicCatalogService) GetTenant(
	ctx context.Context,
	slug string,
) (*model.PublicTenant, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "GetPublicTenant")
	defer spanEnd()

	tenant, err := s.activeTenant(traceCtx, slug)
	if err != nil {
		return nil, fmt.Errorf("get public tenant: %w", err)
	}

	return publicTenantView(traceCtx, tenant), nil
}

// ListTenants возвращает каталог действующих тенантов.
func (s *publicCatalogService) ListTenants(
	ctx context.Context,
	page model.Page,
) ([]*model.PublicTenant, string, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "ListPublicTenants")
	defer spanEnd()

	tenants, err := s.tenants.ListActive(traceCtx, page.Limit+1, page.Offset)
	if err != nil {
		return nil, "", fmt.Errorf("list public tenants: %w", err)
	}

	hasMore := len(tenants) > page.Limit
	if hasMore {
		tenants = tenants[:page.Limit]
	}

	views := make([]*model.PublicTenant, 0, len(tenants))
	for _, tenant := range tenants {
		views = append(views, publicTenantView(traceCtx, tenant))
	}

	return views, page.Next(hasMore), nil
}

// ListObjects возвращает публичный каталог объектов тенанта.
func (s *publicCatalogService) ListObjects(
	ctx context.Context,
	slug string,
	page model.Page,
) ([]*model.PublicObject, string, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "ListPublicObjects")
	defer spanEnd()

	tenant, err := s.activeTenant(traceCtx, slug)
	if err != nil {
		return nil, "", fmt.Errorf("list public objects: %w", err)
	}

	objects, err := s.objects.ListPublicObjects(traceCtx, tenant.ID, page.Limit+1, page.Offset)
	if err != nil {
		return nil, "", fmt.Errorf("list public objects: %w", err)
	}

	hasMore := len(objects) > page.Limit
	if hasMore {
		objects = objects[:page.Limit]
	}

	return objects, page.Next(hasMore), nil
}

// activeTenant находит действующий тенант по slug; приостановленный и
// удалённый неотличимы от несуществующего.
func (s *publicCatalogService) activeTenant(ctx context.Context, slug string) (*model.Tenant, error) {
	normalized := model.NormalizeSlug(slug)
	if normalized == "" {
		return nil, model.ErrInvalidTenant
	}

	tenant, err := s.tenants.GetBySlug(ctx, normalized)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return nil, database.ErrNotFound
		}

		return nil, err
	}

	if !tenant.Active() {
		return nil, database.ErrNotFound
	}

	return tenant, nil
}

// publicTenantView отображает тенант в публичное представление.
func publicTenantView(ctx context.Context, tenant *model.Tenant) *model.PublicTenant {
	view := &model.PublicTenant{Name: tenant.Name, Slug: tenant.Slug}
	if len(tenant.Metadata) == 0 {
		return view
	}

	var metadata map[string]string
	if err := json.Unmarshal(tenant.Metadata, &metadata); err != nil {
		// Metadata — внутренние настройки: повреждённое значение не должно
		// ломать публичную страницу, поэтому отдаём профиль без описания.
		slog.DebugContext(ctx, "public tenant metadata is invalid",
			"tenant", tenant.ID, "err", err)

		return view
	}

	view.Description = metadata["description"]
	view.Location = metadata["location"]

	return view
}
