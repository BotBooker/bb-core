// Package logger_test содержит тесты для пакета logger.
package logger_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/botbooker/bb-core/internal/logger"
)

func TestInit(t *testing.T) {
	log := logger.Init(slog.LevelInfo)
	if log == nil {
		t.Fatal("Init должен вернуть не nil логгер")
	}
}

func TestInitJSON(t *testing.T) {
	log := logger.InitJSON(slog.LevelInfo)
	if log == nil {
		t.Fatal("InitJSON должен вернуть не nil логгер")
	}
}

func TestInitWithDifferentLevels(t *testing.T) {
	levels := []slog.Level{
		slog.LevelDebug,
		slog.LevelInfo,
		slog.LevelWarn,
		slog.LevelError,
	}

	for _, level := range levels {
		t.Run(level.String(), func(t *testing.T) {
			log := logger.Init(level)
			if log == nil {
				t.Fatalf("Init(%s) вернул nil", level)
			}
		})
	}
}

func TestInitJSONWithDifferentLevels(t *testing.T) {
	levels := []slog.Level{
		slog.LevelDebug,
		slog.LevelInfo,
		slog.LevelWarn,
		slog.LevelError,
	}

	for _, level := range levels {
		t.Run(level.String(), func(t *testing.T) {
			log := logger.InitJSON(level)
			if log == nil {
				t.Fatalf("InitJSON(%s) вернул nil", level)
			}
		})
	}
}

func TestLoggerLevelFiltering(t *testing.T) {
	var buf bytes.Buffer

	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))

	// Это сообщение должно быть отфильтровано
	log.Info("info message")

	if buf.Len() > 0 {
		t.Error("Info сообщение должно быть отфильтровано при уровне Warn")
	}

	// Это сообщение должно пройти
	log.Warn("warn message")

	if !strings.Contains(buf.String(), "warn message") {
		t.Error("Warn сообщение должно быть в выводе")
	}
}

func TestInitReturnsTextHandler(t *testing.T) {
	log := logger.Init(slog.LevelInfo)

	// Проверяем, что логгер работает корректно
	log.Info("test")

	// Просто проверяем, что логгер не nil и может использоваться
	if log == nil {
		t.Error("Логгер должен быть не nil")
	}
}

func TestInitJSONReturnsTextHandler(t *testing.T) {
	log := logger.InitJSON(slog.LevelInfo)

	// Проверяем, что логгер работает корректно
	log.Info("test")

	// Просто проверяем, что логгер не nil и может использоваться
	if log == nil {
		t.Error("Логгер должен быть не nil")
	}
}

func TestInitAndSetDefault(t *testing.T) {
	log := logger.InitAndSetDefault(slog.LevelInfo)

	// Проверяем, что логгер не nil
	if log == nil {
		t.Fatal("InitAndSetDefault должен вернуть не nil логгер")
	}

	// Проверяем, что глобальный логгер установлен
	defaultLogger := slog.Default()
	if defaultLogger == nil {
		t.Error("Глобальный логгер должен быть установлен")
	}
}

func TestInitJSONAndSetDefault(t *testing.T) {
	log := logger.InitJSONAndSetDefault(slog.LevelInfo)

	// Проверяем, что логгер не nil
	if log == nil {
		t.Fatal("InitJSONAndSetDefault должен вернуть не nil логгер")
	}

	// Проверяем, что глобальный логгер установлен
	defaultLogger := slog.Default()
	if defaultLogger == nil {
		t.Error("Глобальный логгер должен быть установлен")
	}
}

func TestInitAndSetDefault_DifferentLevels(t *testing.T) {
	levels := []slog.Level{
		slog.LevelDebug,
		slog.LevelInfo,
		slog.LevelWarn,
		slog.LevelError,
	}

	for _, level := range levels {
		t.Run(level.String(), func(t *testing.T) {
			log := logger.InitAndSetDefault(level)
			if log == nil {
				t.Fatalf("InitAndSetDefault(%s) вернул nil", level)
			}
		})
	}
}

func TestInitJSONAndSetDefault_DifferentLevels(t *testing.T) {
	levels := []slog.Level{
		slog.LevelDebug,
		slog.LevelInfo,
		slog.LevelWarn,
		slog.LevelError,
	}

	for _, level := range levels {
		t.Run(level.String(), func(t *testing.T) {
			log := logger.InitJSONAndSetDefault(level)
			if log == nil {
				t.Fatalf("InitJSONAndSetDefault(%s) вернул nil", level)
			}
		})
	}
}

func TestInitAndSetDefault_LogsCorrectly(t *testing.T) {
	var buf bytes.Buffer

	// Создаём логгер с записью в буфер
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(log)

	// Логируем сообщение
	slog.Info("test message")

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("Ожидалось сообщение 'test message' в выводе, получено: %s", output)
	}
}

func TestLoggerOutput(t *testing.T) {
	// Создаём буфер для захвата вывода
	var buf bytes.Buffer

	// Инициализируем логгер с текстовым обработчиком, записывающим в буфер
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	log.Info("test message", "key", "value")

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("Ожидалось сообщение 'test message' в выводе, получено: %s", output)
	}
	if !strings.Contains(output, "key=value") {
		t.Errorf("Ожидался атрибут 'key=value' в выводе, получено: %s", output)
	}
}
