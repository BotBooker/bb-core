package serverapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/botbooker/bb-core/internal/config"
)

func TestNew(t *testing.T) {
	// Save and restore env
	originalHost := os.Getenv("SERVER_HOST")
	originalPort := os.Getenv("SERVER_PORT")
	defer func() {
		os.Setenv("SERVER_HOST", originalHost)
		os.Setenv("SERVER_PORT", originalPort)
	}()
	os.Setenv("SERVER_HOST", "localhost")
	os.Setenv("SERVER_PORT", "8080")

	a := New()
	if a == nil {
		t.Fatal("New() returned nil")
	}
	if a.diContainer == nil {
		t.Error("diContainer should not be nil")
	}
}

func TestConfigAppConfig(t *testing.T) {
	// Test that config.AppConfig() works
	cfg := config.AppConfig()
	if cfg == nil {
		t.Fatal("config.AppConfig() returned nil")
	}

	// Verify HTTPAddr is constructed
	if cfg.HTTPAddr == "" {
		t.Error("HTTPAddr should not be empty")
	}
}

func TestHandlerRoutes(t *testing.T) {
	// Save and restore env
	originalDSN := os.Getenv("DSN")
	originalRedis := os.Getenv("REDIS_HOST")
	defer func() {
		os.Setenv("DSN", originalDSN)
		os.Setenv("REDIS_HOST", originalRedis)
	}()
	os.Setenv("DSN", "postgres://test:5432/test")
	os.Setenv("REDIS_HOST", "localhost:6379")

	a := New()
	handler := a.diContainer.Handler()
	routes := handler.Routes()

	// Create a test server
	srv := httptest.NewServer(routes)
	defer srv.Close()

	// Test health endpoint
	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Health check returned status %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Test users/me endpoint without auth
	resp, err = http.Get(srv.URL + "/users/me")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Users/me endpoint returned status %d, want %d", resp.StatusCode, http.StatusOK)
	}
}
