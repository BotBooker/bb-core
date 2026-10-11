package serverapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/botbooker/bb-core/internal/closer"
	"github.com/botbooker/bb-core/internal/config"
	"github.com/botbooker/bb-core/pkg/logger"
)

// ServerAPI — структура приложения.
// Содержит DI-контейнер и HTTP-сервер.
type ServerAPI struct {
	diContainer *diContainer
	httpServer  *http.Server
}

// New создаёт приложение.
// Зависимости инициализируются в Run(): так ошибка подключения к БД
// возвращается вызывающему коду, а не завершает процесс из библиотеки.
func New() *ServerAPI {
	logger.InitAndSetDefault(config.AppConfig().LogLevel, logger.LogFormat(config.AppConfig().LogFormat))

	return &ServerAPI{}
}

// initDeps последовательно вызывает функции инициализации.
func (a *ServerAPI) initDeps(ctx context.Context) error {
	inits := []func(context.Context) error{
		a.initDIContainer,
		a.initHTTPServer,
	}

	for _, fn := range inits {
		if err := fn(ctx); err != nil {
			return err
		}
	}

	return nil
}

// initDIContainer создаёт DI-контейнер: подключает БД и собирает граф зависимостей.
func (a *ServerAPI) initDIContainer(ctx context.Context) error {
	container, err := newDIContainer(ctx)
	if err != nil {
		return fmt.Errorf("init di container: %w", err)
	}

	a.diContainer = container

	return nil
}

// initHTTPServer создаёт HTTP-сервер.
func (a *ServerAPI) initHTTPServer(_ context.Context) error {
	a.httpServer = &http.Server{
		Addr:              config.AppConfig().HTTPAddr,
		Handler:           a.diContainer.Handler().Routes(),
		ReadHeaderTimeout: 3 * time.Second,
	}

	return nil
}

// Run запускает HTTP-сервер с graceful shutdown.
//
// Что происходит:
//  0. initDeps инициализирует зависимости (в том числе подключение к БД)
//  1. signal.NotifyContext перехватывает SIGINT (Ctrl+C) и SIGTERM (Kubernetes)
//  2. HTTP-сервер запускается в отдельной горутине
//  3. Main-горутина ждёт сигнал через <-ctx.Done()
//  4. При сигнале: server.Shutdown дожидается текущих запросов
//  5. closer.CloseAll закрывает все ресурсы в обратном порядке (LIFO)
//
// Паттерн "двойной Ctrl+C":
//   - Первый Ctrl+C → graceful shutdown
//   - stop() снимает custom handler после первого сигнала
//   - Второй Ctrl+C → ОС убивает процесс мгновенно (для разработки, когда shutdown завис)
//
// Результат err не именованный: отложенная остановка OpenTelemetry логируется,
// как и остальные ошибки на этапе shutdown, чтобы отсутствие коллектора
// не превращало корректное завершение в код возврата 1.
func (a *ServerAPI) Run() error {
	// 1. Перехват сигналов.
	// signal.NotifyContext создаёт канал с ёмкостью 1 (буферизованный).
	// Если бы канал был unbuffered — сигнал мог бы потеряться,
	// пока main ещё инициализирует зависимости и не слушает канал.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Set up OpenTelemetry.
	otelShutdown, err := setupOTelSDK(ctx)
	if err != nil {
		return err
	}
	// Handle shutdown properly so nothing leaks.
	defer func() {
		if shutdownErr := otelShutdown(context.Background()); shutdownErr != nil {
			slog.Error("failed to shutdown opentelemetry", "err", shutdownErr)
		}
	}()

	// 2. Инициализация зависимостей: БД, кэш, репозитории, сервисы, роутинг.
	// Ошибка подключения к БД возвращается наверх — процесс завершится с кодом 1.
	if err := a.initDeps(ctx); err != nil {
		return err
	}

	slog.Info("the server is running", "addr", config.AppConfig().HTTPAddr)

	// 3. Запуск сервера в горутине.
	// ListenAndServe блокирует — поэтому запускаем в горутине, а main ждёт сигнал.
	// http.ErrServerClosed — нормальное завершение (мы сами вызвали Shutdown), не ошибка.
	go func() {
		if err := a.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "err", err)
		}
	}()

	// 4. Ожидание сигнала.
	<-ctx.Done()
	slog.Info("signal received, shutting down...")

	// Паттерн "двойной Ctrl+C": снимаем custom handler.
	// Теперь второй Ctrl+C убьёт процесс мгновенно (дефолтное поведение ОС).
	stop()

	// 5. Graceful shutdown HTTP-сервера.
	// Таймаут 15 секунд. Используем context.Background(), а не ctx — тот уже отменён.
	//
	// Что делает server.Shutdown внутри:
	//   1) Закрывает listeners — новые TCP-соединения невозможны
	//   2) Закрывает idle connections (keep-alive без активных запросов)
	//   3) Ждёт активные connections — пока handler вернёт ответ
	//   4) Если контекст истёк — возвращает ошибку, НО handlers продолжают работать в фоне
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	if err := a.httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("error while stopping the server", "err", err)
	}

	slog.Info("server stopped")

	// 6. Закрытие всех ресурсов через глобальный closer (LIFO).
	// Отдельный контекст с таймаутом 10 секунд — свой бюджет для ресурсов.
	// Суммарно: 15с (сервер) + 10с (ресурсы) = 25с из 30с Kubernetes grace period.
	closerCtx, closerCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer closerCancel()

	if err := closer.CloseAll(closerCtx); err != nil {
		slog.Error("failed to close resources", "err", err)
	}

	return nil
}
