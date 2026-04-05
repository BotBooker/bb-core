package router_api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
)

func TestSetupRouter(t *testing.T) {
	router := setupEngine(RouterConfig{})

	assert.NotNil(t, router)
}

func TestRegisterRoutes(t *testing.T) {
	router := chi.NewRouter()
	RegisterRoutes(router)

	// Проверяем, что маршруты зарегистрированы через тестовые запросы
	// chi doesn't expose routes directly, so we test via HTTP requests

	// Test /ping
	req := httptest.NewRequest("GET", "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test /healthz
	req = httptest.NewRequest("GET", "/healthz", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPingRoute(t *testing.T) {
	router := setupEngine(RouterConfig{})

	req, err := http.NewRequest("GET", "/ping", nil)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "pong")
}

func TestHealthRoute(t *testing.T) {
	router := setupEngine(RouterConfig{})

	req, err := http.NewRequest("GET", "/healthz", nil)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "OK")
}

func TestSetupRouter_WithTrustedProxies(t *testing.T) {
	router := setupEngine(RouterConfig{
		TrustedProxies: []string{"127.0.0.1", "192.168.1.0/24"},
	})

	assert.NotNil(t, router)
}

func TestSetupRouter_WithTrustedPlatform(t *testing.T) {
	router := setupEngine(RouterConfig{
		TrustedPlatform: "X-Forwarded-For",
	})

	assert.NotNil(t, router)
}

func TestSetupRouter_WithFullConfig(t *testing.T) {
	router := setupEngine(RouterConfig{
		TrustedProxies:  []string{"10.0.0.0/8"},
		TrustedPlatform: "X-Real-IP",
	})

	assert.NotNil(t, router)
}
