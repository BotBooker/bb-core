package otel

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace"
)

func TestGetTraceInfo_WithSpanContext(t *testing.T) {
	// Создаем контекст с трассировкой
	ctx := context.Background()
	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanID:     trace.SpanID{17, 18, 19, 20, 21, 22, 23, 24},
		TraceFlags: trace.TraceFlags(0x01),
	})
	ctx = trace.ContextWithSpanContext(ctx, spanCtx)

	// Вызываем функцию
	traceID, spanID, isSampled := GetTraceInfo(ctx)

	// Проверяем результат
	assert.Equal(t, "0102030405060708090a0b0c0d0e0f10", traceID)
	assert.Equal(t, "1112131415161718", spanID)
	assert.True(t, isSampled)
}

func TestGetTraceInfo_WithoutSpanContext(t *testing.T) {
	// Создаем контекст без трассировки
	ctx := context.Background()

	// Вызываем функцию
	traceID, spanID, isSampled := GetTraceInfo(ctx)

	// Проверяем результат
	assert.Equal(t, "", traceID)
	assert.Equal(t, "", spanID)
	assert.False(t, isSampled)
}

func TestGetTraceInfo_LogHook(t *testing.T) {
	// Устанавливаем лог-хук
	logged := false
	LogHook = func(format string, args ...any) {
		logged = true
	}
	defer func() {
		LogHook = nil
	}()

	// Создаем контекст с трассировкой
	ctx := context.Background()
	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanID:     trace.SpanID{17, 18, 19, 20, 21, 22, 23, 24},
		TraceFlags: trace.TraceFlags(0x01),
	})
	ctx = trace.ContextWithSpanContext(ctx, spanCtx)

	// Вызываем функцию
	GetTraceInfo(ctx)

	// Проверяем, что хук был вызван
	assert.True(t, logged)
}

func TestGetTraceInfo_NotSampled(t *testing.T) {
	// Создаем контекст с трассировкой, но без флага sampled
	ctx := context.Background()
	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanID:     trace.SpanID{17, 18, 19, 20, 21, 22, 23, 24},
		TraceFlags: trace.TraceFlags(0x00), // Not sampled
	})
	ctx = trace.ContextWithSpanContext(ctx, spanCtx)

	// Вызываем функцию
	traceID, spanID, isSampled := GetTraceInfo(ctx)

	// Проверяем результат
	assert.Equal(t, "0102030405060708090a0b0c0d0e0f10", traceID)
	assert.Equal(t, "1112131415161718", spanID)
	assert.False(t, isSampled)
}

func TestGetTraceInfo_OnlyTraceID(t *testing.T) {
	// Создаем контекст только с TraceID
	ctx := context.Background()
	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		TraceFlags: trace.TraceFlags(0x01),
	})
	ctx = trace.ContextWithSpanContext(ctx, spanCtx)

	// Вызываем функцию
	traceID, spanID, isSampled := GetTraceInfo(ctx)

	// Проверяем результат
	assert.Equal(t, "0102030405060708090a0b0c0d0e0f10", traceID)
	assert.Equal(t, "", spanID) // SpanID должен быть пустым
	assert.True(t, isSampled)
}

func TestGetTraceInfo_OnlySpanID(t *testing.T) {
	// Создаем контекст только с SpanID
	ctx := context.Background()
	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		SpanID:     trace.SpanID{17, 18, 19, 20, 21, 22, 23, 24},
		TraceFlags: trace.TraceFlags(0x01),
	})
	ctx = trace.ContextWithSpanContext(ctx, spanCtx)

	// Вызываем функцию
	traceID, spanID, isSampled := GetTraceInfo(ctx)

	// Проверяем результат
	assert.Equal(t, "", traceID) // TraceID должен быть пустым
	assert.Equal(t, "1112131415161718", spanID)
	assert.True(t, isSampled)
}

func TestInitTracer(t *testing.T) {
	// Проверяем, что InitTracer не вызывает панику
	assert.NotPanics(t, func() {
		InitTracer()
	})
}

func TestGetApplicationName_WithEnvSet(t *testing.T) {
	os.Setenv("OTEL_SERVICE_NAME", "custom-service")
	defer os.Unsetenv("OTEL_SERVICE_NAME")

	result := GetApplicationName("default-service")

	assert.Equal(t, "custom-service", result)
}

func TestGetApplicationName_WithEnvUnset(t *testing.T) {
	os.Unsetenv("OTEL_SERVICE_NAME")

	result := GetApplicationName("default-service")

	assert.Equal(t, "default-service", result)
}

func TestGetApplicationName_WithEmptyEnv(t *testing.T) {
	os.Setenv("OTEL_SERVICE_NAME", "")
	defer os.Unsetenv("OTEL_SERVICE_NAME")

	result := GetApplicationName("default-service")

	assert.Equal(t, "default-service", result)
}
