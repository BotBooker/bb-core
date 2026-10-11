package serverapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/botbooker/bb-core/internal/config"
)

func TestServerAPI_New(t *testing.T) {
	withDatabase(t, &fakeDB{}, nil)

	a := New()
	if a == nil {
		t.Fatal("New() returned nil")
	}

	// Зависимости создаются в initDeps: до его вызова контейнера ещё нет.
	if a.diContainer != nil {
		t.Error("diContainer should be nil before initDeps()")
	}

	if err := a.initDeps(context.Background()); err != nil {
		t.Fatalf("initDeps() unexpected error = %v", err)
	}
	if a.diContainer == nil {
		t.Error("diContainer should not be nil after initDeps()")
	}
	if a.httpServer == nil {
		t.Error("httpServer should not be nil after initDeps()")
	}
}

func TestServerAPI_InitDepsDatabaseError(t *testing.T) {
	withDatabase(t, nil, errTest)

	a := New()

	err := a.initDeps(context.Background())
	if !errors.Is(err, errTest) {
		t.Errorf("initDeps() error = %v, want %v", err, errTest)
	}
	if a.diContainer != nil {
		t.Error("diContainer should be nil when the database is unavailable")
	}
	if a.httpServer != nil {
		t.Error("httpServer should not be created when the database is unavailable")
	}
}

func TestServerAPI_AppConfig(t *testing.T) {
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

func TestServerAPI_HandlerRoutes(t *testing.T) {
	withDatabase(t, &fakeDB{}, nil)

	a := New()
	if err := a.initDeps(context.Background()); err != nil {
		t.Fatalf("initDeps() unexpected error = %v", err)
	}

	routes := a.diContainer.Handler().Routes()

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
