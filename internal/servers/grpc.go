package servers

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
	auth_v1 "github.com/botbooker/bb-core/pkg/proto/auth/v1"
	public_v1 "github.com/botbooker/bb-core/pkg/proto/public/v1"
	tenant_v1 "github.com/botbooker/bb-core/pkg/proto/tenant/v1"
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
	a.grpcServer = newGRPCServer(
		traceCtx,
		a.diContainer.UserServiceServer(traceCtx),
		a.diContainer.TenantServiceServer(traceCtx),
		a.diContainer.PublicCatalogServiceServer(traceCtx),
		a.diContainer.AuthService(traceCtx),
		a.diContainer.DB(),
	)

	return nil
}

// grpcHealthInterval — как часто health-сервис gRPC сверяет состояние с БД.
// Пробы kubelet опрашивают HTTP /ready чаще; здесь важно, чтобы
// долгоживущие gRPC-клиенты и балансировщики видели NOT_SERVING при падении
// базы.
const grpcHealthInterval = 10 * time.Second

// grpcHealthServices — имена health-сервисов: пустое (весь сервер) и каждый
// зарегистрированный proto-сервис.
func grpcHealthServices() []string {
	return []string{
		"",
		user_v1.UserService_ServiceDesc.ServiceName,
		auth_v1.AuthService_ServiceDesc.ServiceName,
		tenant_v1.TenantService_ServiceDesc.ServiceName,
		public_v1.PublicCatalogService_ServiceDesc.ServiceName,
	}
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
// readiness — проверка БД для health-сервиса: состояние SERVING/NOT_SERVING
// обновляется в фоне (см. watchHealth), а не выставляется один раз на старте.
//
// Интерцепторы объединяются в цепочку, а не устанавливаются по одному: порядок
// значим. Access-лог (см. NewLogInterceptor) стоит первым — снаружи проверки
// токена, поэтому в лог попадают и вызовы, отклонённые без токена. Метрики
// (см. NewMetricsInterceptor) идут следом и тоже охватывают аутентификацию.
func newGRPCServer(
	ctx context.Context,
	userServiceServer user_v1.UserServiceServer,
	tenantServiceServer tenant_v1.TenantServiceServer,
	publicCatalogServiceServer public_v1.PublicCatalogServiceServer,
	authService AuthService,
	readiness readinessChecker,
) *grpc.Server {
	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(NewLogInterceptor(), NewMetricsInterceptor(), NewAuthInterceptor(authService)),
	)

	user_v1.RegisterUserServiceServer(server, userServiceServer)
	tenant_v1.RegisterTenantServiceServer(server, tenantServiceServer)
	public_v1.RegisterPublicCatalogServiceServer(server, publicCatalogServiceServer)
	auth_v1.RegisterAuthServiceServer(server, NewAuthServiceServer(authService))
	go watchHealth(ctx, registerHealth(server), readiness)
	reflection.Register(server)

	return server
}

// registerHealth регистрирует стандартный health-сервис и возвращает его:
// состояние обновляет watchHealth по результату проверки БД.
func registerHealth(server grpc.ServiceRegistrar) *health.Server {
	healthServer := health.NewServer()

	for _, name := range grpcHealthServices() {
		healthServer.SetServingStatus(name, health_v1.HealthCheckResponse_SERVING)
	}

	health_v1.RegisterHealthServer(server, healthServer)

	return healthServer
}

// watchHealth держит состояние health-сервиса в соответствии с доступностью БД:
// пока ping проходит — SERVING, иначе NOT_SERVING.
//
// Без этого health всегда отвечал SERVING, и балансировщик отправлял трафик в
// под, все запросы которого к данным падали. Первая проверка выполняется сразу,
// дальше — раз в grpcHealthInterval; при остановке сервера горутина выходит по
// ctx.Done().
func watchHealth(ctx context.Context, healthServer *health.Server, readiness readinessChecker) {
	update := func() {
		pingCtx, cancel := context.WithTimeout(ctx, readinessTimeout)
		defer cancel()

		status := health_v1.HealthCheckResponse_SERVING
		if err := readiness.PingContext(pingCtx); err != nil {
			status = health_v1.HealthCheckResponse_NOT_SERVING
			slog.WarnContext(ctx, "grpc health check failed", "err", err)
		}

		for _, name := range grpcHealthServices() {
			healthServer.SetServingStatus(name, status)
		}
	}

	update()

	ticker := time.NewTicker(grpcHealthInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			update()
		}
	}
}

// stopGRPC останавливает gRPC-сервер.
//
// GracefulStop ждёт завершения текущих вызовов, поэтому ожидание ограничено:
// иначе выключение «зависнет» на клиенте, который держит долгий запрос.
//
// ctx нужен для логирования: trace_id активной трассы (трассы shutdown)
// добавляет обработчик логов (см. pkg/logger.TraceHandler).
func (a *ServerAPI) stopGRPC(ctx context.Context) {
	stopped := make(chan struct{})

	go func() {
		a.grpcServer.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(grpcStopTimeout):
		slog.WarnContext(ctx, "grpc graceful stop timed out, forcing stop")
		a.grpcServer.Stop()
	}
}
