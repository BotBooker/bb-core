package server_api

import (
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/botbooker/bb-core/internal/logger"
	"github.com/botbooker/bb-core/internal/observability/otel"
	router_api "github.com/botbooker/bb-core/internal/router/api"
	"github.com/botbooker/bb-core/internal/server"
)

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
	cfg := server.GetServerConfig()
	logger.InitAndSetDefault(cfg.LogLevel)

	handler := router_api.SetupRouter()
	// Override application name from environment if set
	serviceName := otel.GetApplicationName("botbooker-api")
	srv := server.NewServer(handler, serviceName)

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

	slog.Info("shutting down server...")

	return server.ShutdownServer(srv)
}
