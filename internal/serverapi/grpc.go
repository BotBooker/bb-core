package serverapi

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	health_v1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/botbooker/bb-core/internal/config"
	"github.com/botbooker/bb-core/pkg/observability"
	user_v1 "github.com/botbooker/bb-core/pkg/proto/user/v1"
)

// listenGRPC открывает сетевой слушатель для gRPC.
//
// Переменная, а не прямой вызов net.Listen: unit-тесты подменяют её слушателем
// на свободном порту, чтобы не занимать порт из конфигурации.
var listenGRPC = net.Listen

// grpcStopTimeout — сколько ждём завершения текущих gRPC-вызовов
// при graceful shutdown, прежде чем оборвать их принудительно.
const grpcStopTimeout = 5 * time.Second

// initGRPCServer создаёт gRPC-сервер и открывает слушатель.
//
// Слушатель открывается на этапе инициализации: занятый порт должен валить
// старт приложения, а не выясняться после успешного запуска HTTP-сервера.
func (a *ServerAPI) initGRPCServer(ctx context.Context) error {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "initGRPCServer")
	defer spanEnd()

	listener, err := listenGRPC("tcp", config.AppConfig().GRPCAddr)
	if err != nil {
		return fmt.Errorf("listen grpc: %w", err)
	}

	a.grpcListener = listener
	a.grpcServer = newGRPCServer(a.diContainer.UserServiceServer(traceCtx), a.diContainer.AuthService(traceCtx))

	return nil
}

// newGRPCServer создаёт gRPC-сервер со всеми сервисами приложения.
//
// health и reflection включаются всегда: первый нужен балансировщикам и
// readiness-пробам, второй — grpcurl при отладке (в том числе в smoke-тестах).
// Оба метода не защищаются токеном (см. isPublicGRPCMethod).
//
// authService нужен интерцептору: он проверяет токен вызова до обработчика,
// поэтому ни одна ручка сервиса пользователей не остаётся открытой.
//
// Интерцепторы объединяются в цепочку, а не устанавливаются по одному: порядок
// значим. Access-лог (см. NewLogInterceptor) стоит первым — снаружи проверки
// токена, поэтому в лог попадают и вызовы, отклонённые без токена.
func newGRPCServer(userServiceServer user_v1.UserServiceServer, authService AuthService) *grpc.Server {
	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(NewLogInterceptor(), NewAuthInterceptor(authService)),
	)

	user_v1.RegisterUserServiceServer(server, userServiceServer)
	registerHealth(server)
	reflection.Register(server)

	return server
}

// registerHealth регистрирует стандартный health-сервис: и сам сервер,
// и каждый зарегистрированный сервис отвечают SERVING.
func registerHealth(server grpc.ServiceRegistrar) {
	healthServer := health.NewServer()

	healthServer.SetServingStatus("", health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus(
		user_v1.UserService_ServiceDesc.ServiceName,
		health_v1.HealthCheckResponse_SERVING,
	)

	health_v1.RegisterHealthServer(server, healthServer)
}

// stopGRPC останавливает gRPC-сервер.
//
// GracefulStop ждёт завершения текущих вызовов, поэтому ожидание ограничено:
// иначе выключение «зависнет» на клиенте, который держит долгий запрос.
func (a *ServerAPI) stopGRPC() {
	stopped := make(chan struct{})

	go func() {
		a.grpcServer.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(grpcStopTimeout):
		slog.Warn("grpc graceful stop timed out, forcing stop")
		a.grpcServer.Stop()
	}
}
