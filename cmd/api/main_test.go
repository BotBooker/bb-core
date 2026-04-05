package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/botbooker/bb-core/internal/observability/otel"
	router "github.com/botbooker/bb-core/internal/router/api"
)

// setupTestRouter creates a test router using the api router package
func setupTestRouter() *chi.Mux {
	return router.SetupRouter()
}

func TestPingEndpoint(t *testing.T) {
	// Создаём тестовый роутер
	r := setupTestRouter()

	ctx := context.Background()

	// Создаём запрос
	req, err := http.NewRequestWithContext(ctx, "GET", "/ping", nil)
	if err != nil {
		t.Fatal("не удалось создать запрос:", err)
	}
	w := httptest.NewRecorder()

	// Выполняем запрос
	r.ServeHTTP(w, req)

	// Проверяем статус
	if w.Code != http.StatusOK {
		t.Errorf("Ожидаемый статус %d, получен %d", http.StatusOK, w.Code)
	}

	// Проверяем Content-Type
	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Content-Type должен быть application/json, получен %s", contentType)
	}

	// Декодируем ответ
	var response map[string]string
	err = json.NewDecoder(w.Body).Decode(&response)
	if err != nil {
		t.Fatalf("Не удалось декодировать JSON: %v", err)
	}

	// Проверяем поле "message"
	if response["message"] != "pong" {
		t.Errorf("Поле 'message' должно быть 'pong', получено %s", response["message"])
	}
}

func TestHealthEndpoint(t *testing.T) {
	// Создаём тестовый роутер
	r := setupTestRouter()

	ctx := context.Background()

	req, err := http.NewRequestWithContext(ctx, "GET", "/healthz", nil)
	if err != nil {
		t.Fatal("не удалось создать запрос:", err)
	}
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Ожидаемый статус %d, получен %d", http.StatusOK, w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Content-Type должен быть application/json, получен %s", contentType)
	}

	// Проверяем, что ответ не пустой
	if w.Body.Len() == 0 {
		t.Error("Ответ /healthz пуст")
	}
}

func TestPingLogsTraceInfo(t *testing.T) {
	var logOutput strings.Builder
	otel.LogHook = func(format string, a ...any) {
		logOutput.WriteString(format)
	}

	r := setupTestRouter()

	ctx := context.Background()

	// Создаём запрос
	req, err := http.NewRequestWithContext(ctx, "GET", "/ping", nil)
	if err != nil {
		t.Fatal("не удалось создать запрос:", err)
	}
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if !strings.Contains(logOutput.String(), "traceID:") {
		t.Error("В логах отсутствует traceID")
	}
	if !strings.Contains(logOutput.String(), "spanID:") {
		t.Error("В логах отсутствует spanID")
	}

	// Сброс хука после теста
	otel.LogHook = nil
}
