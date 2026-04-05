// Package main реализует HTTP‑сервер API для сервиса бронирования BotBooker.
package main

import (
	"log/slog"

	server_api "github.com/botbooker/bb-core/internal/server/api"
)

func main() {
	// Run server with graceful shutdown
	if err := server_api.RunServer(); err != nil {
		slog.Error("server failed", "error", err)
	}
}
