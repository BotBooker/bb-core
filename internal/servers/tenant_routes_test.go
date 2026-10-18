package servers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/model"
	"github.com/botbooker/bb-core/internal/service"
)

// newTenantRoutesHandler собирает хендлер с моками авторизации и каталога.
func newTenantRoutesHandler(t *testing.T, auth *mockAuthService, catalog *mockPublicCatalogService) Handler {
	t.Helper()

	if auth.authenticateFunc == nil {
		principal := testPrincipal(t, handlerTestUser(t), model.RoleClient)
		auth.authenticateFunc = func(_ context.Context, token string) (*service.Principal, error) {
			if token == "" {
				return nil, service.ErrUnauthorized
			}

			return principal, nil
		}
	}

	handler, err := NewHandler(
		auth,
		NewUserServiceServer(&mockUserServiceAPI{}),
		NewTenantServiceServer(&mockTenantManagementService{}),
		catalog,
	)
	if err != nil {
		t.Fatalf("NewHandler() unexpected error = %v", err)
	}

	return handler
}

// serveAuthenticated выполняет запрос с токеном.
func serveAuthenticated(handler Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set(AuthorizationHeader, bearerPrefix+"token")

	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	return rec
}

func TestHandler_TenantRoutes(t *testing.T) {
	auth := &mockAuthService{
		listMyTenantsFunc: func(_ context.Context, _ *service.Principal) ([]*model.TenantMembership, error) {
			return []*model.TenantMembership{{
				Tenant: model.Tenant{ID: testTenantID, Name: "Acme", Slug: "acme", Status: model.TenantStatusActive},
				Member: model.TenantMember{TenantID: testTenantID, Role: model.RoleClient, JoinedAt: time.Now().UTC()},
			}}, nil
		},
		switchTenantFunc: func(_ context.Context, _ *service.Principal, slug string) (*service.Session, error) {
			if slug != "acme" {
				t.Errorf("SwitchTenant() slug = %q", slug)
			}

			now := time.Now().UTC()

			return &service.Session{
				AccessToken: "new-token",
				Token:       &model.Token{ExpiresAt: now.Add(time.Hour)},
				Tenant:      &model.Tenant{ID: testTenantID, Slug: "acme", Name: "Acme"},
				Role:        model.RoleManager,
			}, nil
		},
		logoutAllFunc: func(context.Context, *service.Principal) error { return nil },
	}

	handler := newTenantRoutesHandler(t, auth, &mockPublicCatalogService{})

	t.Run("GET /auth/tenants", func(t *testing.T) {
		rec := serveAuthenticated(handler, http.MethodGet, "/auth/tenants", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}

		for _, want := range []string{"acme", "Acme", "client"} {
			if !strings.Contains(rec.Body.String(), want) {
				t.Errorf("body = %q, want %q", rec.Body.String(), want)
			}
		}
	})

	t.Run("POST /auth/tenant/switch", func(t *testing.T) {
		rec := serveAuthenticated(handler, http.MethodPost, "/auth/tenant/switch", `{"tenant_slug":"acme"}`)

		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "new-token") {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}

		if !strings.Contains(rec.Body.String(), "manager") {
			t.Errorf("body = %q, want роль manager", rec.Body.String())
		}
	})

	t.Run("POST /auth/logout-all", func(t *testing.T) {
		rec := serveAuthenticated(handler, http.MethodPost, "/auth/logout-all", "")

		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("битый JSON переключения — 400", func(t *testing.T) {
		rec := serveAuthenticated(handler, http.MethodPost, "/auth/tenant/switch", "{")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})
}

func TestHandler_PublicTenantPage(t *testing.T) {
	catalog := &mockPublicCatalogService{
		getTenantFunc: func(_ context.Context, slug string) (*model.PublicTenant, error) {
			if slug != "acme" {
				return nil, database.ErrNotFound
			}

			return &model.PublicTenant{
				Name: "Acme", Slug: "acme", Description: "Аренда", Location: "Москва",
			}, nil
		},
	}

	handler := newTenantRoutesHandler(t, &mockAuthService{}, catalog)

	t.Run("GET /t/acme без токена", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/t/acme", nil)
		rec := httptest.NewRecorder()
		handler.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}

		for _, want := range []string{"acme", "Аренда", "Москва"} {
			if !strings.Contains(rec.Body.String(), want) {
				t.Errorf("body = %q, want %q", rec.Body.String(), want)
			}
		}
	})

	t.Run("GET /t/missing — 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/t/missing", nil)
		rec := httptest.NewRecorder()
		handler.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /api/v1/public/tenants/acme без токена", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/public/tenants/acme", nil)
		rec := httptest.NewRecorder()
		handler.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})
}
