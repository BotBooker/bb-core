// Package health предоставляет обработчики для проверок работоспособности сервиса.
//
// Включает:
//   - Health: эндпоинт /health для проверки статуса сервиса.
//   - PingHandler: эндпоинт /ping для проверки работоспособности с трассировкой.
package health

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/botbooker/bb-core/internal/observability/otel"
)

// Health — обработчик HTTP‑запроса для проверки здоровья сервиса.
//
// Возвращает JSON‑ответ с статусом 200 OK и сообщением "OK".
// Используется для:
//   - мониторинга доступности сервиса (например, в Kubernetes liveness probe);
//   - базовых проверок работоспособности API.
//
// Параметры:
//   - w http.ResponseWriter: ответ HTTP;
//   - r *http.Request: запрос HTTP.
func Health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"message": "OK"}); err != nil {
		slog.Error("failed to encode health response", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// PingHandler — обработчик для проверки работоспособности с логированием трассировки.
//
// Возвращает JSON-ответ с статусом 200 OK и сообщением "pong".
// Логирует информацию о трассировке (traceID, spanID, isSampled).
//
// Параметры:
//   - w http.ResponseWriter: ответ HTTP;
//   - r *http.Request: запрос HTTP.
func PingHandler(w http.ResponseWriter, r *http.Request) {
	traceID, spanID, isSampled := otel.GetTraceInfo(r.Context())

	// Log via LogHook if set (for testing), otherwise use slog
	if otel.LogHook != nil {
		otel.LogHook("traceID: %v; spanID: %v; isSampled: %v", traceID, spanID, isSampled)
	} else {
		slog.DebugContext(r.Context(), "ping request",
			"traceID", traceID,
			"spanID", spanID,
			"isSampled", isSampled,
		)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"message": "pong"}); err != nil {
		slog.Error("failed to encode ping response", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}
