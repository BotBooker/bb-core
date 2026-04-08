package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
)

func TestSetupRouter(t *testing.T) {
	router := SetupRouter()

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
	router := setupEngine()

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
	router := setupEngine()

	req, err := http.NewRequest("GET", "/healthz", nil)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "OK")
}
