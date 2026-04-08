// Package main реализует HTTP‑сервер API для сервиса бронирования BotBooker.
package main

import (
	"log/slog"

	"github.com/botbooker/bb-core/internal/server"
)

func main() {
	// Run server with graceful shutdown
	if err := server.RunServer(); err != nil {
		slog.Error("server failed", "error", err)
	}
}
