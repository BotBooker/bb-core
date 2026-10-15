package servers

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// captureLogs подменяет глобальный логгер обработчиком, который пишет в буфер,
// и возвращает буфер для проверки записей. Уровень — DEBUG, чтобы проверять и
// служебные записи access-лога. Исходный логгер возвращается после теста:
// подмена глобальная.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()

	buffer := &bytes.Buffer{}
	previous := slog.Default()

	slog.SetDefault(slog.New(slog.NewTextHandler(buffer, &slog.HandlerOptions{Level: slog.LevelDebug})))

	t.Cleanup(func() { slog.SetDefault(previous) })

	return buffer
}

// TestLogMiddleware проверяет access-лог HTTP-запросов: набор полей и выбор
// уровня записи для бизнес- и служебных ручек.
func TestLogMiddleware(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		handler    http.Handler
		wantLevel  string
		wantFields []string
	}{
		{
			name:       "бизнес-запрос пишется на INFO",
			path:       "/users/me",
			handler:    statusHandler(http.StatusUnauthorized),
			wantLevel:  "level=INFO",
			wantFields: []string{"protocol=HTTP", "method=GET", "path=/users/me", "status=401"},
		},
		{
			name:       "служебная ручка пишется на DEBUG",
			path:       healthPath,
			handler:    bodyHandler("ok"),
			wantLevel:  "level=DEBUG",
			wantFields: []string{"protocol=HTTP", "path=/health", "status=200", "bytes=2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buffer := captureLogs(t)

			logMiddleware(tt.handler).
				ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, tt.path, nil))

			assertLogContains(t, buffer.String(), append([]string{httpRequestMessage, tt.wantLevel}, tt.wantFields...))
		})
	}
}

// TestNewLogInterceptor проверяет access-лог gRPC-вызовов: метод, код ответа,
// уровень записи, а также то, что интерцептор не подменяет ответ и ошибку.
func TestNewLogInterceptor(t *testing.T) {
	tests := []struct {
		name       string
		fullMethod string
		handlerErr error
		wantLevel  string
		wantFields []string
	}{
		{
			name:       "успешный бизнес-метод пишется на INFO",
			fullMethod: "/user.v1.UserService/Get",
			wantLevel:  "level=INFO",
			wantFields: []string{"protocol=gRPC", "method=/user.v1.UserService/Get", "code=OK"},
		},
		{
			name:       "ошибка вызова видна в логе",
			fullMethod: "/user.v1.UserService/Get",
			handlerErr: status.Error(codes.Unauthenticated, unauthorizedMessage),
			wantLevel:  "level=INFO",
			wantFields: []string{"code=Unauthenticated"},
		},
		{
			name:       "служебный метод пишется на DEBUG",
			fullMethod: healthMethodPrefix + "Check",
			wantLevel:  "level=DEBUG",
			wantFields: []string{"protocol=gRPC", "method=/grpc.health.v1.Health/Check"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buffer := captureLogs(t)

			interceptor := NewLogInterceptor()
			info := &grpc.UnaryServerInfo{FullMethod: tt.fullMethod}
			handler := func(context.Context, any) (any, error) { return "response", tt.handlerErr }

			resp, err := interceptor(context.Background(), nil, info, handler)

			if !errors.Is(err, tt.handlerErr) {
				t.Errorf("interceptor() error = %v, want %v", err, tt.handlerErr)
			}
			if resp != "response" {
				t.Errorf("interceptor() response = %v, want %q", resp, "response")
			}

			assertLogContains(t, buffer.String(), append([]string{grpcRequestMessage, tt.wantLevel}, tt.wantFields...))
		})
	}
}

// TestIsServicePath проверяет разделение ручек на бизнес- и служебные: от этого
// зависит уровень записи access-лога.
func TestIsServicePath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: healthPath, want: true},
		{path: SwaggerPrefix + "swagger-ui.html", want: true},
		{path: "/users/me", want: false},
		{path: "/api/v1/users", want: false},
	}

	for _, tt := range tests {
		if got := isServicePath(tt.path); got != tt.want {
			t.Errorf("isServicePath(%q) = %t, want %t", tt.path, got, tt.want)
		}
	}
}

// statusHandler отвечает заданным статусом без тела.
func statusHandler(status int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	})
}

// bodyHandler отвечает 200 OK с заданным телом.
func bodyHandler(body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	})
}

// assertLogContains проверяет, что в накопленных записях есть все подстроки.
func assertLogContains(t *testing.T, log string, want []string) {
	t.Helper()

	for _, substring := range want {
		if !strings.Contains(log, substring) {
			t.Errorf("лог %q не содержит %q", log, substring)
		}
	}
}
