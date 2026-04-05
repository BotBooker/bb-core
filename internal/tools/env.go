// Package tools предоставляет вспомогательные утилиты для работы с переменными окружения.
package tools

import (
	"os"
	"strings"
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
