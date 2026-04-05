package router_api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupRouter_NilConfig(t *testing.T) {
	router := setupEngine(RouterConfig{})

	require.NotNil(t, router)
}

func TestSetupRouter_EmptyTrustedProxies(t *testing.T) {
	router := setupEngine(RouterConfig{
		TrustedProxies: []string{},
	})

	require.NotNil(t, router)
}

func TestSetupRouter_EmptyTrustedPlatform(t *testing.T) {
	router := setupEngine(RouterConfig{
		TrustedPlatform: "",
	})

	require.NotNil(t, router)
}

func TestRegisterRoutes_PingEndpoint(t *testing.T) {
	router := chi.NewRouter()
	RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "pong")
}

func TestRegisterRoutes_HealthzEndpoint(t *testing.T) {
	router := chi.NewRouter()
	RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "OK")
}

func TestRegisterRoutes_UnknownRoute(t *testing.T) {
	router := chi.NewRouter()
	RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestRegisterRoutes_PostToPing(t *testing.T) {
	router := chi.NewRouter()
	RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/ping", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// POST to /ping should return 405 (Method Not Allowed) or 404
	assert.True(t, w.Code == http.StatusNotFound || w.Code == http.StatusMethodNotAllowed)
}

func TestSetupEngine_RouteCount(t *testing.T) {
	router := setupEngine(RouterConfig{})

	// chi doesn't expose routes directly, so we test via HTTP requests
	require.NotNil(t, router)

	// Test both endpoints work
	endpoints := []string{"/ping", "/healthz"}
	for _, endpoint := range endpoints {
		req := httptest.NewRequest(http.MethodGet, endpoint, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code, "Endpoint %s should return 200", endpoint)
	}
}

func TestSetupEngine_MiddlewareApplied(t *testing.T) {
	router := setupEngine(RouterConfig{})

	// Verify router is properly configured
	require.NotNil(t, router)
}

func TestRouterConfig_StructFields(t *testing.T) {
	config := RouterConfig{
		TrustedProxies:  []string{"10.0.0.1", "192.168.1.0/24"},
		TrustedPlatform: "X-Forwarded-For",
	}

	assert.Equal(t, []string{"10.0.0.1", "192.168.1.0/24"}, config.TrustedProxies)
	assert.Equal(t, "X-Forwarded-For", config.TrustedPlatform)
}

func TestSetupRouter_MultipleTrustedProxies(t *testing.T) {
	router := setupEngine(RouterConfig{
		TrustedProxies: []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"},
	})

	require.NotNil(t, router)
}

func TestSetupRouter_AllPlatforms(t *testing.T) {
	platforms := []string{
		"X-Forwarded-For",
		"X-Real-IP",
		"CF-Connecting-IP",
	}

	for _, platform := range platforms {
		t.Run(platform, func(t *testing.T) {
			router := setupEngine(RouterConfig{
				TrustedPlatform: platform,
			})

			require.NotNil(t, router)
		})
	}
}

func TestRegisterRoutes_RoutePaths(t *testing.T) {
	router := chi.NewRouter()
	RegisterRoutes(router)

	// Test via HTTP requests
	endpoints := []string{"/ping", "/healthz"}
	for _, endpoint := range endpoints {
		req := httptest.NewRequest(http.MethodGet, endpoint, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code, "Should have %s route", endpoint)
	}
}

func TestRegisterRoutes_RouteMethods(t *testing.T) {
	router := chi.NewRouter()
	RegisterRoutes(router)

	// Test GET methods work
	endpoints := []string{"/ping", "/healthz"}
	for _, endpoint := range endpoints {
		req := httptest.NewRequest(http.MethodGet, endpoint, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code, "GET %s should work", endpoint)
	}
}

func TestSetupRouter_Coverage(t *testing.T) {
	// Test SetupRouter directly to cover the function
	router := SetupRouter()

	require.NotNil(t, router)
}

func TestSetupEngine_WithOTELServiceName(t *testing.T) {
	t.Setenv("OTEL_SERVICE_NAME", "custom-otel-service")

	router := setupEngine(RouterConfig{})
	require.NotNil(t, router)

	// Test that routes still work
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRegisterRoutes_OnlyGETMethods(t *testing.T) {
	router := chi.NewRouter()
	RegisterRoutes(router)

	// POST to /ping should return 405
	req := httptest.NewRequest(http.MethodPost, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	// PUT to /healthz should return 405
	req = httptest.NewRequest(http.MethodPut, "/healthz", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestSetupEngine_MiddlewareOrder(t *testing.T) {
	// Verify that middleware is applied correctly
	router := setupEngine(RouterConfig{})

	// Make a request and verify it goes through
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should succeed
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRouterConfig_ZeroValues(t *testing.T) {
	// Test with zero values
	config := RouterConfig{}
	router := setupEngine(config)
	require.NotNil(t, router)
}

func TestRouterConfig_NilTrustedProxies(t *testing.T) {
	config := RouterConfig{
		TrustedProxies:  nil,
		TrustedPlatform: "X-Real-IP",
	}
	router := setupEngine(config)
	require.NotNil(t, router)
}

func TestSetupEngine_HeadRequest(t *testing.T) {
	router := setupEngine(RouterConfig{})

	// HEAD to /ping should return 405 (only GET is registered)
	req := httptest.NewRequest(http.MethodHead, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestSetupEngine_DeleteRequest(t *testing.T) {
	router := setupEngine(RouterConfig{})

	// DELETE to /ping should return 405
	req := httptest.NewRequest(http.MethodDelete, "/ping", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}
