package servers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// healthPath — ручка liveness: процесс жив и принимает соединения. Ничего не
// проверяет, поэтому не может «уронить» под из-за внешней зависимости.
const healthPath = "/health"

// readyPath — ручка readiness: приложение готово обслуживать запросы, то есть
// база данных отвечает. Kubernetes снимает под с балансировщика по 503.
const readyPath = "/ready"

// readinessTimeout — бюджет проверки БД: проба не должна висеть дольше своего
// timeoutSeconds (в Helm — 2 с).
const readinessTimeout = 2 * time.Second

// readinessChecker — минимум, который нужен пробе готовности.
//
// В DI передаётся database.DB: presentation-слою не нужен весь контракт
// подключения, достаточно PingContext.
type readinessChecker interface {
	PingContext(ctx context.Context) error
}

// healthHandler — GET /health: liveness-проба.
//
// Отвечает 200, пока жив процесс, даже если БД недоступна: перезапуск контейнера
// не лечит упавшую базу, а readiness-проба уже вывела под из балансировки.
func (h *handler) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)

	if _, err := fmt.Fprintln(w, "ok"); err != nil {
		slog.ErrorContext(r.Context(), "failed to write response", "err", err)
	}
}

// readyHandler — GET /ready: readiness-проба с проверкой БД.
//
// Ошибка ping — 503: под не получает трафик, пока база не восстановится, и
// восстанавливается без перезапуска. Текст фиксированный: детали подключения
// (адрес, DSN) наружу не отдаются.
func (h *handler) readyHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
	defer cancel()

	if err := h.readiness.PingContext(ctx); err != nil {
		slog.WarnContext(r.Context(), "readiness check failed", "err", err)
		writeError(r.Context(), w, http.StatusServiceUnavailable, "not ready")

		return
	}

	w.WriteHeader(http.StatusOK)

	if _, err := fmt.Fprintln(w, "ready"); err != nil {
		slog.ErrorContext(r.Context(), "failed to write response", "err", err)
	}
}
