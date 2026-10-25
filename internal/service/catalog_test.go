package service

import (
	"context"
	"errors"
	"testing"

	"github.com/botbooker/bb-core/internal/domain"
	"github.com/botbooker/bb-core/internal/model"
)

// mockPublicTenantRepository — мок чтения тенантов для публичного каталога.
type mockPublicTenantRepository struct {
	getBySlugFunc  func(ctx context.Context, slug string) (*model.Tenant, error)
	listActiveFunc func(ctx context.Context, limit, offset int) ([]*model.Tenant, error)
}

func (m *mockPublicTenantRepository) GetBySlug(ctx context.Context, slug string) (*model.Tenant, error) {
	if m.getBySlugFunc != nil {
		return m.getBySlugFunc(ctx, slug)
	}

	return nil, domain.ErrNotFound
}

func (m *mockPublicTenantRepository) ListActive(
	ctx context.Context,
	limit, offset int,
) ([]*model.Tenant, error) {
	if m.listActiveFunc != nil {
		return m.listActiveFunc(ctx, limit, offset)
	}

	return nil, nil
}

// mockPublicCatalogRepository — мок публичных объектов.
type mockPublicCatalogRepository struct {
	listFunc func(ctx context.Context, tenantID string, limit, offset int) ([]*model.PublicObject, error)
}

func (m *mockPublicCatalogRepository) ListPublicObjects(
	ctx context.Context,
	tenantID string,
	limit, offset int,
) ([]*model.PublicObject, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, tenantID, limit, offset)
	}

	return nil, nil
}

func TestPublicCatalogService_GetTenant(t *testing.T) {
	t.Run("отдаёт публичные поля из metadata", func(t *testing.T) {
		repo := &mockPublicTenantRepository{
			getBySlugFunc: func(_ context.Context, slug string) (*model.Tenant, error) {
				if slug != "acme" {
					t.Errorf("GetBySlug() slug = %q, want acme", slug)
				}

				return &model.Tenant{
					ID:       testTenantID,
					Name:     "Acme",
					Slug:     "acme",
					Status:   model.TenantStatusActive,
					Metadata: []byte(`{"description":"Аренда","location":"Москва"}`),
				}, nil
			},
		}

		svc := NewPublicCatalogService(context.Background(), repo, &mockPublicCatalogRepository{})

		tenant, err := svc.GetTenant(context.Background(), "  ACME ")
		if err != nil {
			t.Fatalf("GetTenant() unexpected error = %v", err)
		}

		if tenant.Name != "Acme" || tenant.Slug != "acme" || tenant.Description != "Аренда" || tenant.Location != "Москва" {
			t.Errorf("tenant = %+v", tenant)
		}
	})

	t.Run("приостановленный неотличим от несуществующего", func(t *testing.T) {
		repo := &mockPublicTenantRepository{
			getBySlugFunc: func(_ context.Context, _ string) (*model.Tenant, error) {
				return &model.Tenant{ID: testTenantID, Status: model.TenantStatusSuspended}, nil
			},
		}

		svc := NewPublicCatalogService(context.Background(), repo, &mockPublicCatalogRepository{})

		if _, err := svc.GetTenant(context.Background(), "acme"); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("GetTenant() error = %v, want %v", err, domain.ErrNotFound)
		}
	})

	t.Run("пустой slug", func(t *testing.T) {
		svc := NewPublicCatalogService(context.Background(), &mockPublicTenantRepository{}, &mockPublicCatalogRepository{})

		if _, err := svc.GetTenant(context.Background(), " "); !errors.Is(err, model.ErrInvalidTenant) {
			t.Errorf("GetTenant() error = %v, want %v", err, model.ErrInvalidTenant)
		}
	})
}

func TestPublicCatalogService_ListTenants(t *testing.T) {
	repo := &mockPublicTenantRepository{
		listActiveFunc: func(_ context.Context, limit, offset int) ([]*model.Tenant, error) {
			if limit != 2 || offset != 0 {
				t.Errorf("ListActive() = %d/%d, want 2/0", limit, offset)
			}

			return []*model.Tenant{
				activeTenant(testTenantID),
				activeTenant(otherTenantID),
			}, nil
		},
	}

	svc := NewPublicCatalogService(context.Background(), repo, &mockPublicCatalogRepository{})

	tenants, next, err := svc.ListTenants(context.Background(), model.Page{Limit: 1})
	if err != nil || len(tenants) != 1 || next == "" {
		t.Fatalf("ListTenants() = %d, %q, %v", len(tenants), next, err)
	}
}

func TestPublicCatalogService_ListObjects(t *testing.T) {
	t.Run("объекты тенанта", func(t *testing.T) {
		tenantRepo := &mockPublicTenantRepository{
			getBySlugFunc: func(_ context.Context, _ string) (*model.Tenant, error) {
				return activeTenant(testTenantID), nil
			},
		}
		objectRepo := &mockPublicCatalogRepository{
			listFunc: func(_ context.Context, tenantID string, limit, offset int) ([]*model.PublicObject, error) {
				if tenantID != testTenantID || limit != 2 || offset != 0 {
					t.Errorf("ListPublicObjects() = %q/%d/%d", tenantID, limit, offset)
				}

				return []*model.PublicObject{{Title: "Дом", PriceMinor: 1000}}, nil
			},
		}

		svc := NewPublicCatalogService(context.Background(), tenantRepo, objectRepo)

		objects, next, err := svc.ListObjects(context.Background(), "acme", model.Page{Limit: 1})
		if err != nil || len(objects) != 1 || next != "" {
			t.Fatalf("ListObjects() = %v, %q, %v", objects, next, err)
		}
	})

	t.Run("неизвестный тенант", func(t *testing.T) {
		svc := NewPublicCatalogService(context.Background(), &mockPublicTenantRepository{}, &mockPublicCatalogRepository{})

		if _, _, err := svc.ListObjects(context.Background(), "nope", model.Page{Limit: 10}); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("ListObjects() error = %v, want %v", err, domain.ErrNotFound)
		}
	})
}
