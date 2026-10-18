package servers

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/model"
	public_v1 "github.com/botbooker/bb-core/pkg/proto/public/v1"
)

func TestPublicCatalogServer(t *testing.T) {
	tenant := &model.PublicTenant{Name: "Acme", Slug: "acme", Description: "Аренда", Location: "Москва"}

	catalog := &mockPublicCatalogService{
		getTenantFunc: func(_ context.Context, slug string) (*model.PublicTenant, error) {
			if slug == "missing" {
				return nil, database.ErrNotFound
			}

			return tenant, nil
		},
		listTenantsFunc: func(_ context.Context, page model.Page) ([]*model.PublicTenant, string, error) {
			return []*model.PublicTenant{tenant}, "next", nil
		},
		listObjectsFunc: func(_ context.Context, _ string, _ model.Page) ([]*model.PublicObject, string, error) {
			return []*model.PublicObject{{
				Title:       "Дом",
				Description: "Уютный",
				PriceMinor:  100000,
				Currency:    "RUB",
				Address:     "Москва",
			}}, "", nil
		},
	}

	server := NewPublicCatalogServer(catalog)

	got, err := server.GetTenant(context.Background(), &public_v1.GetTenantRequest{Slug: "acme"})
	if err != nil || got.GetTenant().GetDescription() != "Аренда" {
		t.Fatalf("GetTenant() = %v, %v", got, err)
	}

	if _, err := server.GetTenant(context.Background(), &public_v1.GetTenantRequest{Slug: "missing"}); status.Code(err) != codes.NotFound {
		t.Errorf("GetTenant() missing code = %v", status.Code(err))
	}

	if _, err := server.GetTenant(context.Background(), &public_v1.GetTenantRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("GetTenant() пустой slug code = %v", status.Code(err))
	}

	tenants, err := server.ListTenants(context.Background(), &public_v1.ListTenantsRequest{PageSize: 10})
	if err != nil || len(tenants.GetTenants()) != 1 || tenants.GetNextPageToken() != "next" {
		t.Fatalf("ListTenants() = %v, %v", tenants, err)
	}

	objects, err := server.ListObjects(context.Background(), &public_v1.ListObjectsRequest{Slug: "acme", PageSize: 10})
	if err != nil || len(objects.GetObjects()) != 1 || objects.GetObjects()[0].GetCurrency() != "RUB" {
		t.Fatalf("ListObjects() = %v, %v", objects, err)
	}

	if _, err := server.ListTenants(context.Background(), &public_v1.ListTenantsRequest{PageToken: "not-base64!"}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("ListTenants() плохой page_token code = %v", status.Code(err))
	}
}
