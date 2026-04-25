// Package logger предоставляет утилиты для инициализации и настройки логирования.
//
// Основные функции:
//   - Init: инициализирует slog логгер с заданными параметрами.
package logger

import (
	"log/slog"
	"os"
)

// Init инициализирует slog логгер с текстовым обработчиком.
//
// Параметры:
//   - level: уровень логирования (например, slog.LevelInfo, slog.LevelDebug).
//
// Возвращает:
//   - *slog.Logger: настроенный экземпляр логгера.
//
// Пример использования:
//
//	log := logger.Init(slog.LevelInfo)
//	slog.SetDefault(log)
func Init(level slog.Level) *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
}

// Init инициализирует slog логгер с JSON обработчиком.
func InitJSON(level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
}

// InitAndSetDefault инициализирует логгер и устанавливает его как глобальный по умолчанию.
//
// Параметры:
//   - level: уровень логирования (например, slog.LevelInfo, slog.LevelDebug).
//
// Возвращает:
//   - *slog.Logger: настроенный экземпляр логгера.
//
// Пример использования:
//
//	log := logger.InitAndSetDefault(slog.LevelInfo)
func InitAndSetDefault(level slog.Level) *slog.Logger {
	log := Init(level)
	slog.SetDefault(log)
	return log
}

func InitJSONAndSetDefault(level slog.Level) *slog.Logger {
	log := InitJSON(level)
	slog.SetDefault(log)
	return log
}

// parseLogLevel parses a slog.Level from a string.
// Returns the default level if the input is invalid.
func ParseLogLevel(level string, defaultLevel slog.Level) slog.Level {
	switch level {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO":
		return slog.LevelInfo
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return defaultLevel
	}
}
