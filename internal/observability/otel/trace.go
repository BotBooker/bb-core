// Package otel предоставляет утилиты для работы с OpenTelemetry трассировкой.
//
// Основные функции:
//   - GetTraceInfo: извлекает идентификаторы trace/span и флаг sampled из контекста.
//   - LogHook: опциональный хук для логирования данных трассировки.
package otel

import (
	"context"
	"os"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// LogHook — опциональный хук для логирования информации о трассировке.
//
// Если установлен, вызывается внутри GetTraceInfo с форматированной строкой,
// содержащей traceID, spanID и isSampled.
//
// Пример установки:
//
//	otel.LogHook = func(format string, args ...any) {
//		log.Printf(format, args...)
//	}
var LogHook func(string, ...any)

// GetTraceInfo извлекает данные трассировки из контекста.
//
// Параметры:
//   - ctx context.Context: контекст, содержащий SpanContext OpenTelemetry.
//
// Возвращает:
//   - traceID string: идентификатор трассировки (пустая строка, если отсутствует);
//   - spanID string: идентификатор спана (пустая строка, если отсутствует);
//   - isSampled bool: флаг, указывающий, был ли спан выбран для сбора (sampled).
//
// Если LogHook установлен, функция вызывает его с отформатированной строкой,
// содержащей извлечённые данные.
func GetTraceInfo(ctx context.Context) (traceID string, spanID string, isSampled bool) {
	spanCtx := trace.SpanContextFromContext(ctx)

	if spanCtx.HasTraceID() {
		traceID = spanCtx.TraceID().String()
	}
	if spanCtx.HasSpanID() {
		spanID = spanCtx.SpanID().String()
	}

	isSampled = spanCtx.IsSampled()

	if LogHook != nil {
		LogHook("traceID: %v; spanID: %v; isSampled: %v", traceID, spanID, isSampled)
	}

	return traceID, spanID, isSampled
}

// InitTracer инициализирует провайдер трассировки OpenTelemetry.
//
// Создаёт новый TracerProvider с сэмплером ParentBased(AlwaysSample())
// и устанавливает его как глобальный провайдер трассировки.
//
// Пример использования:
//
//	otel.InitTracer()
func InitTracer() {
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())),
	)
	otel.SetTracerProvider(tp)
}

// GetApplicationName возвращает имя приложения из переменной окружения OTEL_SERVICE_NAME
// или значение по умолчанию, если переменная не установлена.
//
// Параметры:
//   - defaultName: имя по умолчанию.
//
// Возвращает:
//   - string: имя приложения из переменной окружения или значение по умолчанию.
//
// Пример использования:
//
//	appName := otel.GetApplicationName("botbooker-api")
func GetApplicationName(defaultName string) string {
	if name := os.Getenv("OTEL_SERVICE_NAME"); name != "" {
		return name
	}
	return defaultName
}
