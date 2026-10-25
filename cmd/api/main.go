// Package main реализует точку входа сервиса бронирования BotBooker:
// HTTP-сервер (ручки приложения и grpc-gateway) и gRPC-сервер.
//
// Тот же бинарник обслуживает Docker HEALTHCHECK: с флагом --readycheck процесс
// не поднимает серверы и не подключается к БД, а опрашивает GET /ready уже
// работающего экземпляра (см. readycheck.go). Отдельная команда healthcheck не
// нужна: проба готовности — это одна HTTP-ручка, а не второе приложение.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/botbooker/bb-core/internal/servers"
)

// readycheckFlag — имя флага, включающего режим проверки готовности.
const readycheckFlag = "readycheck"

// exitCodeFailure — код возврата, когда приложение завершилось с ошибкой:
// сервер не запустился или проба готовности не прошла.
const exitCodeFailure = 1

// exitCodeUsage — код возврата при неверных аргументах командной строки;
// 2 — то же соглашение, что у пакета flag.
const exitCodeUsage = 2

// usage — справка по флагам: одновременно документация для человека и
// напоминание, какой аргумент передаёт Docker HEALTHCHECK.
const usage = "usage: api [--readycheck]\n\n" +
	"  --readycheck  проверить готовность работающего экземпляра (GET /ready) и выйти\n"

// runner — минимальный интерфейс сервера, необходимый точке входа.
// Определён в пакете main (а не импортирован), чтобы фабрику сервера
// можно было подменить моком в unit-тестах.
type runner interface {
	Run() error
}

// newServerAPI — фабрика сервера. Это переменная, а не прямой вызов
// servers.New, чтобы тесты могли подставить фейковую реализацию
// без запуска реального сетевого сервера.
var newServerAPI = func() runner {
	return servers.New()
}

// osExit — обёртка над os.Exit. Это переменная, а не прямой вызов os.Exit,
// чтобы функцию main можно было вызвать из unit-тестов, не завершая процесс.
var osExit = os.Exit

// args — аргументы командной строки без имени программы. Это переменная, а не
// прямой os.Args[1:] в Main, чтобы тесты передавали нужный набор флагов и не
// разбирали аргументы `go test` (-test.timeout и прочие).
var args = os.Args[1:]

func main() {
	if code := Main(); code != 0 {
		osExit(code)
	}
}

// Main содержит логику точки входа и возвращает код возврата процесса.
// Вынесена из main, чтобы её можно было тестировать напрямую, не завершая
// процесс тестирования вызовом os.Exit.
//
// Режимы:
//   - без флагов — обычный запуск сервера;
//   - --readycheck — разовая проба готовности работающего экземпляра:
//     0 — готов, 1 — не готов или недоступен, 2 — неверные аргументы.
func Main() int {
	readycheck, err := parseArgs(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_, _ = fmt.Fprint(os.Stdout, usage)

			return 0
		}

		_, _ = fmt.Fprintf(os.Stderr, "api: %v\n%s", err, usage)

		return exitCodeUsage
	}

	if readycheck {
		return readycheckExitCode()
	}

	if err := run(); err != nil {
		slog.ErrorContext(context.Background(), "server failed", "error", err)

		return exitCodeFailure
	}

	return 0
}

// parseArgs разбирает аргументы командной строки.
//
// Ошибка разбора возвращается как есть, вместе с flag.ErrHelp: справку и
// сообщение об ошибке печатает Main, поэтому вывод flag отключён и в тестах
// не засоряет stderr. Позиционные аргументы запрещены: единственный режим
// запуска задаётся флагом, а опечатку лучше увидеть, чем проигнорировать.
func parseArgs(argv []string) (bool, error) {
	fs := flag.NewFlagSet("api", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	readycheck := fs.Bool(readycheckFlag, false,
		"проверить готовность работающего экземпляра (GET /ready) и выйти")

	if err := fs.Parse(argv); err != nil {
		return false, err
	}

	if fs.NArg() > 0 {
		return false, errors.New("unexpected arguments: the command accepts flags only")
	}

	return *readycheck, nil
}

// run создаёт сервер и запускает его с graceful shutdown.
func run() error {
	server := newServerAPI()

	return server.Run()
}
