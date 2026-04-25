package serverapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/botbooker/bb-core/internal/closer"
	"github.com/botbooker/bb-core/internal/config"
)

// ServerAPI — структура приложения.
// Содержит DI-контейнер и HTTP-сервер.
type ServerAPI struct {
	diContainer *diContainer
	httpServer  *http.Server
}

// New создаёт приложение и инициализирует все зависимости через DI-контейнер.
func New() *ServerAPI {
	a := &ServerAPI{
		diContainer: newDIContainer(),
	}
	a.initDeps()
	return a
}

// initDeps последовательно вызывает функции инициализации.
func (a *ServerAPI) initDeps() {
	inits := []func(){
		a.initHTTPServer,
	}

	for _, fn := range inits {
		fn()
	}
}

// initHTTPServer создаёт HTTP-сервер.
func (a *ServerAPI) initHTTPServer() {
	a.httpServer = &http.Server{
		Addr:              config.AppConfig().HTTPAddr,
		Handler:           a.diContainer.Handler().Routes(),
		ReadHeaderTimeout: 3 * time.Second,
	}
}

// Run запускает HTTP-сервер с graceful shutdown.
//
// Что происходит:
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
		err = errors.Join(err, otelShutdown(context.Background()))
	}()

	slog.Info("сервер запущен", "addr", config.AppConfig().HTTPAddr)

	// 2. Запуск сервера в горутине.
	// ListenAndServe блокирует — поэтому запускаем в горутине, а main ждёт сигнал.
	// http.ErrServerClosed — нормальное завершение (мы сами вызвали Shutdown), не ошибка.
	go func() {
		if err := a.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("ошибка сервера", "err", err)
		}
	}()

	// 3. Ожидание сигнала.
	<-ctx.Done()
	slog.Info("получен сигнал, завершаем...")

	// Паттерн "двойной Ctrl+C": снимаем custom handler.
	// Теперь второй Ctrl+C убьёт процесс мгновенно (дефолтное поведение ОС).
	stop()

	// 4. Graceful shutdown HTTP-сервера.
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
		slog.Error("ошибка при остановке сервера", "err", err)
	}

	slog.Info("сервер остановлен")

	// 5. Закрытие всех ресурсов через глобальный closer (LIFO).
	// Отдельный контекст с таймаутом 10 секунд — свой бюджет для ресурсов.
	// Суммарно: 15с (сервер) + 10с (ресурсы) = 25с из 30с Kubernetes grace period.
	closerCtx, closerCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer closerCancel()

	if err := closer.CloseAll(closerCtx); err != nil {
		slog.Error("ошибки при закрытии ресурсов", "err", err)
	}

	return nil
}
