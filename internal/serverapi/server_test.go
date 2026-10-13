package serverapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/botbooker/bb-core/internal/config"
)

// withGRPCListener подменяет слушатель gRPC на свободный порт: тесты не должны
// занимать порт из конфигурации (он может быть занят запущенным приложением).
func withGRPCListener(t *testing.T) net.Listener {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() unexpected error = %v", err)
	}

	original := listenGRPC
	listenGRPC = func(_, _ string) (net.Listener, error) { return listener, nil }

	t.Cleanup(func() {
		listenGRPC = original
		_ = listener.Close()
	})

	return listener
}

func TestServerAPI_New(t *testing.T) {
	withDatabase(t, &fakeDB{}, nil)
	withGRPCListener(t)

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
	if a.grpcServer == nil {
		t.Error("grpcServer should not be nil after initDeps()")
	}
	if a.grpcListener == nil {
		t.Error("grpcListener should not be nil after initDeps()")
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
	if a.grpcServer != nil {
		t.Error("grpcServer should not be created when the database is unavailable")
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

	// Verify GRPCAddr is constructed
	if cfg.GRPCAddr == "" {
		t.Error("GRPCAddr should not be empty")
	}
}

func TestServerAPI_HandlerRoutes(t *testing.T) {
	withDatabase(t, &fakeDB{}, nil)
	withGRPCListener(t)

	a := New()
	if err := a.initDeps(context.Background()); err != nil {
		t.Fatalf("initDeps() unexpected error = %v", err)
	}

	handler, err := a.diContainer.Handler(context.Background())
	if err != nil {
		t.Fatalf("Handler() unexpected error = %v", err)
	}

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

	// Test users/me endpoint without auth: the route is protected by the auth
	// middleware, so an anonymous request must be rejected with 401.
	resp, err = http.Get(srv.URL + "/users/me")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Users/me endpoint returned status %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

// TestServerAPI_InitGRPCServerListenError проверяет, что занятый (или иначе
// недоступный) порт gRPC валит старт приложения, а не проходит незамеченным.
func TestServerAPI_InitGRPCServerListenError(t *testing.T) {
	withDatabase(t, &fakeDB{}, nil)

	original := listenGRPC
	listenGRPC = func(_, _ string) (net.Listener, error) { return nil, errTest }

	t.Cleanup(func() { listenGRPC = original })

	a := New()

	err := a.initDeps(context.Background())
	if !errors.Is(err, errTest) {
		t.Errorf("initDeps() error = %v, want %v", err, errTest)
	}
	if a.grpcServer != nil {
		t.Error("grpcServer should not be created when the port is unavailable")
	}
}
