package servers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/model"
	"github.com/botbooker/bb-core/internal/service"
)

// mockAuthService — мок сервиса авторизации для сборки хендлера: сам сервис
// проверяется в тестах service, здесь важен только транспорт.
type mockAuthService struct {
	authenticateFunc   func(ctx context.Context, token string) (*service.Principal, error)
	loginFunc          func(ctx context.Context, email, password, tenantSlug string) (*service.Session, error)
	changePasswordFunc func(ctx context.Context, email, currentPassword, newPassword, tenantSlug string) (*service.Session, error)
	registerFunc       func(ctx context.Context, in model.RegisterInput) (*model.User, *service.Session, error)
	logoutFunc         func(ctx context.Context, token string) error
	switchTenantFunc   func(ctx context.Context, actor *service.Principal, tenantSlug string) (*service.Session, error)
	listMyTenantsFunc  func(ctx context.Context, actor *service.Principal) ([]*model.TenantMembership, error)
	logoutAllFunc      func(ctx context.Context, actor *service.Principal) error
}

func (m *mockAuthService) Authenticate(ctx context.Context, token string) (*service.Principal, error) {
	if m.authenticateFunc != nil {
		return m.authenticateFunc(ctx, token)
	}

	return nil, service.ErrUnauthorized
}

func (m *mockAuthService) Login(
	ctx context.Context,
	email, password, tenantSlug string,
) (*service.Session, error) {
	if m.loginFunc != nil {
		return m.loginFunc(ctx, email, password, tenantSlug)
	}

	return nil, service.ErrInvalidCredentials
}

func (m *mockAuthService) ChangePassword(
	ctx context.Context,
	email, currentPassword, newPassword, tenantSlug string,
) (*service.Session, error) {
	if m.changePasswordFunc != nil {
		return m.changePasswordFunc(ctx, email, currentPassword, newPassword, tenantSlug)
	}

	return nil, service.ErrInvalidCredentials
}

func (m *mockAuthService) Register(
	ctx context.Context,
	in model.RegisterInput,
) (*model.User, *service.Session, error) {
	if m.registerFunc != nil {
		return m.registerFunc(ctx, in)
	}

	return nil, nil, service.ErrForbidden
}

func (m *mockAuthService) Logout(ctx context.Context, token string) error {
	if m.logoutFunc != nil {
		return m.logoutFunc(ctx, token)
	}

	return nil
}

func (m *mockAuthService) SwitchTenant(
	ctx context.Context,
	actor *service.Principal,
	tenantSlug string,
) (*service.Session, error) {
	if m.switchTenantFunc != nil {
		return m.switchTenantFunc(ctx, actor, tenantSlug)
	}

	return nil, service.ErrForbidden
}

func (m *mockAuthService) ListMyTenants(
	ctx context.Context,
	actor *service.Principal,
) ([]*model.TenantMembership, error) {
	if m.listMyTenantsFunc != nil {
		return m.listMyTenantsFunc(ctx, actor)
	}

	return nil, nil
}

func (m *mockAuthService) LogoutAll(ctx context.Context, actor *service.Principal) error {
	if m.logoutAllFunc != nil {
		return m.logoutAllFunc(ctx, actor)
	}

	return nil
}

// mockTenantManagementService — заглушка адаптера тенантов: тесты HTTP-слоя
// не вызывают эти ручки, но NewHandler регистрирует их в gateway.
type mockTenantManagementService struct {
	TenantManagementService
}

// mockPublicCatalogService — заглушка публичного каталога.
type mockPublicCatalogService struct {
	getTenantFunc   func(ctx context.Context, slug string) (*model.PublicTenant, error)
	listTenantsFunc func(ctx context.Context, page model.Page) ([]*model.PublicTenant, string, error)
	listObjectsFunc func(ctx context.Context, slug string, page model.Page) ([]*model.PublicObject, string, error)
}

func (m *mockPublicCatalogService) GetTenant(
	ctx context.Context,
	slug string,
) (*model.PublicTenant, error) {
	if m.getTenantFunc != nil {
		return m.getTenantFunc(ctx, slug)
	}

	return nil, database.ErrNotFound
}

func (m *mockPublicCatalogService) ListTenants(
	ctx context.Context,
	page model.Page,
) ([]*model.PublicTenant, string, error) {
	if m.listTenantsFunc != nil {
		return m.listTenantsFunc(ctx, page)
	}

	return nil, "", nil
}

func (m *mockPublicCatalogService) ListObjects(
	ctx context.Context,
	slug string,
	page model.Page,
) ([]*model.PublicObject, string, error) {
	if m.listObjectsFunc != nil {
		return m.listObjectsFunc(ctx, slug, page)
	}

	return nil, "", nil
}

// handlerTestUser — пользователь, которого «вернул» сервис авторизации.
// Роли у профиля нет: она приходит из членства в тенанте (service.Principal).
func handlerTestUser(t *testing.T) *model.User {
	t.Helper()

	user, err := model.NewUser(model.CreateUserInput{
		Email:       "ivan@example.com",
		FirstName:   "Иван",
		LastName:    "Иванов",
		PhoneNumber: "+79990000001",
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("model.NewUser() unexpected error = %v", err)
	}

	return user
}

// testTenantID — тенант сессии в тестах HTTP-слоя: токен всегда действует в
// рамках тенанта (user_tokens.tenant_id NOT NULL).
const testTenantID = "0f0c8f0e-9f60-4e0e-9d5f-1f6f1f6f1f6f"

// testPrincipal собирает действующего пользователя с ролью в тенанте: роль
// принадлежит членству (tenant_members.role_id), поэтому задаётся явно, а не
// берётся из профиля.
func testPrincipal(t *testing.T, user *model.User, role model.Role) *service.Principal {
	t.Helper()

	now := time.Now().UTC()

	return &service.Principal{
		User: user,
		Token: &model.Token{
			ID:        "1c9edc8e-1f1c-4d5c-8a5e-6f2f1d3c4b5a",
			UserID:    user.ID,
			TenantID:  testTenantID,
			Role:      role,
			IssuedAt:  now,
			ExpiresAt: now.Add(time.Hour),
		},
		Role: role,
	}
}

// authenticatedHandler собирает хендлер, в котором любой непустой токен
// считается действующим, а его владелец — пользователем user.
func authenticatedHandler(t *testing.T, user *model.User) Handler {
	t.Helper()

	principal := testPrincipal(t, user, model.RoleClient)

	auth := &mockAuthService{authenticateFunc: func(_ context.Context, token string) (*service.Principal, error) {
		if token == "" {
			return nil, service.ErrUnauthorized
		}

		return principal, nil
	}}

	handler, err := NewHandler(auth, NewUserServiceServer(&mockUserServiceAPI{}), NewTenantServiceServer(&mockTenantManagementService{}), &mockPublicCatalogService{})
	if err != nil {
		t.Fatalf("NewHandler() unexpected error = %v", err)
	}

	return handler
}

func TestHandler_NewHandler(t *testing.T) {
	tests := []struct {
		name string
		auth AuthService
	}{
		{name: "с сервисом авторизации", auth: &mockAuthService{}},
		{name: "без сервиса авторизации", auth: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := NewHandler(tt.auth, NewUserServiceServer(&mockUserServiceAPI{}), NewTenantServiceServer(&mockTenantManagementService{}), &mockPublicCatalogService{})
			if err != nil {
				t.Fatalf("NewHandler() unexpected error = %v", err)
			}

			if handler == nil {
				t.Error("NewHandler() returned nil, expected non-nil Handler")
			}
		})
	}
}

func TestHandler_Routes(t *testing.T) {
	handler := authenticatedHandler(t, handlerTestUser(t))

	if handler.Routes() == nil {
		t.Fatal("Routes() returned nil http.Handler")
	}
}

func TestHandler_healthHandler(t *testing.T) {
	handler := authenticatedHandler(t, handlerTestUser(t))

	// health открыт (см. publicPaths): пробы не должны требовать токена.
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("healthHandler status = %d, want %d", rec.Code, http.StatusOK)
	}

	if body := rec.Body.String(); body != "ok\n" {
		t.Errorf("healthHandler body = %q, want %q", body, "ok\n")
	}
}

// TestHandler_Gateway проверяет ручки proto-сервиса через gateway: они
// защищены, а коды gRPC переводятся в статусы HTTP.
func TestHandler_Gateway(t *testing.T) {
	handler := authenticatedHandler(t, handlerTestUser(t)).Routes()

	tests := []struct {
		name       string
		method     string
		path       string
		token      string
		wantStatus int
	}{
		{
			name:       "Get без токена — 401",
			method:     http.MethodGet,
			path:       "/api/v1/users/11111111-1111-4111-8111-111111111111",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Get отсутствующего пользователя — 404",
			method:     http.MethodGet,
			path:       "/api/v1/users/11111111-1111-4111-8111-111111111111",
			token:      "access-token",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "Get с невалидным UUID — 400",
			method:     http.MethodGet,
			path:       "/api/v1/users/not-a-uuid",
			token:      "access-token",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Create без токена — 401",
			method:     http.MethodPost,
			path:       "/api/v1/users",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "неизвестная ручка — 404",
			method:     http.MethodGet,
			path:       "/api/v1/unknown",
			token:      "access-token",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.token != "" {
				req.Header.Set(AuthorizationHeader, "Bearer "+tt.token)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("%s %s = %d, want %d (body = %s)",
					tt.method, tt.path, rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

// TestHandler_getCurrentUser проверяет GET /users/me: профиль отдаётся в том же
// формате, что и ручки gateway (protojson), и только с действующим токеном.
func TestHandler_getCurrentUser(t *testing.T) {
	user := handlerTestUser(t)
	handler := authenticatedHandler(t, user).Routes()

	t.Run("без токена — 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/users/me", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("с токеном — профиль пользователя", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/users/me", nil)
		req.Header.Set(AuthorizationHeader, "Bearer access-token")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body = %s)", rec.Code, http.StatusOK, rec.Body.String())
		}

		if contentType := rec.Header().Get("Content-Type"); contentType != "application/json; charset=utf-8" {
			t.Errorf("Content-Type = %q, want %q", contentType, "application/json; charset=utf-8")
		}

		body := rec.Body.String()

		for _, want := range []string{user.ID, user.Email, user.FirstName, user.LastName} {
			if !strings.Contains(body, want) {
				t.Errorf("тело ответа %q не содержит %q", body, want)
			}
		}
	})
}

// TestHandler_PublicPaths проверяет, что открытые ручки доступны без токена, а
// защищённые — отвечают 401.
func TestHandler_PublicPaths(t *testing.T) {
	handler := authenticatedHandler(t, handlerTestUser(t)).Routes()

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{name: "health открыт", method: http.MethodGet, path: "/health", wantStatus: http.StatusOK},
		{name: "вход открыт", method: http.MethodPost, path: "/auth/login", wantStatus: http.StatusUnauthorized},
		{name: "регистрация открыта", method: http.MethodPost, path: "/auth/register", wantStatus: http.StatusForbidden},
		{name: "смена пароля открыта", method: http.MethodPost, path: "/auth/password", wantStatus: http.StatusUnauthorized},
		// swagger без токена отдаёт сам UI (файл swagger-ui.html из pkg/api):
		// index.html в статике нет, а http.FileServer на путь, оканчивающийся
		// на /index.html, отвечает 301-редиректом — проверяем реальную страницу.
		{name: "swagger открыт", method: http.MethodGet, path: "/api/swagger/swagger-ui.html", wantStatus: http.StatusOK},
		{name: "профиль защищён", method: http.MethodGet, path: "/users/me", wantStatus: http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader("{}"))
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			// Открытые ручки доходят до своих хендлеров (и отвечают их кодами),
			// защищённые — нет: middleware отвечает 401 без токена.
			if rec.Code != tt.wantStatus {
				t.Errorf("%s %s = %d, want %d (body = %s)",
					tt.method, tt.path, rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

// TestHandler_AuthenticateError проверяет ответ middleware, когда сервис
// авторизации отказывает: 401 с фиксированным текстом, без деталей причины.
func TestHandler_AuthenticateError(t *testing.T) {
	tests := []struct {
		name       string
		authErr    error
		wantStatus int
		wantBody   string
	}{
		{name: "нет токена", authErr: nil, wantStatus: http.StatusUnauthorized, wantBody: unauthorizedMessage},
		{name: "отказ сервиса", authErr: service.ErrUnauthorized, wantStatus: http.StatusUnauthorized, wantBody: unauthorizedMessage},
		{name: "ошибка базы", authErr: errors.New("database is down"), wantStatus: http.StatusInternalServerError, wantBody: internalErrorMessage},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := &mockAuthService{authenticateFunc: func(_ context.Context, token string) (*service.Principal, error) {
				if tt.authErr == nil {
					if token == "" {
						return nil, service.ErrUnauthorized
					}

					return testPrincipal(t, handlerTestUser(t), model.RoleClient), nil
				}

				return nil, tt.authErr
			}}

			handler, err := NewHandler(auth, NewUserServiceServer(&mockUserServiceAPI{}), NewTenantServiceServer(&mockTenantManagementService{}), &mockPublicCatalogService{})
			if err != nil {
				t.Fatalf("NewHandler() unexpected error = %v", err)
			}

			req := httptest.NewRequest(http.MethodGet, "/users/me", nil)
			if tt.authErr != nil {
				req.Header.Set(AuthorizationHeader, "Bearer access-token")
			}

			rec := httptest.NewRecorder()
			handler.Routes().ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want содержит %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

// TestHandler_loginPasswordExpired проверяет ответ на вход с просроченным
// паролем: 403 с признаком password_expired, по которому клиент ведёт
// пользователя на смену пароля.
func TestHandler_loginPasswordExpired(t *testing.T) {
	auth := &mockAuthService{loginFunc: func(context.Context, string, string, string) (*service.Session, error) {
		return nil, service.ErrPasswordExpired
	}}

	handler, err := NewHandler(auth, NewUserServiceServer(&mockUserServiceAPI{}), NewTenantServiceServer(&mockTenantManagementService{}), &mockPublicCatalogService{})
	if err != nil {
		t.Fatalf("NewHandler() unexpected error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/auth/login",
		strings.NewReader(`{"email":"ivan@example.com","password":"secret-pass"}`))
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (body = %s)", rec.Code, http.StatusForbidden, rec.Body.String())
	}

	body := rec.Body.String()

	if !strings.Contains(body, passwordExpiredMessage) || !strings.Contains(body, passwordExpiredCode) {
		t.Errorf("body = %q, want %q и %q", body, passwordExpiredMessage, passwordExpiredCode)
	}
}

// TestHandler_changePassword проверяет POST /auth/password: ручка открыта
// (токена нет), отвечает сессией либо ошибкой.
func TestHandler_changePassword(t *testing.T) {
	user := handlerTestUser(t)

	t.Run("успех — 200 с токеном", func(t *testing.T) {
		var gotEmail, gotCurrent, gotNew string

		auth := &mockAuthService{changePasswordFunc: func(
			_ context.Context,
			email, currentPassword, newPassword, tenantSlug string,
		) (*service.Session, error) {
			gotEmail, gotCurrent, gotNew = email, currentPassword, newPassword

			if tenantSlug != "" {
				t.Errorf("ChangePassword() tenant slug = %q, want empty", tenantSlug)
			}

			now := time.Now().UTC()

			return &service.Session{
				AccessToken: "new-token",
				Token: &model.Token{
					ID:        "1c9edc8e-1f1c-4d5c-8a5e-6f2f1d3c4b5a",
					UserID:    user.ID,
					TenantID:  testTenantID,
					Role:      model.RoleClient,
					IssuedAt:  now,
					ExpiresAt: now.Add(time.Hour),
				},
			}, nil
		}}

		handler, err := NewHandler(auth, NewUserServiceServer(&mockUserServiceAPI{}), NewTenantServiceServer(&mockTenantManagementService{}), &mockPublicCatalogService{})
		if err != nil {
			t.Fatalf("NewHandler() unexpected error = %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/auth/password", strings.NewReader(
			`{"email":"ivan@example.com","current_password":"old-password","new_password":"new-password"}`,
		))
		rec := httptest.NewRecorder()
		handler.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body = %s)", rec.Code, http.StatusOK, rec.Body.String())
		}

		if !strings.Contains(rec.Body.String(), "new-token") {
			t.Errorf("body = %q, want токен", rec.Body.String())
		}

		if gotEmail != "ivan@example.com" || gotCurrent != "old-password" || gotNew != "new-password" {
			t.Errorf("ChangePassword(%q, %q, %q)", gotEmail, gotCurrent, gotNew)
		}
	})

	t.Run("неверные данные — 401", func(t *testing.T) {
		auth := &mockAuthService{changePasswordFunc: func(context.Context, string, string, string, string) (*service.Session, error) {
			return nil, service.ErrInvalidCredentials
		}}

		handler, err := NewHandler(auth, NewUserServiceServer(&mockUserServiceAPI{}), NewTenantServiceServer(&mockTenantManagementService{}), &mockPublicCatalogService{})
		if err != nil {
			t.Fatalf("NewHandler() unexpected error = %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/auth/password", strings.NewReader(`{}`))
		rec := httptest.NewRecorder()
		handler.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("битый JSON — 400", func(t *testing.T) {
		handler, err := NewHandler(&mockAuthService{}, NewUserServiceServer(&mockUserServiceAPI{}), NewTenantServiceServer(&mockTenantManagementService{}), &mockPublicCatalogService{})
		if err != nil {
			t.Fatalf("NewHandler() unexpected error = %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/auth/password", strings.NewReader("not json"))
		rec := httptest.NewRecorder()
		handler.Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})
}
