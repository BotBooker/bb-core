// Package tools предоставляет вспомогательные утилиты для работы с переменными окружения.
package tools

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// GetEnvOrDefault возвращает значение переменной окружения или значение по умолчанию.
func GetEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

// GetEnvList возвращает список строк из переменной окружения, разделённых запятыми.
// Если переменная не установлена или пуста, возвращает значение по умолчанию.
func GetEnvList(key string, defaultValue []string) []string {
	if v := os.Getenv(key); v != "" {
		parts := strings.Split(v, ",")
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				result = append(result, trimmed)
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return defaultValue
}

// GetEnvOrDefaultToBool возвращает значение переменной окружения,
// приведённое к bool, или значение по умолчанию.
//
// Значение разбирает strconv.ParseBool (1, t, true, 0, f, false и их регистры);
// неразобранное значение не роняет приложение — используется значение по умолчанию.
func GetEnvOrDefaultToBool(key string, defaultValue bool) bool {
	if v := os.Getenv(key); v != "" {
		val, err := strconv.ParseBool(v)
		if err != nil {
			slog.Error("failed to parse environment variable", "key", key, "value", v, "error", err)
			return defaultValue
		}
		return val
	}
	return defaultValue
}

// GetEnvOrDefaultToInt32 возвращает значение переменной окружения,
// приведённое к int32, или значение по умолчанию.
// Если значение не является числом, возвращается значение по умолчанию.
func GetEnvOrDefaultToInt32(key string, defaultValue int32) int32 {
	if v := os.Getenv(key); v != "" {
		val, err := strconv.ParseInt(v, 10, 32)
		if err != nil {
			slog.Error("failed to parse environment variable", "key", key, "value", v, "error", err)
			return defaultValue
		}
		return int32(val)
	}
	return defaultValue
}

// GetEnvOrDefaultToDuration возвращает значение переменной окружения,
// приведённое к time.Duration (например, "720h", "30m", "45s"),
// или значение по умолчанию.
//
// Значение разбирает time.ParseDuration; неразобранное значение не роняет
// приложение — используется значение по умолчанию (и пишется ошибка в лог).
func GetEnvOrDefaultToDuration(key string, defaultValue time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		val, err := time.ParseDuration(v)
		if err != nil {
			slog.Error("failed to parse environment variable", "key", key, "value", v, "error", err)
			return defaultValue
		}
		return val
	}
	return defaultValue
}
