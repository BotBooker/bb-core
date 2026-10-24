package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// readycheckServer запускает httptest-сервер, который отвечает на GET /ready
// заданным статусом, и возвращает его адрес в формате host:port — ровно в таком
// виде пробе передаётся SERVER_HOST:SERVER_PORT.
func readycheckServer(t *testing.T, status int) string {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != readyPath {
			t.Errorf("readiness probe requested %s, want %s", r.URL.Path, readyPath)
			http.NotFound(w, r)

			return
		}

		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)

	return strings.TrimPrefix(srv.URL, "http://")
}

// withReadycheckAddr подменяет адрес, который опрашивает проба готовности.
func withReadycheckAddr(t *testing.T, addr string) {
	t.Helper()

	original := readycheckAddr
	readycheckAddr = func() string { return addr }
	t.Cleanup(func() { readycheckAddr = original })
}

func TestReadycheckURL(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want string
	}{
		{name: "loopback", addr: "127.0.0.1:8080", want: "http://127.0.0.1:8080/ready"},
		{name: "hostname", addr: "localhost:8080", want: "http://localhost:8080/ready"},
		{name: "all interfaces ipv4", addr: "0.0.0.0:8080", want: "http://127.0.0.1:8080/ready"},
		{name: "all interfaces ipv6", addr: "[::]:8080", want: "http://127.0.0.1:8080/ready"},
		{name: "empty host", addr: ":8080", want: "http://127.0.0.1:8080/ready"},
		{name: "without port", addr: "0.0.0.0", want: "http://0.0.0.0/ready"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := readycheckURL(tt.addr); got != tt.want {
				t.Errorf("readycheckURL(%q) = %q, want %q", tt.addr, got, tt.want)
			}
		})
	}
}

func TestRunReadycheck(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		wantErr bool
	}{
		{name: "ready", status: http.StatusOK},
		{name: "not ready", status: http.StatusServiceUnavailable, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runReadycheck(readycheckServer(t, tt.status))
			if (err != nil) != tt.wantErr {
				t.Fatalf("runReadycheck() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestRunReadycheck_ConnectionRefused проверяет сценарий «сервер не слушает
// порт»: Docker HEALTHCHECK в этом случае должен получить код 1, а не зависнуть.
func TestRunReadycheck_ConnectionRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	addr := strings.TrimPrefix(srv.URL, "http://")
	srv.Close()

	if err := runReadycheck(addr); err == nil {
		t.Fatal("runReadycheck() = nil for closed listener, want error")
	}
}

// TestMain_Readycheck проверяет режим --readycheck целиком: проба не поднимает
// серверы, а код возврата отражает готовность экземпляра.
func TestMain_Readycheck(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		wantCode int
	}{
		{name: "ready", status: http.StatusOK, wantCode: 0},
		{name: "not ready", status: http.StatusServiceUnavailable, wantCode: exitCodeFailure},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withArgs(t, "--readycheck")
			withReadycheckAddr(t, readycheckServer(t, tt.status))
			withServer(t, &fakeServer{runFunc: func() error {
				t.Error("Main() started the server in readycheck mode")

				return nil
			}})

			if code := Main(); code != tt.wantCode {
				t.Errorf("Main() = %d, want %d", code, tt.wantCode)
			}
		})
	}
}

// TestMain_ReadycheckUnavailable проверяет, что недоступный экземпляр даёт код 1.
func TestMain_ReadycheckUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	addr := strings.TrimPrefix(srv.URL, "http://")
	srv.Close()

	withArgs(t, "--readycheck")
	withReadycheckAddr(t, addr)

	if code := Main(); code != exitCodeFailure {
		t.Errorf("Main() = %d, want %d", code, exitCodeFailure)
	}
}
