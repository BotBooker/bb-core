package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/botbooker/bb-core/internal/logger"
	"github.com/botbooker/bb-core/internal/observability/otel"
	"github.com/botbooker/bb-core/internal/router"
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

// RunServer запускает HTTP-сервер с graceful shutdown.
//
// Параметры:
//   - cfg: конфигурация сервера;
//   - handler: HTTP handler для обработки запросов;
//   - serviceName: имя сервиса для логирования.
//
// Возвращает:
//   - error: ошибка при запуске сервера (nil при успешном завершении).
//
// Пример использования:
//
//	cfg := api.GetServerConfig()
//	router := router.SetupRouter(...)
//	if err := api.RunServer(cfg, router, "botbooker-api"); err != nil {
//	    log.Fatal(err)
//	}

func RunServer() error {
	// Initialize logger
	cfg := GetServerConfig()
	logger.InitAndSetDefault(cfg.LogLevel)

	handler := router.SetupRouter()
	// Override application name from environment if set
	serviceName := otel.GetApplicationName("botbooker-api")
	srv := NewServer(handler, serviceName)

	// Initialize OpenTelemetry tracer
	otel.InitTracer()

	// Start server in a goroutine
	go func() {
		slog.Info("HTTP server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down ..")

	return ShutdownServer(srv)
}
