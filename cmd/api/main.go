// Package main реализует HTTP‑сервер API для сервиса бронирования BotBooker.
package main

import (
	"log/slog"
	"os"

	"github.com/botbooker/bb-core/internal/serverapi"
)

// runner — минимальный интерфейс сервера, необходимый точке входа.
// Определён в пакете main (а не импортирован), чтобы фабрику сервера
// можно было подменить моком в unit-тестах.
type runner interface {
	Run() error
}

// newServerAPI — фабрика сервера. Это переменная, а не прямой вызов
// serverapi.New, чтобы тесты могли подставить фейковую реализацию
// без запуска реального сетевого сервера.
var newServerAPI = func() runner {
	return serverapi.New()
}

// osExit — обёртка над os.Exit. Это переменная, а не прямой вызов os.Exit,
// чтобы функцию main можно было вызвать из unit-тестов, не завершая процесс.
var osExit = os.Exit

func main() {
	if code := Main(); code != 0 {
		osExit(code)
	}
}

// Main содержит логику точки входа и возвращает код возврата процесса.
// Вынесена из main, чтобы её можно было тестировать напрямую, не завершая
// процесс тестирования вызовом os.Exit.
func Main() int {
	if err := run(); err != nil {
		slog.Error("server failed", "error", err)

		return 1
	}

	return 0
}

// run создаёт сервер и запускает его с graceful shutdown.
func run() error {
	server := newServerAPI()

	return server.Run()
}
