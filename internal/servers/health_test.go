package servers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newHealthHandler собирает хендлер с заданной проверкой БД: остальные
// зависимости для проб не важны.
func newHealthHandler(t *testing.T, readiness readinessChecker) Handler {
	t.Helper()

	handler, err := NewHandler(
		&mockAuthService{},
		NewUserServiceServer(&mockUserServiceAPI{}),
		NewTenantServiceServer(&mockTenantManagementService{}),
		&mockPublicCatalogService{},
		readiness,
	)
	if err != nil {
		t.Fatalf("NewHandler() unexpected error = %v", err)
	}

	return handler
}

// TestHandler_Health проверяет liveness: ручка отвечает 200 без токена, даже
// если база недоступна — перезапуск контейнера не лечит упавшую базу, а трафик
// снимает readiness.
func TestHandler_Health(t *testing.T) {
	handler := newHealthHandler(t, mockReadiness{err: errors.New("database is down")})

	recorder := httptest.NewRecorder()
	handler.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, healthPath, nil))

	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	if body := strings.TrimSpace(recorder.Body.String()); body != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
}

// TestHandler_Ready проверяет readiness: пока база отвечает — 200, при ошибке
// ping — 503 без деталей подключения (адрес и DSN наружу не отдаются).
func TestHandler_Ready(t *testing.T) {
	t.Run("база доступна", func(t *testing.T) {
		handler := newHealthHandler(t, mockReadiness{})

		recorder := httptest.NewRecorder()
		handler.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, readyPath, nil))

		if recorder.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", recorder.Code, http.StatusOK)
		}

		if body := strings.TrimSpace(recorder.Body.String()); body != "ready" {
			t.Errorf("body = %q, want %q", body, "ready")
		}
	})

	t.Run("база недоступна", func(t *testing.T) {
		handler := newHealthHandler(t, mockReadiness{err: errors.New("dial tcp 10.0.0.5:5432: connection refused")})

		recorder := httptest.NewRecorder()
		handler.Routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, readyPath, nil))

		if recorder.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
		}

		body := recorder.Body.String()
		if !strings.Contains(body, "not ready") {
			t.Errorf("body = %q, want %q", body, "not ready")
		}

		if strings.Contains(body, "5432") {
			t.Errorf("body = %q: наружу попали детали подключения", body)
		}
	})
}
