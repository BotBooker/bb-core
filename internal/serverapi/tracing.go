package serverapi

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/botbooker/bb-core/pkg/observability"
)

// traceMiddleware извлекает контекст трассировки из заголовков запроса и
// запускает серверный спан.
//
// Сценарий 2: W3C Trace Context (traceparent/tracestate) разбирается
// otel.GetTextMapPropagator().Extract() — если передан валидный Traceparent,
// сэмплер pkg/observability запишет спан как продолжение входящей трассы.
// Сценарий 1: при ENABLE_FORCE_TRACE=true записываются все запросы, даже без
// заголовков трассировки.
//
// Если адрес коллектора не задан, трейсер no-op и middleware почти ничего не
// делает: пропагатор всё равно извлекает контекст, чтобы он остался доступен
// ниже по стеку.
func traceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// 1. Извлекаем контекст трассировки из заголовков запроса
		ctx := otel.GetTextMapPropagator().Extract(req.Context(), propagation.HeaderCarrier(req.Header))

		// 2. Стартуем серверный спан
		ctx, span := observability.Tracer().Start(ctx, req.Method+" "+req.URL.Path,
			trace.WithSpanKind(trace.SpanKindServer),
		)
		defer span.End()

		// 3. Оборачиваем оригинальный response writer в наш интерцептор
		interceptor := &responseWriterInterceptor{ResponseWriter: w}

		// 4. Передаем управление следующему обработчику
		next.ServeHTTP(interceptor, req.WithContext(ctx))

		// 5. Если в обработчиках статус не был установлен явно — это был успешный ответ 200 OK
		if interceptor.statusCode == 0 {
			interceptor.statusCode = http.StatusOK
		}

		// 6. Записываем стандартный тег OpenTelemetry для HTTP статуса
		span.SetAttributes(attribute.Int("http.response.status_code", interceptor.statusCode))

		// 7. Если статус 5xx (ошибка сервера), красим спан в Jaeger в красный цвет
		if interceptor.statusCode >= 500 {
			span.SetStatus(codes.Error, "HTTP server error")
		}
	})
}

// startStartupSpan открывает стартовый спан приложения (сценарий 3).
//
// Спан начинает новую трассу: trace.WithNewRoot() заставляет SDK сгенерировать
// собственный TraceID. Закрывать спан нужно, когда все компоненты приложения
// готовы принимать трафик, — тогда стартовая трасса отражает реальное время
// запуска, а не только создание провайдера.
//
// Атрибут observability.ForceSample() помечает спан обязательным к записи: у
// него нет входящего traceparent, поэтому сэмплер pkg/observability записал бы
// его только в форс-режиме.
func startStartupSpan(ctx context.Context) (context.Context, trace.Span) {
	return observability.Tracer().Start(ctx, "app.startup",
		trace.WithNewRoot(),
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(observability.ForceSample()),
	)
}

// startShutdownSpan открывает спан graceful shutdown (сценарий 4).
//
// Вызывается после перехвата SIGINT/SIGTERM: новый TraceID (trace.WithNewRoot)
// выделяет завершение процесса в отдельную трассу, не связанную с входящими
// запросами. Спан закрывается перед выходом процесса — после остановки
// серверов и закрытия ресурсов.
func startShutdownSpan(ctx context.Context) (context.Context, trace.Span) {
	return observability.Tracer().Start(ctx, "app.shutdown",
		trace.WithNewRoot(),
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(observability.ForceSample()),
	)
}

// responseWriterInterceptor — это обертка над стандартным http.ResponseWriter,
// которая запоминает HTTP статус-код и размер тела ответа.
//
// Одна и та же обёртка используется и трассировкой (статус нужен для тега спана
// http.response.status_code), и access-логом (статус и число байт — поля записи).
// Вложенные обёртки не мешают друг другу: Write и WriteHeader прозрачно
// пробрасываются во writer ниже.
type responseWriterInterceptor struct {
	http.ResponseWriter
	statusCode int
	bytes      int
}

func (w *responseWriterInterceptor) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseWriterInterceptor) Write(b []byte) (int, error) {
	// Если WriteHeader не был вызван явно, net/http автоматически выставляет 200 OK
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}

	// Размер считаем по фактически записанным байтам: writer ниже может
	// отбросить часть записи, но для access-лога это и есть отправленное тело.
	written, err := w.ResponseWriter.Write(b)
	w.bytes += written

	return written, err
}
