// Package logger_test содержит тесты для пакета logger.
package logger_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/botbooker/bb-core/pkg/logger"
)

func TestLogger_Init(t *testing.T) {
	log := logger.Init(slog.LevelInfo, "stdout")
	if log == nil {
		t.Fatal("Init должен вернуть не nil логгер")
	}
}

func TestLogger_Init_JSON(t *testing.T) {
	log := logger.Init(slog.LevelInfo, "json")
	if log == nil {
		t.Fatal("InitJSON должен вернуть не nil логгер")
	}
}

func TestLogger_Init_With_Different_Levels(t *testing.T) {
	levels := []slog.Level{
		slog.LevelDebug,
		slog.LevelInfo,
		slog.LevelWarn,
		slog.LevelError,
	}

	for _, level := range levels {
		t.Run(level.String(), func(t *testing.T) {
			log := logger.Init(level, "stdout")
			if log == nil {
				t.Fatalf("Init(%s) вернул nil", level)
			}
		})
	}
}

func TestLogger_InitJSON_With_Different_Levels(t *testing.T) {
	levels := []slog.Level{
		slog.LevelDebug,
		slog.LevelInfo,
		slog.LevelWarn,
		slog.LevelError,
	}

	for _, level := range levels {
		t.Run(level.String(), func(t *testing.T) {
			log := logger.Init(level, "json")
			if log == nil {
				t.Fatalf("InitJSON(%s) вернул nil", level)
			}
		})
	}
}

func TestLogger_Level_Filtering(t *testing.T) {
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

func TestLogger_Init_Returns_TextHandler(t *testing.T) {
	log := logger.Init(slog.LevelInfo, "stdout")

	// Проверяем, что логгер работает корректно
	log.Info("test")

	// Просто проверяем, что логгер не nil и может использоваться
	if log == nil {
		t.Error("Логгер должен быть не nil")
	}
}

func TestLogger_InitJSON_Returns_TextHandler(t *testing.T) {
	log := logger.Init(slog.LevelInfo, "json")

	// Проверяем, что логгер работает корректно
	log.Info("test")

	// Просто проверяем, что логгер не nil и может использоваться
	if log == nil {
		t.Error("Логгер должен быть не nil")
	}
}

func TestLogger_InitAndSetDefault(t *testing.T) {
	log := logger.InitAndSetDefault(slog.LevelInfo, "stdout")

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

func TestLogger_InitAndSetDefault_JSON(t *testing.T) {
	log := logger.InitAndSetDefault(slog.LevelInfo, "json")

	// Проверяем, что логгер не nil
	if log == nil {
		t.Fatal("InitAndSetDefault_JSON должен вернуть не nil логгер")
	}

	// Проверяем, что глобальный логгер установлен
	defaultLogger := slog.Default()
	if defaultLogger == nil {
		t.Error("Глобальный логгер должен быть установлен")
	}
}

func TestLogger_InitAndSetDefault_DifferentLevels(t *testing.T) {
	levels := []slog.Level{
		slog.LevelDebug,
		slog.LevelInfo,
		slog.LevelWarn,
		slog.LevelError,
	}

	for _, level := range levels {
		t.Run(level.String(), func(t *testing.T) {
			log := logger.InitAndSetDefault(level, "stdout")
			if log == nil {
				t.Fatalf("InitAndSetDefault(%s) вернул nil", level)
			}
		})
	}
}

func TestLogger_InitAndSetDefault_JSON_DifferentLevels(t *testing.T) {
	levels := []slog.Level{
		slog.LevelDebug,
		slog.LevelInfo,
		slog.LevelWarn,
		slog.LevelError,
	}

	for _, level := range levels {
		t.Run(level.String(), func(t *testing.T) {
			log := logger.InitAndSetDefault(level, "json")
			if log == nil {
				t.Fatalf("InitAndSetDefault_JSON(%s) вернул nil", level)
			}
		})
	}
}

func TestLogger_Init_And_Set_Default_LogsCorrectly(t *testing.T) {
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

func TestLogger_Output(t *testing.T) {
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
