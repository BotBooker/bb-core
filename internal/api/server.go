package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

// UserService — интерфейс сервиса пользователей для хендлера.
type UserService interface {
	GetProfile(token string) string
}

// AuthService — интерфейс сервиса авторизации для хендлера.
type AuthService any

// методы, которые хендлер использует из сервиса авторизации

// NotificationService — интерфейс сервиса уведомлений для хендлера.
type NotificationService any

// методы, которые хендлер использует из сервиса уведомлений

// Handler — интерфейс обработчика HTTP-запросов.
type Handler interface {
	Routes() http.Handler
}

// handler — конкретная реализация, скрыта от внешних пакетов.
// Содержит только хендлеры и роутинг.
// Ничего не знает про http.Server, порт или lifecycle —
// это ответственность app-слоя.
type handler struct {
	userService         UserService
	authService         AuthService
	notificationService NotificationService
}

// NewHandler создаёт обработчик HTTP-запросов.
func NewHandler(
	userService UserService,
	authService AuthService,
	notificationService NotificationService,
) Handler {
	return &handler{
		userService:         userService,
		authService:         authService,
		notificationService: notificationService,
	}
}

// Routes возвращает маршрутизатор со всеми зарегистрированными хендлерами.
// App-слой использует этот http.Handler при создании http.Server.
func (h *handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.healthHandler)
	mux.HandleFunc("GET /users/me", h.getUserProfile)

	return mux
}

func (h *handler) healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)

	if _, err := fmt.Fprintln(w, "ok"); err != nil {
		slog.Error("ошибка записи ответа", "err", err)
	}
}

func (h *handler) getUserProfile(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("Authorization")
	profile := h.userService.GetProfile(token)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(profile); err != nil {
		slog.Error("ошибка записи ответа", "err", err)
	}
}
