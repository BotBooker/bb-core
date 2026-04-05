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
