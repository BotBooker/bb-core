// Package main реализует HTTP‑сервер API для сервиса бронирования BotBooker.
package main

import (
	"log/slog"
	"os"

	"github.com/botbooker/bb-core/internal/config"
	"github.com/botbooker/bb-core/internal/logger"
	"github.com/botbooker/bb-core/internal/serverapi"
)

func main() {
	logger.InitJSONAndSetDefault(config.AppConfig().LogLevel)
	server := serverapi.New()
	// Run server with graceful shutdown
	if err := server.Run(); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}
