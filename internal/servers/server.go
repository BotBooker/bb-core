package servers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"github.com/botbooker/bb-core/internal/closer"
	"github.com/botbooker/bb-core/internal/config"
	"github.com/botbooker/bb-core/pkg/logger"
	"github.com/botbooker/bb-core/pkg/observability"
)

// ServerAPI — структура приложения.
// Содержит DI-контейнер, HTTP-сервер и gRPC-сервер.
type ServerAPI struct {
	diContainer *diContainer
	httpServer  *http.Server
	grpcServer  *grpc.Server
	// grpcListener открывается в initGRPCServer и закрывается вместе с сервером.
	grpcListener net.Listener
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
		a.initGRPCServer,
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
	traceCtx, spanEnd := observability.TraceSpan(ctx, "initDIContainer")
	defer spanEnd()

	container, err := newDIContainer(traceCtx)
	if err != nil {
		return fmt.Errorf("init di container: %w", err)
	}

	a.diContainer = container

	return nil
}

// initHTTPServer создаёт HTTP-сервер.
func (a *ServerAPI) initHTTPServer(ctx context.Context) error {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "initHTTPServer")
	defer spanEnd()

	handler, err := a.diContainer.Handler(traceCtx)
	if err != nil {
		return fmt.Errorf("init http handler: %w", err)
	}

	a.httpServer = &http.Server{
		Addr:              config.AppConfig().HTTPAddr,
		Handler:           handler.Routes(),
		ReadHeaderTimeout: 3 * time.Second,
	}

	return nil
}

// Run запускает HTTP-сервер с graceful shutdown.
//
// Что происходит:
//  0. initDeps инициализирует зависимости (в том числе подключение к БД)
//  1. signal.NotifyContext перехватывает SIGINT (Ctrl+C) и SIGTERM (Kubernetes)
//  2. HTTP-сервер и gRPC-сервер запускаются каждый в своей горутине
//  3. Main-горутина ждёт сигнал через <-ctx.Done()
//  4. При сигнале: server.Shutdown и GracefulStop дожидаются текущих запросов
//  5. closer.CloseAll закрывает все ресурсы в обратном порядке (LIFO)
//
// Паттерн "двойной Ctrl+C":
//   - Первый Ctrl+C → graceful shutdown
//   - stop() снимает custom handler после первого сигнала
//   - Второй Ctrl+C → ОС убивает процесс мгновенно (для разработки, когда shutdown завис)
//
// Результат err не именованный: отложенная остановка OpenTelemetry логируется,
// как и остальные ошибки на этапе shutdown, чтобы отсутствие коллектора
// не превращало корректное завершение в код возврата 1. На выгрузку телеметрии
// отводится отдельный бюджет по времени: OTLP-экспортёры повторяют попытки
// до минуты, и недоступный коллектор не должен задерживать выход.
func (a *ServerAPI) Run() error {
	// 1. Перехват сигналов.
	// signal.NotifyContext создаёт канал с ёмкостью 1 (буферизованный).
	// Если бы канал был unbuffered — сигнал мог бы потеряться,
	// пока main ещё инициализирует зависимости и не слушает канал.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Set up OpenTelemetry.
	otelShutdown, err := observability.SetupOTelSDK(ctx)
	if err != nil {
		return err
	}
	// Handle shutdown properly so nothing leaks.
	defer func() {
		// Бюджет на выгрузку телеметрии: у OTLP-экспортёров retry может ждать
		// коллектор до минуты, а процесс не должен висеть при завершении, если
		// коллектор недоступен.
		otelCtx, otelCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer otelCancel()

		if shutdownErr := otelShutdown(otelCtx); shutdownErr != nil {
			slog.Error("failed to shutdown opentelemetry", "err", shutdownErr)
		}
	}()

	// 2. Инициализация зависимостей: БД, кэш, репозитории, сервисы, роутинг.
	// Ошибка подключения к БД возвращается наверх — процесс завершится с кодом 1.
	//
	// Сценарий 3: стартовый спан открывается до инициализации и закрывается
	// (ниже) строго тогда, когда все компоненты готовы принимать трафик.
	startupCtx, startupSpan := startStartupSpan(ctx)

	if err := a.initDeps(startupCtx); err != nil {
		startupSpan.End()

		return err
	}

	slog.Info("the server is running",
		"http_addr", config.AppConfig().HTTPAddr,
		"grpc_addr", config.AppConfig().GRPCAddr,
	)

	// 3. Запуск серверов в горутинах.
	// ListenAndServe и Serve блокируют — поэтому запускаем в горутинах,
	// а main ждёт сигнал.
	// http.ErrServerClosed и grpc.ErrServerStopped — нормальное завершение
	// (остановку запросили мы сами), а не ошибка.
	go func() {
		if err := a.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server error", "err", err)
		}
	}()

	go func() {
		if err := a.grpcServer.Serve(a.grpcListener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			slog.Error("grpc server error", "err", err)
		}
	}()

	// Все компоненты готовы принимать трафик — закрываем стартовый спан.
	startupSpan.End()

	// 4. Ожидание сигнала.
	<-ctx.Done()
	slog.Info("signal received, shutting down...")

	// Сценарий 4: трасса graceful shutdown. Перехвачен SIGINT/SIGTERM — начинаем
	// новую трассу с собственным TraceID и закрываем её перед выходом процесса,
	// когда серверы остановлены, а ресурсы закрыты.
	shutdownTraceCtx, shutdownSpan := startShutdownSpan(context.Background())
	defer shutdownSpan.End()

	// Паттерн "двойной Ctrl+C": снимаем custom handler.
	// Теперь второй Ctrl+C убьёт процесс мгновенно (дефолтное поведение ОС).
	stop()

	// 5. Graceful shutdown HTTP-сервера.
	// Таймаут 15 секунд. Родитель — контекст трассы shutdown (ctx уже отменён).
	//
	// Что делает server.Shutdown внутри:
	//   1) Закрывает listeners — новые TCP-соединения невозможны
	//   2) Закрывает idle connections (keep-alive без активных запросов)
	//   3) Ждёт активные connections — пока handler вернёт ответ
	//   4) Если контекст истёк — возвращает ошибку, НО handlers продолжают работать в фоне
	shutdownCtx, shutdownCancel := context.WithTimeout(shutdownTraceCtx, 15*time.Second)
	defer shutdownCancel()

	if err := a.httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("error while stopping the server", "err", err)
	}

	slog.Info("http server stopped")

	// 6. Graceful shutdown gRPC-сервера: ждём завершения текущих вызовов
	// (с ограничением по времени, см. stopGRPC).
	a.stopGRPC()

	slog.Info("grpc server stopped")

	// 7. Закрытие всех ресурсов через глобальный closer (LIFO).
	// Отдельный контекст с таймаутом 10 секунд — свой бюджет для ресурсов.
	// Суммарно: 15с (HTTP) + 5с (gRPC) + 10с (ресурсы) = 30с Kubernetes grace period.
	closerCtx, closerCancel := context.WithTimeout(shutdownTraceCtx, 10*time.Second)
	defer closerCancel()

	if err := closer.CloseAll(closerCtx); err != nil {
		slog.Error("failed to close resources", "err", err)
	}

	return nil
}
