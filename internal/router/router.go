package router

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/botbooker/bb-core/internal/health"
	"github.com/botbooker/bb-core/internal/observability/otel"
)

// RouterConfig содержит конфигурацию маршрутизатора.
type RouterConfig struct {
	TrustedProxies  []string
	TrustedPlatform string
}

// SetupRouter инициализирует и настраивает chi-роутер с middleware и маршрутами.
// Вынесено в отдельную функцию для удобства тестирования.
func SetupRouter() *chi.Mux {
	return setupEngine()
}

// setupEngine создает и настраивает базовый chi-роутер.
func setupEngine() *chi.Mux {
	router := chi.NewRouter()
	applicationName := otel.GetApplicationName("botbooker-api")

	// Use standard middleware
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)

	// OpenTelemetry middleware
	router.Use(otelhttp.NewMiddleware(applicationName))

	// Register routes
	RegisterRoutes(router)

	return router
}

// RegisterRoutes регистрирует все маршруты API.
func RegisterRoutes(router chi.Router) {
	router.Get("/ping", health.PingHandler)
	router.Get("/healthz", health.Health)
}
