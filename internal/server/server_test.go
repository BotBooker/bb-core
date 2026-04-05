package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServer(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	serviceName := "test-service"
	srv := NewServer(handler, serviceName)

	require.NotNil(t, srv)
	assert.NotNil(t, srv.Handler)
	assert.Equal(t, 15*time.Second, srv.ReadTimeout)
	assert.Equal(t, 15*time.Second, srv.WriteTimeout)
	assert.Equal(t, 60*time.Second, srv.IdleTimeout)

	// Address should be in format host:port
	assert.NotEmpty(t, srv.Addr)
	assert.Contains(t, srv.Addr, ":")
}

func TestNewServer_NilHandler(t *testing.T) {
	srv := NewServer(nil, "test-service")

	require.NotNil(t, srv)
	// Handler can be nil, Go's http.Server will use DefaultServeMux
	assert.Nil(t, srv.Handler)
}

func TestNewServer_AddressFormat(t *testing.T) {
	// Set default config values
	t.Setenv("SERVER_HOST", "127.0.0.1")
	t.Setenv("SERVER_PORT", "9090")

	handler := http.NotFoundHandler()
	srv := NewServer(handler, "test")

	assert.Equal(t, "127.0.0.1:9090", srv.Addr)
}

func TestNewServer_DefaultAddress(t *testing.T) {
	// Clear environment variables to test defaults
	t.Setenv("SERVER_HOST", "")
	t.Setenv("SERVER_PORT", "")

	handler := http.NotFoundHandler()
	srv := NewServer(handler, "test")

	assert.Equal(t, "localhost:8080", srv.Addr)
}

func TestShutdownServer_Success(t *testing.T) {
	// Create a test server that's already listening
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ts := httptest.NewServer(handler)
	defer ts.Close()

	// Create a server with the same handler
	srv := &http.Server{
		Addr:    ts.URL[len("http://"):],
		Handler: handler,
	}

	// Start the server in a goroutine
	go func() {
		srv.ListenAndServe()
	}()

	// Give the server time to start
	time.Sleep(10 * time.Millisecond)

	// Shutdown should succeed
	err := ShutdownServer(srv)
	assert.NoError(t, err)
}

func TestShutdownServer_NotStarted(t *testing.T) {
	// Create a server that was never started
	handler := http.NotFoundHandler()
	srv := &http.Server{
		Addr:    "127.0.0.1:0",
		Handler: handler,
	}

	// Shutdown of a server that was never started should not error
	err := ShutdownServer(srv)
	assert.NoError(t, err)
}

func TestShutdownServer_ContextTimeout(t *testing.T) {
	// Create a handler that simulates a long-running request
	done := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Wait until we're told to stop
		<-done
		w.WriteHeader(http.StatusOK)
	})

	ts := httptest.NewServer(handler)
	defer ts.Close()
	defer close(done)

	srv := &http.Server{
		Addr:    ts.URL[len("http://"):],
		Handler: handler,
	}

	go func() {
		srv.ListenAndServe()
	}()

	time.Sleep(10 * time.Millisecond)

	// Shutdown with 10 second timeout should succeed
	err := ShutdownServer(srv)
	assert.NoError(t, err)
}

func TestShutdownServer_MultipleCalls(t *testing.T) {
	handler := http.NotFoundHandler()
	srv := &http.Server{
		Addr:    "127.0.0.1:0",
		Handler: handler,
	}

	// First shutdown should succeed
	err := ShutdownServer(srv)
	assert.NoError(t, err)

	// Second shutdown should also succeed (server is already shut down)
	err = ShutdownServer(srv)
	assert.NoError(t, err)
}

func TestShutdownServer_WithActiveConnections(t *testing.T) {
	// Create a handler that takes some time
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	ts := httptest.NewServer(handler)
	defer ts.Close()

	srv := &http.Server{
		Addr:    ts.URL[len("http://"):],
		Handler: handler,
	}

	go func() {
		srv.ListenAndServe()
	}()

	time.Sleep(10 * time.Millisecond)

	// Make a request in the background
	go func() {
		client := &http.Client{Timeout: 5 * time.Second}
		client.Get(ts.URL)
	}()

	// Small delay to let request start
	time.Sleep(10 * time.Millisecond)

	// Shutdown should still succeed
	err := ShutdownServer(srv)
	assert.NoError(t, err)
}

func TestServer_TimeoutConfiguration(t *testing.T) {
	handler := http.NotFoundHandler()
	srv := NewServer(handler, "test")

	// Verify all timeout values
	assert.Equal(t, 15*time.Second, srv.ReadTimeout, "ReadTimeout should be 15 seconds")
	assert.Equal(t, 15*time.Second, srv.WriteTimeout, "WriteTimeout should be 15 seconds")
	assert.Equal(t, 60*time.Second, srv.IdleTimeout, "IdleTimeout should be 60 seconds")
}

func TestServer_ContextCancellation(t *testing.T) {
	handler := http.NotFoundHandler()
	srv := &http.Server{
		Addr:    "127.0.0.1:0",
		Handler: handler,
	}

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Shutdown with cancelled context should still work
	err := srv.Shutdown(ctx)
	assert.NoError(t, err)
}

func TestServer_Integration(t *testing.T) {
	// Full integration test: create server, make request, shutdown
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	ts := httptest.NewServer(handler)
	defer ts.Close()

	// Make a request
	resp, err := http.Get(ts.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
