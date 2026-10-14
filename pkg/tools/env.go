// Package tools предоставляет вспомогательные утилиты для работы с переменными окружения.
package tools

import (
	"fmt"
	"log/slog"
	"math"
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

// dayDuration — сутки в time.Duration.
const dayDuration = 24 * time.Hour

// maxDaysDuration — наибольшее число суток, которое помещается в time.Duration;
// нужно, чтобы разбор вида "1e10d" упирался в ошибку, а не в переполнение.
const maxDaysDuration = float64(math.MaxInt64) / float64(dayDuration)

// parseDurationWithDays разбирает значение как time.ParseDuration ("720h",
// "30m", "45s") и дополнительно поддерживает суффикс «d» — сутки, которого в
// стандартном разборе нет: "90d" = 2160h, "1.5d" = 36h, "-1d" = -24h.
// Значения с другими единицами остаются на усмотрение time.ParseDuration.
func parseDurationWithDays(value string) (time.Duration, error) {
	days, ok := strings.CutSuffix(value, "d")
	if !ok {
		return time.ParseDuration(value)
	}

	amount, err := strconv.ParseFloat(days, 64)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(amount) || math.Abs(amount) > maxDaysDuration {
		return 0, fmt.Errorf("duration %q is out of range", value)
	}

	return time.Duration(amount * float64(dayDuration)), nil
}

// GetEnvOrDefaultToDuration возвращает значение переменной окружения,
// приведённое к time.Duration (например, "720h", "30m", "45s", "90d"),
// или значение по умолчанию.
//
// Значение разбирает time.ParseDuration плюс суффикс «d» — сутки ("90d",
// "1.5d"); неразобранное значение не роняет приложение — используется значение
// по умолчанию (и пишется ошибка в лог).
func GetEnvOrDefaultToDuration(key string, defaultValue time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		val, err := parseDurationWithDays(v)
		if err != nil {
			slog.Error("failed to parse environment variable", "key", key, "value", v, "error", err)
			return defaultValue
		}
		return val
	}
	return defaultValue
}
