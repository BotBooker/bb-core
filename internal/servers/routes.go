package servers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"

	"github.com/botbooker/bb-core/internal/repository/swagger"
	auth_v1 "github.com/botbooker/bb-core/pkg/proto/auth/v1"
	user_v1 "github.com/botbooker/bb-core/pkg/proto/user/v1"
)

// SwaggerPrefix — префикс ручек Swagger UI и спецификации.
const SwaggerPrefix = "/api/swagger/"

// healthPath — ручка готовности приложения. Вынесена константой: её использует
// и роутинг, и список открытых путей (см. isPublicPath), и выбор уровня записи
// access-лога для служебных ручек (см. isServicePath).
const healthPath = "/health"

// Handler — интерфейс обработчика HTTP-запросов.
type Handler interface {
	Routes() http.Handler
}

// handler — конкретная реализация, скрыта от внешних пакетов.
// Содержит только хендлеры и роутинг.
// Ничего не знает про http.Server, порт или lifecycle —
// это ответственность app-слоя.
type handler struct {
	authService AuthService
	// gateway — HTTP-транспорт proto-сервисов: те же ручки, что и в gRPC.
	gateway *runtime.ServeMux
}

// NewHandler создаёт обработчик HTTP-запросов.
//
// userServiceServer — адаптер proto-сервиса. grpc-gateway вызывает его
// напрямую, в процессе приложения: HTTP-ручки работают, не требуя сетевого
// обращения к gRPC-порту (и падения одного порта не ломают другой).
//
// Ошибка возможна только при конфликте маршрутов gateway — это ошибка сборки
// приложения, поэтому возвращается наверх, а не игнорируется.
func NewHandler(authService AuthService, userServiceServer user_v1.UserServiceServer) (Handler, error) {
	gateway := runtime.NewServeMux()

	if err := user_v1.RegisterUserServiceHandlerServer(context.Background(), gateway, userServiceServer); err != nil {
		return nil, fmt.Errorf("register user service gateway: %w", err)
	}

	if err := auth_v1.RegisterAuthServiceHandlerServer(context.Background(), gateway, NewAuthServiceServer(authService)); err != nil {
		return nil, fmt.Errorf("register auth service gateway: %w", err)
	}

	return &handler{authService: authService, gateway: gateway}, nil
}

// Routes возвращает маршрутизатор со всеми зарегистрированными хендлерами.
// App-слой использует этот http.Handler при создании http.Server.
//
// Все ручки, кроме перечисленных в publicPaths, проходят через middleware
// authenticate: аутентификация не «размазана» по хендлерам, поэтому забыть
// проверить токен в новой ручке нельзя.
//
// Снаружи стоит traceMiddleware: он извлекает W3C Trace Context из заголовков
// запроса и запускает серверный спан — трасса запроса начинается до
// аутентификации и покрывает весь путь запроса.
//
// logMiddleware стоит между трассировкой и аутентификацией: запись access-лога
// попадает в контекст трассы (trace_id добавляет TraceHandler) и видит
// статус отказа 401/403, который выставляет authenticate.
//
// metricsMiddleware стоит внутри authenticate — непосредственно вокруг mux, —
// чтобы после обслуживания запроса был доступен Request.Pattern и метка
// маршрута не зависела от конкретных идентификаторов (см. metricsMiddleware).
func (h *handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+healthPath, h.healthHandler)
	mux.HandleFunc("GET /users/me", h.getCurrentUser)
	mux.HandleFunc("POST /auth/register", h.registerHandler)
	mux.HandleFunc("POST /auth/login", h.loginHandler)
	mux.HandleFunc("POST /auth/password", h.changePasswordHandler)
	mux.HandleFunc("POST /auth/logout", h.logoutHandler)

	// Ручки proto-сервиса: POST /api/v1/users, GET|PATCH|DELETE /api/v1/users/{uuid}.
	// StripPrefix снимает префикс: в proto пути заданы от корня (/users).
	mux.Handle("/api/v1/", http.StripPrefix("/api/v1", h.gateway))

	// Swagger UI эндпоинты
	mux.Handle(SwaggerPrefix, http.StripPrefix("/api/swagger", swagger.Routes("/api/swagger")))

	return traceMiddleware(logMiddleware(h.authenticate(metricsMiddleware(mux))))
}

func (h *handler) healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)

	if _, err := fmt.Fprintln(w, "ok"); err != nil {
		slog.Error("failed to write response", "err", err)
	}
}
