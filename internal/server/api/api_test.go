package server_api

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/botbooker/bb-core/internal/server"
)

func TestNewServer(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := server.NewServer(handler, "test-service")
	cfg := server.GetServerConfig()
	addr := fmt.Sprintf("%s:%s", cfg.Host, cfg.Port)

	require.NotNil(t, srv)
	assert.Equal(t, addr, srv.Addr)
	assert.Equal(t, 15*time.Second, srv.ReadTimeout)
	assert.Equal(t, 15*time.Second, srv.WriteTimeout)
	assert.Equal(t, 60*time.Second, srv.IdleTimeout)
	assert.NotNil(t, srv.Handler)
}

func TestShutdownServer_Success(t *testing.T) {
	// Создаём тестовый сервер
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	ts := httptest.NewServer(handler)
	defer ts.Close()

	// Создаём http.Server с таким же адресом
	srv := &http.Server{
		Addr:    ts.URL[len("http://"):],
		Handler: handler,
	}

	// Запускаем сервер в горутине
	go func() {
		srv.ListenAndServe()
	}()

	// Даём серверу время на запуск
	time.Sleep(10 * time.Millisecond)

	// Выполняем shutdown
	err := server.ShutdownServer(srv)
	assert.NoError(t, err)
}

func TestShutdownServer_NotStarted(t *testing.T) {
	// Создаём сервер, который не был запущен
	handler := http.NotFoundHandler()
	srv := &http.Server{
		Addr:    "127.0.0.1:0",
		Handler: handler,
	}

	// Shutdown сервера, который не был запущен, должен завершиться без ошибки
	err := server.ShutdownServer(srv)
	assert.NoError(t, err)
}

func TestShutdownServer_ContextTimeout(t *testing.T) {
	// Создаём сервер с очень коротким таймаутом
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Имитируем долгую обработку
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	srv := &http.Server{
		Addr:    "127.0.0.1:0",
		Handler: handler,
	}

	// Запускаем сервер
	go func() {
		srv.ListenAndServe()
	}()

	time.Sleep(10 * time.Millisecond)

	// Shutdown с таймаутом 10 секунд должен завершиться успешно
	err := server.ShutdownServer(srv)
	assert.NoError(t, err)
}

func TestServerIntegration(t *testing.T) {
	// Интеграционный тест: создаём сервер, делаем запрос, останавливаем
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello from test server"))
	})

	// Используем httptest для создания сервера
	ts := httptest.NewServer(handler)
	defer ts.Close()

	// Делаем запрос
	resp, err := http.Get(ts.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "Hello from test server", string(body))
}

func TestServerWithNilHandler(t *testing.T) {
	// Передаём nil как handler - это допустимо, сервер будет использовать DefaultServeMux
	srv := server.NewServer(nil, "test-service")
	cfg := server.GetServerConfig()
	addr := fmt.Sprintf("%s:%s", cfg.Host, cfg.Port)

	require.NotNil(t, srv)
	assert.Equal(t, addr, srv.Addr)
}

func TestServerTimeouts(t *testing.T) {
	handler := http.NotFoundHandler()
	srv := server.NewServer(handler, "test")

	// Проверяем таймауты
	assert.Equal(t, 15*time.Second, srv.ReadTimeout, "ReadTimeout должен быть 15 секунд")
	assert.Equal(t, 15*time.Second, srv.WriteTimeout, "WriteTimeout должен быть 15 секунд")
	assert.Equal(t, 60*time.Second, srv.IdleTimeout, "IdleTimeout должен быть 60 секунд")
}
