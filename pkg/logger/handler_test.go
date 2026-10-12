package logger_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/botbooker/bb-core/pkg/logger"
)

// errorHandler — обработчик, который всегда возвращает ошибку. Нужен, чтобы
// проверить, что сбой одного обработчика не мешает остальным.
type errorHandler struct {
	err error
}

func (h errorHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h errorHandler) Handle(context.Context, slog.Record) error { return h.err }

func (h errorHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h errorHandler) WithGroup(string) slog.Handler { return h }

func TestMultiHandler_Handle_FansOut(t *testing.T) {
	var first, second bytes.Buffer

	handler := logger.NewMultiHandler(
		slog.NewTextHandler(&first, nil),
		slog.NewTextHandler(&second, nil),
	)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "fan out", 0)
	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle() вернул ошибку: %v", err)
	}

	if !strings.Contains(first.String(), "fan out") {
		t.Errorf("первый обработчик не получил запись: %q", first.String())
	}
	if !strings.Contains(second.String(), "fan out") {
		t.Errorf("второй обработчик не получил запись: %q", second.String())
	}
}

func TestMultiHandler_Handle_JoinsErrors(t *testing.T) {
	sentinel := errors.New("boom")

	var buf bytes.Buffer

	handler := logger.NewMultiHandler(
		errorHandler{err: sentinel},
		slog.NewTextHandler(&buf, nil),
	)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "still written", 0)
	if err := handler.Handle(context.Background(), record); !errors.Is(err, sentinel) {
		t.Fatalf("Handle() = %v, ожидалась ошибка %v", err, sentinel)
	}
	if !strings.Contains(buf.String(), "still written") {
		t.Error("ошибка первого обработчика не должна мешать остальным")
	}
}

func TestMultiHandler_Enabled(t *testing.T) {
	var buf bytes.Buffer

	handler := logger.NewMultiHandler(
		slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}),
	)

	if handler.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("Enabled(INFO) должен вернуть false при уровне WARN")
	}
	if !handler.Enabled(context.Background(), slog.LevelError) {
		t.Error("Enabled(ERROR) должен вернуть true при уровне WARN")
	}
}

func TestMultiHandler_Enabled_NoHandlers(t *testing.T) {
	handler := logger.NewMultiHandler()

	if handler.Enabled(context.Background(), slog.LevelError) {
		t.Error("пустой MultiHandler не должен быть Enabled")
	}
	if err := handler.Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelInfo, "x", 0)); err != nil {
		t.Errorf("пустой MultiHandler не должен возвращать ошибку: %v", err)
	}
}

func TestMultiHandler_SkipsNilHandlers(t *testing.T) {
	var buf bytes.Buffer

	handler := logger.NewMultiHandler(nil, slog.NewTextHandler(&buf, nil))

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "not nil", 0)
	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle() вернул ошибку: %v", err)
	}
	if !strings.Contains(buf.String(), "not nil") {
		t.Errorf("не nil обработчик не получил запись: %q", buf.String())
	}
}

func TestMultiHandler_WithAttrs(t *testing.T) {
	var buf bytes.Buffer

	handler := logger.NewMultiHandler(slog.NewTextHandler(&buf, nil)).
		WithAttrs([]slog.Attr{slog.String("component", "test")})

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "with attrs", 0)
	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle() вернул ошибку: %v", err)
	}
	if !strings.Contains(buf.String(), "component=test") {
		t.Errorf("атрибут не добавлен: %q", buf.String())
	}
}

func TestMultiHandler_WithGroup(t *testing.T) {
	var buf bytes.Buffer

	handler := logger.NewMultiHandler(slog.NewTextHandler(&buf, nil)).WithGroup("g")

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "with group", 0)
	record.AddAttrs(slog.String("key", "value"))
	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle() вернул ошибку: %v", err)
	}
	if !strings.Contains(buf.String(), "g.key=value") {
		t.Errorf("группа не открыта: %q", buf.String())
	}
}

func TestMultiHandler_Handle_RespectsHandlerLevels(t *testing.T) {
	var quiet, loud bytes.Buffer

	handler := logger.NewMultiHandler(
		slog.NewTextHandler(&quiet, &slog.HandlerOptions{Level: slog.LevelError}),
		slog.NewTextHandler(&loud, &slog.HandlerOptions{Level: slog.LevelDebug}),
	)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "info", 0)
	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle() вернул ошибку: %v", err)
	}

	if quiet.Len() != 0 {
		t.Errorf("обработчик уровня ERROR не должен писать INFO: %q", quiet.String())
	}
	if !strings.Contains(loud.String(), "info") {
		t.Errorf("обработчик уровня DEBUG должен писать INFO: %q", loud.String())
	}
}
