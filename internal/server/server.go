package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// NewServer создаёт и настраивает HTTP-сервер.
//
// Параметры:
//   - cfg: конфигурация сервера;
//   - handler: HTTP handler для обработки запросов;
//   - serviceName: имя сервиса для логирования.
//
// Возвращает:
//   - *http.Server: настроенный экземпляр HTTP-сервера.
func NewServer(handler http.Handler, serviceName string) *http.Server {
	cfg := GetServerConfig()
	addr := fmt.Sprintf("%s:%s", cfg.Host, cfg.Port)

	slog.Info("starting BotBooker API server",
		"host", cfg.Host,
		"port", cfg.Port,
		"service", serviceName,
	)

	return &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

// ShutdownServer выполняет graceful shutdown сервера.
//
// Параметры:
//   - srv: HTTP-сервер для остановки.
//
// Возвращает:
//   - error: ошибка при остановке сервера (nil при успешном завершении).
func ShutdownServer(srv *http.Server) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
		return err
	}

	slog.Info("server exited properly")
	return nil
}
