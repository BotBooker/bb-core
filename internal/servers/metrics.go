package servers

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/botbooker/bb-core/pkg/observability"
)

// Имена атрибутов метрик — из семантических конвенций OpenTelemetry, чтобы
// метрики без доработок читались стандартными дашбордами.
const (
	attrHTTPRequestMethod = "http.request.method"
	attrHTTPRoute         = "http.route"
	attrHTTPStatusCode    = "http.response.status_code"

	attrRPCSystem     = "rpc.system"
	attrRPCMethod     = "rpc.method"
	attrRPCStatusCode = "rpc.grpc.status_code"

	rpcSystemGRPC = "grpc"
)

// gatewayRoutePrefix — префикс, под которым верхний mux отдаёт все ручки
// grpc-gateway одним шаблоном; уточнить маршрут по нему нельзя.
const gatewayRoutePrefix = "/api/v1/"

// unmatchedRoute — метка для запросов, не совпавших ни с одной ручкой: сырой
// путь произвольного 404 не должен попадать в метрику (взрыв кардинальности).
const unmatchedRoute = "unmatched"

// uuidSegmentPattern — сегмент пути, являющийся UUID: такие идентификаторы
// схлопываются в {uuid}.
var uuidSegmentPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Инструменты метрик создаются лениво: otel.Meter() берёт глобальный провайдер
// метрик в момент вызова, а его выставляет SetupOTelSDK — до старта серверов.
// Инструменты, созданные раньше, навсегда остались бы привязаны к no-op
// провайдеру и не экспортировались бы.
var (
	metricsOnce       sync.Once
	httpServerLatency metric.Float64Histogram
	grpcServerLatency metric.Float64Histogram
)

// metricsMiddleware записывает длительность HTTP-запроса в гистограмму
// http.server.request.duration.
//
// Middleware ставится внутри authenticate — непосредственно вокруг mux, — чтобы
// после обслуживания запроса был доступен Request.Pattern, который net/http
// проставляет на тот же объект запроса. Маршрут нормализуется (routeLabel):
// идентификаторы в пути схлопываются, иначе число серий меток росло бы с числом
// сущностей.
func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		interceptor := &responseWriterInterceptor{ResponseWriter: w}
		next.ServeHTTP(interceptor, r)

		if interceptor.statusCode == 0 {
			interceptor.statusCode = http.StatusOK
		}

		recordHTTPDuration(r, interceptor.statusCode, time.Since(start))
	})
}

// NewMetricsInterceptor создаёт унарный интерцептор gRPC: он записывает
// длительность вызова в гистограмму rpc.server.duration.
//
// Интерцептор ставится после access-лога и снаружи проверки токена, поэтому в
// измерение попадает и время аутентификации. Метка rpc.method — info.FullMethod
// из реестра сервисов: значение доверенное (см. решения ревью, П.5 A2),
// санитизация не требуется.
func NewMetricsInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		start := time.Now()

		resp, err := handler(ctx, req)

		recordGRPCDuration(ctx, info.FullMethod, status.Code(err), time.Since(start))

		return resp, err
	}
}

// recordHTTPDuration записывает длительность HTTP-запроса и его результат.
func recordHTTPDuration(r *http.Request, statusCode int, duration time.Duration) {
	metricsOnce.Do(initMetrics)

	if httpServerLatency == nil {
		return
	}

	httpServerLatency.Record(r.Context(), duration.Seconds(), metric.WithAttributes(
		attribute.String(attrHTTPRequestMethod, r.Method),
		attribute.String(attrHTTPRoute, routeLabel(r)),
		attribute.Int(attrHTTPStatusCode, statusCode),
	))
}

// recordGRPCDuration записывает длительность gRPC-вызова и код ответа.
func recordGRPCDuration(ctx context.Context, fullMethod string, code codes.Code, duration time.Duration) {
	metricsOnce.Do(initMetrics)

	if grpcServerLatency == nil {
		return
	}

	grpcServerLatency.Record(ctx, duration.Seconds(), metric.WithAttributes(
		attribute.String(attrRPCSystem, rpcSystemGRPC),
		attribute.String(attrRPCMethod, fullMethod),
		attribute.Int(attrRPCStatusCode, int(code)),
	))
}

// initMetrics создаёт инструменты метрик приложения.
//
// Ошибка возможна только при конфликте определения инструмента (то же имя с
// другой единицей измерения) — это ошибка сборки приложения, а не запроса.
// Поэтому инструмент остаётся nil, а его запись пропускается: обработка запроса
// из-за диагностики не падает.
func initMetrics() {
	meter := observability.Meter()

	httpServerLatency, _ = meter.Float64Histogram(
		"http.server.request.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Длительность обработки HTTP-запроса"),
	)

	grpcServerLatency, _ = meter.Float64Histogram(
		"rpc.server.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Длительность обработки gRPC-вызова"),
	)
}

// routeLabel возвращает нормализованный маршрут запроса для метрик: шаблон,
// который net/http.ServeMux проставил в Request.Pattern, а для gateway-ручек —
// нормализованный путь. Запрос, не совпавший ни с одной ручкой, попадает в
// единственную метку unmatchedRoute.
func routeLabel(r *http.Request) string {
	switch pattern := normalizePattern(r.Pattern); pattern {
	case "":
		return unmatchedRoute
	case gatewayRoutePrefix:
		return normalizePath(r.URL.Path)
	default:
		return pattern
	}
}

// normalizePattern убирает из шаблона метод, если он там указан: net/http
// хранит в Request.Pattern строку вида "GET /users/me", а метке нужен маршрут.
func normalizePattern(pattern string) string {
	if _, route, ok := strings.Cut(pattern, " "); ok {
		return route
	}

	return pattern
}

// normalizePath заменяет идентификаторы в пути на {uuid}: имя спана и метка
// маршрута не должны зависеть от конкретных сущностей, иначе кардинальность
// растёт с числом пользователей. Используется и в имени серверного спана
// (см. traceMiddleware), где Request.Pattern ещё не проставлен.
func normalizePath(path string) string {
	if path == "" || path == "/" {
		return path
	}

	segments := strings.Split(path, "/")
	for i, segment := range segments {
		if uuidSegmentPattern.MatchString(segment) {
			segments[i] = "{uuid}"
		}
	}

	return strings.Join(segments, "/")
}
