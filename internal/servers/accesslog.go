package servers

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/botbooker/bb-core/pkg/tools"
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
	protocolHTTP = "HTTP"
	protocolGRPC = "gRPC"
)

// Тексты записей access-лога: свой у каждого транспорта, чтобы по сообщению
// сразу читалось, откуда пришёл запрос.
const (
	httpRequestMessage = "HTTP request"
	grpcRequestMessage = "gRPC request"
)

// logMiddleware пишет access-лог каждого HTTP-запроса: метод, путь, статус
// ответа, размер тела и время обработки.
//
// Middleware ставится внутри traceMiddleware — запись наследует trace_id
// активной серверной трассы, — и снаружи authenticate: тогда в лог
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

		// Санитизации подлежат только значения, пришедшие от пользователя:
		// path и user_agent. Остальные поля считаются доверенными и берутся
		// как есть: method валидируется net/http по RFC 7230 (токен без CTL и
		// разделителей), remote_addr берётся из соединения, а не из запроса.
		// Значит log-injection через них невозможен, и правило §2.1 к ним не
		// применяется (см. решения ревью, П.5 A2). Обязательство: если в лог
		// начнёт попадать X-Forwarded-For (HTTP_HEADER_CLIENT_IP), это будет
		// пользовательское значение и его придётся санитизировать.
		slog.Log(r.Context(), requestLogLevel(isServicePath(r.URL.Path)), httpRequestMessage,
			slog.String(logFieldProtocol, protocolHTTP),
			slog.String(logFieldMethod, r.Method),
			slog.String(logFieldPath, tools.SanitizeInputString(r.URL.Path)),
			slog.Int(logFieldStatus, interceptor.statusCode),
			slog.Int(logFieldBytes, interceptor.bytes),
			slog.String(logFieldRemoteAddr, r.RemoteAddr),
			slog.String(logFieldUserAgent, tools.SanitizeInputString(r.UserAgent())),
			slog.Duration(logFieldDuration, time.Since(start)),
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
			slog.String(logFieldProtocol, protocolGRPC),
			slog.String(logFieldMethod, info.FullMethod),
			slog.String(logFieldCode, status.Code(err).String()),
			slog.String(logFieldRemoteAddr, peerAddr(ctx)),
			slog.Duration(logFieldDuration, time.Since(start)),
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
	return path == healthPath || path == readyPath || strings.HasPrefix(path, SwaggerPrefix)
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
