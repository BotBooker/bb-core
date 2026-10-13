package serverapi

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// Поля записей access-лога. Имена общие у обоих транспортов (кроме status/code
// с разными типами), поэтому запросы любого транспорта ищутся одним запросом в
// коллекторе логов, а protocol различает их.
const (
	// logFieldProtocol — транспорт, которым пришёл запрос: http или grpc.
	logFieldProtocol = "protocol"
	// logFieldMethod — HTTP-метод или полное имя gRPC-метода.
	logFieldMethod = "method"
	// logFieldPath — путь HTTP-запроса.
	logFieldPath = "path"
	// logFieldStatus — HTTP-код ответа.
	logFieldStatus = "status"
	// logFieldCode — gRPC-код ответа (OK, Unauthenticated и т. д.).
	logFieldCode = "code"
	// logFieldBytes — размер тела HTTP-ответа в байтах.
	logFieldBytes = "bytes"
	// logFieldRemoteAddr — адрес клиента.
	logFieldRemoteAddr = "remote_addr"
	// logFieldUserAgent — User-Agent HTTP-клиента.
	logFieldUserAgent = "user_agent"
	// logFieldDuration — время обработки запроса.
	logFieldDuration = "duration"
)

// Значения поля protocol.
const (
	protocolHTTP = "http"
	protocolGRPC = "grpc"
)

// Тексты записей access-лога: свой у каждого транспорта, чтобы по сообщению
// сразу читалось, откуда пришёл запрос.
const (
	httpRequestMessage = "http request"
	grpcRequestMessage = "grpc request"
)

// logMiddleware пишет access-лог каждого HTTP-запроса: метод, путь, статус
// ответа, размер тела и время обработки.
//
// Middleware ставится внутри traceMiddleware — запись наследует trace_id и
// span_id активного серверного спана, — и снаружи authenticate: тогда в лог
// попадают и запросы, отклонённые без токена (401/403), иначе отказы были бы
// видны только по метрикам.
//
// Служебные ручки (health, Swagger UI) пишутся на уровне DEBUG: их опрашивают
// пробы и браузер, и на INFO они бы зашумили лог (см. isServicePath). Тела
// запроса и ответа не логируются: в них бывают пароли и персональные данные.
func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Оборачиваем writer, чтобы после обработки знать статус и размер ответа.
		interceptor := &responseWriterInterceptor{ResponseWriter: w}
		next.ServeHTTP(interceptor, r)

		// Если обработчик ничего не записал, net/http автоматически отвечает 200 OK.
		if interceptor.statusCode == 0 {
			interceptor.statusCode = http.StatusOK
		}

		userAgent := strings.ReplaceAll(r.UserAgent(), "\n", "")
		userAgent = strings.ReplaceAll(userAgent, "\r", "")

		path := strings.ReplaceAll(r.URL.Path, "\n", "")
		path = strings.ReplaceAll(path, "\r", "")

		slog.Log(r.Context(), requestLogLevel(isServicePath(r.URL.Path)), httpRequestMessage,
			logFieldProtocol, protocolHTTP,
			logFieldMethod, r.Method,
			logFieldPath, path,
			logFieldStatus, interceptor.statusCode,
			logFieldBytes, interceptor.bytes,
			logFieldRemoteAddr, r.RemoteAddr,
			logFieldUserAgent, userAgent,
			logFieldDuration, time.Since(start),
		)
	})
}

// NewLogInterceptor создаёт унарный интерцептор gRPC: он пишет access-лог
// каждого вызова — метод, код ответа и время обработки.
//
// Интерцептор ставится первым в цепочке
// (grpc.ChainUnaryInterceptor(NewLogInterceptor(), NewAuthInterceptor(...))),
// то есть снаружи auth-интерцептора: тогда в лог попадают и вызовы, отклонённые
// без токена, — иначе отказ не был бы виден в логах вовсе.
//
// Служебные методы (health, reflection) пишутся на уровне DEBUG: их опрашивают
// балансировщики и пробы, и на INFO они бы зашумили лог (см. isServiceGRPCMethod).
func NewLogInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		start := time.Now()

		resp, err := handler(ctx, req)

		slog.Log(ctx, requestLogLevel(isServiceGRPCMethod(info.FullMethod)), grpcRequestMessage,
			logFieldProtocol, protocolGRPC,
			logFieldMethod, info.FullMethod,
			logFieldCode, status.Code(err).String(),
			logFieldRemoteAddr, peerAddr(ctx),
			logFieldDuration, time.Since(start),
		)

		return resp, err
	}
}

// requestLogLevel выбирает уровень записи: служебным запросам — DEBUG,
// бизнес-запросам — INFO.
func requestLogLevel(service bool) slog.Level {
	if service {
		return slog.LevelDebug
	}

	return slog.LevelInfo
}

// isServicePath сообщает, что HTTP-ручку опрашивают проба или инструмент, а не
// пользователь: health — readiness-пробами, Swagger UI — браузером.
func isServicePath(path string) bool {
	return path == healthPath || strings.HasPrefix(path, SwaggerPrefix)
}

// isServiceGRPCMethod сообщает, что gRPC-метод опрашивают проба или инструмент,
// а не клиент: health — балансировщики, reflection — grpcurl при отладке.
func isServiceGRPCMethod(fullMethod string) bool {
	return strings.HasPrefix(fullMethod, healthMethodPrefix) ||
		strings.HasPrefix(fullMethod, reflectionMethodPrefix)
}

// peerAddr возвращает адрес клиента gRPC-вызова, если он есть в контексте.
func peerAddr(ctx context.Context) string {
	caller, ok := peer.FromContext(ctx)
	if !ok {
		return ""
	}

	return caller.Addr.String()
}
