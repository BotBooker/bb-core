package serverapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const (
	// envTracesEndpoint — адрес OTel Collector именно для трасс (приоритет).
	envTracesEndpoint = "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"
	// envForceTrace — форс-трассировка: "true" включает запись всех спанов
	// независимо от входящего traceparent (сценарий 1).
	envForceTrace = "ENABLE_FORCE_TRACE"
	// forceTraceValue — значение envForceTrace, включающее форс-трассировку.
	forceTraceValue = "true"

	// forceSampleAttributeKey — маркер спана, который сэмплер записывает всегда.
	// Нужен стартовому и завершающему спанам приложения (сценарии 3–4): входящего
	// traceparent у них нет, а форс-режим может быть выключен.
	forceSampleAttributeKey = "bb.core.force_sample"
)

// setupTraces настраивает экспорт трасс в OTel Collector по OTLP/gRPC.
//
// Экспорт включается, только если задан адрес коллектора: переменная окружения
// OTEL_EXPORTER_OTLP_TRACES_ENDPOINT (приоритет) или общая
// OTEL_EXPORTER_OTLP_ENDPOINT. Если ни одной из них нет, провайдер трассировки
// не создаётся вовсе: глобальный otel.GetTracerProvider остаётся no-op, а
// функция возвращает no-op shutdown. Порт коллектора по умолчанию — 4317.
func setupTraces(ctx context.Context) (func(context.Context) error, error) {
	endpoint, enabled := tracesEndpoint()
	if !enabled {
		slog.Debug("otlp traces export disabled: collector endpoint is not set",
			"traces_endpoint_env", envTracesEndpoint,
			"common_endpoint_env", envCommonEndpoint,
		)

		return func(context.Context) error { return nil }, nil
	}

	exporter, err := otlptracegrpc.New(ctx, traceExporterOptions(endpoint)...)
	if err != nil {
		return func(context.Context) error { return nil }, fmt.Errorf("create otlp trace exporter: %w", err)
	}

	// bsp — батчевый процессор спанов: копит завершённые спаны в буфере и
	// отправляет их экспортёру пачками, а не по одному. Он создаётся явно (а не
	// через sdktrace.WithBatcher) ради единообразия с логированием, где
	// sdklog.NewBatchProcessor тоже вызывается напрямую: так видно, какой
	// процессор получает провайдер, и его настройки легко переопределить в тестах.
	// Владеет процессором провайдер: provider.Shutdown останавливает bsp и
	// дожидается выгрузки буфера, отдельно закрывать bsp не нужно.
	bsp := sdktrace.NewBatchSpanProcessor(exporter)

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(bsp),
		sdktrace.WithSampler(newTraceSampler()),
	)

	otel.SetTracerProvider(provider)

	slog.Info("otlp traces export enabled", "endpoint", endpoint, "protocol", "grpc")

	return provider.Shutdown, nil
}

// tracesEndpoint возвращает адрес OTLP-коллектора для трасс и признак того, что
// экспорт трасс включён. OTEL_EXPORTER_OTLP_TRACES_ENDPOINT имеет приоритет над
// общей OTEL_EXPORTER_OTLP_ENDPOINT, как и требует спецификация OpenTelemetry.
func tracesEndpoint() (endpoint string, enabled bool) {
	if endpoint = os.Getenv(envTracesEndpoint); endpoint != "" {
		return endpoint, true
	}
	if endpoint = os.Getenv(envCommonEndpoint); endpoint != "" {
		return endpoint, true
	}

	return "", false
}

// traceExporterOptions приводит адрес коллектора к виду, понятному OTLP-экспортёру
// трасс (см. normalizeEndpoint).
func traceExporterOptions(endpoint string) []otlptracegrpc.Option {
	return []otlptracegrpc.Option{otlptracegrpc.WithEndpointURL(normalizeEndpoint(endpoint))}
}

// forceTraceEnabled сообщает, включена ли форс-трассировка (сценарий 1):
// ENABLE_FORCE_TRACE=true заставляет сэмплер записывать все спаны.
func forceTraceEnabled() bool {
	return strings.EqualFold(os.Getenv(envForceTrace), forceTraceValue)
}

// newTraceSampler собирает сэмплер приложения: он реализует правила запуска
// спанов из сценариев 1–4 (см. traceSampler).
func newTraceSampler() sdktrace.Sampler {
	return traceSampler{force: forceTraceEnabled()}
}

// tracer возвращает инструментарий приложения. Если провайдер трассировки не
// настроен (нет адреса коллектора), это no-op трейсер: спаны создаются, но не
// записываются и не экспортируются.
func tracer() trace.Tracer {
	return otel.Tracer(instrumentationScope)
}

// traceSampler решает, попадёт ли спан в экспорт. Правила соответствуют
// сценариям запуска спанов:
//
//   - сценарий 1 (Force Trace): ENABLE_FORCE_TRACE=true — записываем все спаны;
//   - сценарий 2 (Context Propagation): у спана есть валидный родитель — спан
//     продолжает входящую или уже начатую трассу, поэтому тоже записывается;
//   - сценарии 3–4 (Startup/Shutdown Trace): спан помечен forceSampleAttributeKey
//     (startStartupSpan, startShutdownSpan) — записываем независимо от родителя.
//
// Во всех остальных случаях спан отбрасывается: приложение не превращает
// каждый запрос без traceparent в новую трассу.
type traceSampler struct {
	// force — включён ли форс-режим (ENABLE_FORCE_TRACE).
	force bool
}

// ShouldSample применяет правила сэмплирования сценариев 1–4.
func (s traceSampler) ShouldSample(params sdktrace.SamplingParameters) sdktrace.SamplingResult {
	// Сценарий 1: форс-трассировка — записываем все спаны.
	if s.force {
		return recordAndSample()
	}

	// Сценарии 3–4: стартовый и завершающий спаны приложения — записываем всегда.
	if isForceSampled(params.Attributes) {
		return recordAndSample()
	}

	// Сценарий 2: продолжение трассы из входящих заголовков. Валидный удалённый
	// родитель — это полученный по сети traceparent, его продолжаем независимо
	// от флага sampled. Для локального родителя уважаем принятое решение:
	// потомки невыбранного спана тоже не записываются.
	parent := trace.SpanContextFromContext(params.ParentContext)
	if parent.IsValid() && (parent.IsRemote() || parent.IsSampled()) {
		return recordAndSample()
	}

	return sdktrace.SamplingResult{Decision: sdktrace.Drop}
}

// Description описывает сэмплер для диагностики (например, в логах SDK).
func (s traceSampler) Description() string {
	return fmt.Sprintf("bb-core-trace-sampler{force=%t}", s.force)
}

// recordAndSample — решение «записать и сэмплировать» спан.
func recordAndSample() sdktrace.SamplingResult {
	return sdktrace.SamplingResult{Decision: sdktrace.RecordAndSample}
}

// isForceSampled сообщает, помечен ли спан маркером обязательной записи.
func isForceSampled(attrs []attribute.KeyValue) bool {
	for _, attr := range attrs {
		if attr.Key == forceSampleAttributeKey && attr.Value.AsBool() {
			return true
		}
	}

	return false
}

// forceSample — атрибут-маркер для спанов, которые нужно записать всегда.
func forceSample() attribute.KeyValue {
	return attribute.Bool(forceSampleAttributeKey, true)
}

// traceMiddleware извлекает контекст трассировки из заголовков запроса и
// запускает серверный спан.
//
// Сценарий 2: W3C Trace Context (traceparent/tracestate) разбирается
// otel.GetTextMapPropagator().Extract() — если передан валидный Traceparent,
// сэмплер запишет спан как продолжение входящей трассы.
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
		ctx, span := tracer().Start(ctx, req.Method+" "+req.URL.Path,
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
func startStartupSpan(ctx context.Context) (context.Context, trace.Span) {
	return tracer().Start(ctx, "app.startup",
		trace.WithNewRoot(),
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(forceSample()),
	)
}

// startShutdownSpan открывает спан graceful shutdown (сценарий 4).
//
// Вызывается после перехвата SIGINT/SIGTERM: новый TraceID (trace.WithNewRoot)
// выделяет завершение процесса в отдельную трассу, не связанную с входящими
// запросами. Спан закрывается перед выходом процесса — после остановки
// серверов и закрытия ресурсов.
func startShutdownSpan(ctx context.Context) (context.Context, trace.Span) {
	return tracer().Start(ctx, "app.shutdown",
		trace.WithNewRoot(),
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(forceSample()),
	)
}

// For TraceSpan
// traceConfig хранит внутренние настройки для создания спана
type traceConfig struct {
	tracerName string
	tags       []attribute.KeyValue
}

// TraceOption определяет функцию для изменения конфигурации спана
type TraceOption func(*traceConfig)

// responseWriterInterceptor — это обертка над стандартным http.ResponseWriter,
// которая запоминает HTTP статус-код ответа.
type responseWriterInterceptor struct {
	http.ResponseWriter
	statusCode int
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
	return w.ResponseWriter.Write(b)
}

// WithTracerName задает кастомное имя компонента
func WithTracerName(name string) TraceOption {
	return func(c *traceConfig) {
		c.tracerName = name
	}
}

// WithTags добавляет теги к спану
func WithTags(tags ...attribute.KeyValue) TraceOption {
	return func(c *traceConfig) {
		c.tags = append(c.tags, tags...)
	}
}

// TraceSpan оборачивает выполнение кода в спан OpenTelemetry.
// Все аргументы после operationName являются НЕОБЯЗАТЕЛЬНЫМИ.
// Пример:
// И теги, и имя трейсера в любом порядке
// func saveInvoice(ctx context.Context, id int) {
// 	ctx, end := TraceSpan(ctx, "save-to-db",
// 		WithTracerName("database-client"),
// 		WithTags(
// 			attribute.Int("invoice.id", id),
// 			attribute.String("db.table", "invoices"),
// 		),
// 	)
// 	defer end()
// }

func TraceSpan(ctx context.Context, operationName string, opts ...TraceOption) (context.Context, func()) {
	// Настройки по умолчанию
	cfg := traceConfig{
		tracerName: "runBackground",
	}

	// Применяем переданные опции (если они есть)
	for _, opt := range opts {
		opt(&cfg)
	}

	// Формируем опции для OpenTelemetry
	var otelOpts []trace.SpanStartOption
	if len(cfg.tags) > 0 {
		otelOpts = append(otelOpts, trace.WithAttributes(cfg.tags...))
	}

	// Стартуем спан
	tracer := otel.Tracer(cfg.tracerName)
	newCtx, span := tracer.Start(ctx, operationName, otelOpts...)

	return newCtx, func() {
		span.End()
	}
}
