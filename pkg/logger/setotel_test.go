package logger

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// stubLogger подменяет состояние пакета: baseHandler (nil — «логгер ещё не
// инициализирован») и уровень, установленный InitAndSetDefault. Состояние и
// глобальный логгер восстанавливаются после теста. Без подмены SetOTelHandler
// писал бы в os.Stdout, который в тестах не перехватить.
func stubLogger(t *testing.T, base slog.Handler, level slog.Level) {
	t.Helper()

	previousDefault := slog.Default()

	handlerMu.Lock()
	previousBase, previousOTel, previousLevel := baseHandler, otelHandler, logLevel
	baseHandler, otelHandler, logLevel = base, nil, level
	handlerMu.Unlock()

	t.Cleanup(func() {
		handlerMu.Lock()
		baseHandler, otelHandler, logLevel = previousBase, previousOTel, previousLevel
		handlerMu.Unlock()

		slog.SetDefault(previousDefault)
	})
}

// captureStdout фиксирует baseHandler поверх нового буфера и возвращает его.
func captureStdout(t *testing.T) *bytes.Buffer {
	t.Helper()

	buf := &bytes.Buffer{}
	stubLogger(t, slog.NewTextHandler(buf, nil), slog.LevelDebug)

	return buf
}

func TestSetOTelHandler_KeepsStdoutAndAddsOTel(t *testing.T) {
	stdout := captureStdout(t)

	var otel bytes.Buffer

	SetOTelHandler(slog.NewTextHandler(&otel, nil))

	slog.Info("hello")

	if !strings.Contains(stdout.String(), "hello") {
		t.Errorf("запись не попала в stdout: %q", stdout.String())
	}
	if !strings.Contains(otel.String(), "hello") {
		t.Errorf("запись не попала в OTel: %q", otel.String())
	}
}

func TestSetOTelHandler_ReplacesInsteadOfStacking(t *testing.T) {
	stdout := captureStdout(t)

	var otel bytes.Buffer

	for range 2 {
		SetOTelHandler(slog.NewTextHandler(&otel, nil))
	}

	slog.Info("once")

	if got := strings.Count(otel.String(), "once"); got != 1 {
		t.Errorf("OTel-обработчик получил запись %d раз, ожидалось 1: %q", got, otel.String())
	}
	if got := strings.Count(stdout.String(), "once"); got != 1 {
		t.Errorf("stdout получил запись %d раз, ожидалось 1: %q", got, stdout.String())
	}
}

func TestSetOTelHandler_NilDetachesOTel(t *testing.T) {
	stdout := captureStdout(t)

	var otel bytes.Buffer

	SetOTelHandler(slog.NewTextHandler(&otel, nil))
	SetOTelHandler(nil)

	slog.Info("only stdout")

	if !strings.Contains(stdout.String(), "only stdout") {
		t.Errorf("запись не попала в stdout: %q", stdout.String())
	}
	if strings.Contains(otel.String(), "only stdout") {
		t.Errorf("запись не должна попадать в OTel после отключения: %q", otel.String())
	}
}

func TestSetOTelHandler_WithoutInit_DoesNotDeadlock(t *testing.T) {
	// baseHandler не задан — SetOTelHandler обязан взять свой обработчик вместо
	// stdlib defaultHandler. Иначе slog.SetDefault переведёт log.SetOutput на
	// этот же обработчик, и вывод через пакет log уйдёт в рекурсию.
	stubLogger(t, nil, slog.LevelDebug)

	var otel bytes.Buffer

	SetOTelHandler(slog.NewTextHandler(&otel, nil))

	slog.Info("fallback")

	if !strings.Contains(otel.String(), "fallback") {
		t.Errorf("запись не попала в OTel: %q", otel.String())
	}
}

func TestInitAndSetDefault_KeepsAttachedOTelHandler(t *testing.T) {
	stubLogger(t, nil, slog.LevelDebug)

	var otel bytes.Buffer

	SetOTelHandler(slog.NewTextHandler(&otel, nil))

	// Повторная инициализация логгера не должна терять OTel-обработчик.
	InitAndSetDefault(slog.LevelInfo, LogFormatText)

	slog.Info("after reinit")

	if !strings.Contains(otel.String(), "after reinit") {
		t.Errorf("OTel-обработчик потерян после InitAndSetDefault: %q", otel.String())
	}
}

func TestSetOTelHandler_AppliesLogLevel(t *testing.T) {
	stdout := &bytes.Buffer{}
	stubLogger(t, slog.NewTextHandler(stdout, &slog.HandlerOptions{Level: slog.LevelWarn}), slog.LevelWarn)

	var otel bytes.Buffer

	SetOTelHandler(slog.NewTextHandler(&otel, nil))

	slog.Debug("debug must be dropped")
	slog.Warn("warn must pass")

	if strings.Contains(stdout.String(), "debug must be dropped") {
		t.Errorf("stdout не должен писать ниже уровня WARN: %q", stdout.String())
	}
	if strings.Contains(otel.String(), "debug must be dropped") {
		t.Errorf("OTel не должен писать ниже уровня WARN: %q", otel.String())
	}
	if !strings.Contains(stdout.String(), "warn must pass") {
		t.Errorf("stdout должен писать WARN: %q", stdout.String())
	}
	if !strings.Contains(otel.String(), "warn must pass") {
		t.Errorf("OTel должен писать WARN: %q", otel.String())
	}
}
