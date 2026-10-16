package servers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// testUserUUID — UUID в пути запроса: нормализация должна заменить его на {uuid}.
const testUserUUID = "2f6f2a1e-3c1d-4d0e-8f6a-1b2c3d4e5f60"

// TestRouteLabel проверяет выбор метки маршрута: шаблон из Request.Pattern,
// нормализованный путь для gateway-ручек и единая метка для несовпавших
// запросов (иначе кардинальность метрик росла бы от произвольных URL).
func TestRouteLabel(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		want    string
	}{
		{
			name:    "шаблон обычной ручки",
			pattern: "GET /users/me",
			path:    "/users/me",
			want:    "/users/me",
		},
		{
			name:    "gateway-ручка нормализуется по пути",
			pattern: gatewayRoutePrefix,
			path:    "/api/v1/users/" + testUserUUID,
			want:    "/api/v1/users/{uuid}",
		},
		{
			name:    "несовпавший запрос не попадает в метку",
			pattern: "",
			path:    "/unknown/" + testUserUUID,
			want:    unmatchedRoute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req.Pattern = tt.pattern

			if got := routeLabel(req); got != tt.want {
				t.Errorf("routeLabel() = %q, ожидалось %q", got, tt.want)
			}
		})
	}
}

// TestNormalizePath проверяет нормализацию пути: идентификаторы схлопываются,
// остальные сегменты сохраняются.
func TestNormalizePath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "корень", path: "/", want: "/"},
		{name: "пустой путь", path: "", want: ""},
		{name: "без идентификаторов", path: "/users/me", want: "/users/me"},
		{name: "UUID схлопывается", path: "/api/v1/users/" + testUserUUID, want: "/api/v1/users/{uuid}"},
		{
			name: "несколько UUID",
			path: "/a/" + testUserUUID + "/b/" + testUserUUID,
			want: "/a/{uuid}/b/{uuid}",
		},
		{name: "не UUID остаётся", path: "/users/not-a-uuid", want: "/users/not-a-uuid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizePath(tt.path); got != tt.want {
				t.Errorf("normalizePath(%q) = %q, ожидалось %q", tt.path, got, tt.want)
			}
		})
	}
}

// TestMetricsMiddlewareRecordsHTTPRoute проверяет, что middleware метрик пишет
// длительность запроса с нормализованным маршрутом и кодом ответа.
func TestMetricsMiddlewareRecordsHTTPRoute(t *testing.T) {
	reader := useManualMeterProvider(t)

	handler := metricsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+testUserUUID, nil)
	// Так шаблон выглядит у ручек grpc-gateway (см. routeLabel).
	req.Pattern = gatewayRoutePrefix

	handler.ServeHTTP(httptest.NewRecorder(), req)

	histogram := collectHistogram(t, reader, "http.server.request.duration")
	if len(histogram.DataPoints) != 1 {
		t.Fatalf("точек данных = %d, ожидалась 1", len(histogram.DataPoints))
	}

	attrs := histogram.DataPoints[0].Attributes

	if got := attrString(t, attrs, attrHTTPRoute); got != "/api/v1/users/{uuid}" {
		t.Errorf("%s = %q, ожидался %q", attrHTTPRoute, got, "/api/v1/users/{uuid}")
	}
	if got := attrString(t, attrs, attrHTTPRequestMethod); got != http.MethodGet {
		t.Errorf("%s = %q, ожидался %q", attrHTTPRequestMethod, got, http.MethodGet)
	}

	statusAttr, ok := attrs.Value(attribute.Key(attrHTTPStatusCode))
	if !ok || statusAttr.AsInt64() != http.StatusTeapot {
		t.Errorf("%s = %v, ожидался %d", attrHTTPStatusCode, statusAttr, http.StatusTeapot)
	}
}

// TestNewMetricsInterceptor проверяет, что интерцептор метрик не подменяет
// ответ и ошибку обработчика и записывает длительность вызова.
func TestNewMetricsInterceptor(t *testing.T) {
	reader := useManualMeterProvider(t)

	interceptor := NewMetricsInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/user.v1.UserService/Get"}
	handlerErr := status.Error(codes.NotFound, notFoundMessage)
	handler := func(context.Context, any) (any, error) { return "response", handlerErr }

	resp, err := interceptor(context.Background(), nil, info, handler)
	if !errors.Is(err, handlerErr) {
		t.Errorf("interceptor() error = %v, ожидалось %v", err, handlerErr)
	}
	if resp != "response" {
		t.Errorf("interceptor() response = %v, ожидался %q", resp, "response")
	}

	histogram := collectHistogram(t, reader, "rpc.server.duration")
	if len(histogram.DataPoints) != 1 {
		t.Fatalf("точек данных = %d, ожидалась 1", len(histogram.DataPoints))
	}

	attrs := histogram.DataPoints[0].Attributes

	if got := attrString(t, attrs, attrRPCMethod); got != info.FullMethod {
		t.Errorf("%s = %q, ожидался %q", attrRPCMethod, got, info.FullMethod)
	}

	codeAttr, ok := attrs.Value(attribute.Key(attrRPCStatusCode))
	if !ok || codeAttr.AsInt64() != int64(codes.NotFound) {
		t.Errorf("%s = %v, ожидался %d", attrRPCStatusCode, codeAttr, codes.NotFound)
	}
}

// useManualMeterProvider выставляет глобальный провайдер метрик с ручным
// reader'ом и возвращает его, чтобы тест мог собрать записанные измерения.
//
// Инструменты метрик создаются лениво (metricsOnce), поэтому перед сменой
// провайдера их нужно сбросить, а после теста — сбросить ещё раз, чтобы
// следующий тест привязал их к восстановленному провайдеру.
func useManualMeterProvider(t *testing.T) sdkmetric.Reader {
	t.Helper()

	resetMetricsTools(t)

	reader := sdkmetric.NewManualReader()

	previous := otel.GetMeterProvider()
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	t.Cleanup(func() { otel.SetMeterProvider(previous) })

	return reader
}

// resetMetricsTools сбрасывает ленивую инициализацию инструментов метрик до и
// после теста.
func resetMetricsTools(t *testing.T) {
	t.Helper()

	metricsOnce = sync.Once{}
	httpServerLatency = nil
	grpcServerLatency = nil

	t.Cleanup(func() {
		metricsOnce = sync.Once{}
		httpServerLatency = nil
		grpcServerLatency = nil
	})
}

// collectHistogram собирает измерения reader'а и возвращает гистограмму с
// заданным именем.
func collectHistogram(t *testing.T, reader sdkmetric.Reader, name string) metricdata.Histogram[float64] {
	t.Helper()

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &collected); err != nil {
		t.Fatalf("не удалось собрать метрики: %v", err)
	}

	for _, scope := range collected.ScopeMetrics {
		for _, metrics := range scope.Metrics {
			if metrics.Name != name {
				continue
			}

			histogram, ok := metrics.Data.(metricdata.Histogram[float64])
			if !ok {
				t.Fatalf("метрика %q имеет тип %T, ожидалась гистограмма", name, metrics.Data)
			}

			return histogram
		}
	}

	t.Fatalf("метрика %q не найдена", name)

	return metricdata.Histogram[float64]{}
}

// attrString достаёт строковый атрибут точки данных.
func attrString(t *testing.T, attrs attribute.Set, key string) string {
	t.Helper()

	value, ok := attrs.Value(attribute.Key(key))
	if !ok {
		t.Fatalf("атрибут %q отсутствует", key)
	}

	return value.AsString()
}
