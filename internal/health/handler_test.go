package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/botbooker/bb-core/internal/observability/otel"
)

func TestHealth(t *testing.T) {
	// Создаем новый HTTP-запрос
	req, err := http.NewRequest("GET", "/healthz", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Создаем ответ
	w := httptest.NewRecorder()

	// Вызываем обработчик
	Health(w, req)

	// Проверяем код состояния
	assert.Equal(t, http.StatusOK, w.Code)

	// Проверяем Content-Type
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	// Проверяем тело ответа
	var response map[string]string
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "OK", response["message"])
}

func TestPingHandler(t *testing.T) {
	// Создаем новый HTTP-запрос
	req, err := http.NewRequest("GET", "/ping", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Создаем ответ
	w := httptest.NewRecorder()

	// Вызываем обработчик
	PingHandler(w, req)

	// Проверяем код состояния
	assert.Equal(t, http.StatusOK, w.Code)

	// Проверяем Content-Type
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	// Проверяем тело ответа
	var response map[string]string
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "pong", response["message"])
}

func TestPingHandler_WithLogHook(t *testing.T) {
	// Устанавливаем LogHook для захвата логов
	var logOutput strings.Builder
	otel.LogHook = func(format string, args ...any) {
		logOutput.WriteString(format)
	}
	defer func() {
		otel.LogHook = nil
	}()

	// Создаем новый HTTP-запрос
	req, err := http.NewRequest("GET", "/ping", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Создаем ответ
	w := httptest.NewRecorder()

	// Вызываем обработчик
	PingHandler(w, req)

	// Проверяем код состояния
	assert.Equal(t, http.StatusOK, w.Code)

	// Проверяем, что LogHook был вызван
	assert.True(t, strings.Contains(logOutput.String(), "traceID:"), "LogHook должен содержать traceID")
	assert.True(t, strings.Contains(logOutput.String(), "spanID:"), "LogHook должен содержать spanID")
	assert.True(t, strings.Contains(logOutput.String(), "isSampled:"), "LogHook должен содержать isSampled")
}

func TestPingHandler_WithoutLogHook(t *testing.T) {
	// Убеждаемся, что LogHook не установлен
	otel.LogHook = nil

	// Создаем новый HTTP-запрос
	req, err := http.NewRequest("GET", "/ping", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Создаем ответ
	w := httptest.NewRecorder()

	// Вызываем обработчик - не должно быть паники
	assert.NotPanics(t, func() {
		PingHandler(w, req)
	})

	// Проверяем код состояния
	assert.Equal(t, http.StatusOK, w.Code)
}

// failingResponseWriter is a mock ResponseWriter that fails on Write
type failingResponseWriter struct {
	httptest.ResponseRecorder
}

func (f *failingResponseWriter) Write(_ []byte) (int, error) {
	return 0, assert.AnError
}

func (f *failingResponseWriter) WriteHeader(_ int) {
	// Do nothing
}

func TestHealth_EncodeError(t *testing.T) {
	req, err := http.NewRequest("GET", "/healthz", nil)
	require.NoError(t, err)

	w := &failingResponseWriter{}

	// Should not panic
	assert.NotPanics(t, func() {
		Health(w, req)
	})
}

func TestPingHandler_EncodeError(t *testing.T) {
	req, err := http.NewRequest("GET", "/ping", nil)
	require.NoError(t, err)

	w := &failingResponseWriter{}

	// Should not panic
	assert.NotPanics(t, func() {
		PingHandler(w, req)
	})
}
