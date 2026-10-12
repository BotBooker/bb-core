package logger

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestWithLevel(t *testing.T) {
	var buf bytes.Buffer

	handler := withLevel(slog.NewTextHandler(&buf, nil), slog.LevelWarn)

	if handler.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("Enabled(INFO) должен вернуть false при уровне WARN")
	}
	if !handler.Enabled(context.Background(), slog.LevelError) {
		t.Error("Enabled(ERROR) должен вернуть true при уровне WARN")
	}

	// Handle в обход Enabled тоже не должен писать запись ниже уровня.
	drop := slog.NewRecord(time.Now(), slog.LevelInfo, "drop", 0)
	if err := handler.Handle(context.Background(), drop); err != nil {
		t.Fatalf("Handle() вернул ошибку: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("запись ниже уровня WARN не должна попадать в вывод: %q", buf.String())
	}

	pass := slog.NewRecord(time.Now(), slog.LevelWarn, "pass", 0)
	if err := handler.Handle(context.Background(), pass); err != nil {
		t.Fatalf("Handle() вернул ошибку: %v", err)
	}
	if !strings.Contains(buf.String(), "pass") {
		t.Errorf("запись уровня WARN должна попадать в вывод: %q", buf.String())
	}
}

func TestWithLevel_WithAttrsAndWithGroup(t *testing.T) {
	var buf bytes.Buffer

	handler := withLevel(slog.NewTextHandler(&buf, nil), slog.LevelInfo).
		WithAttrs([]slog.Attr{slog.String("component", "test")}).
		WithGroup("g")

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	record.AddAttrs(slog.String("key", "value"))
	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle() вернул ошибку: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "component=test") {
		t.Errorf("атрибут не добавлен: %q", out)
	}
	if !strings.Contains(out, "g.key=value") {
		t.Errorf("группа не открыта: %q", out)
	}
}
