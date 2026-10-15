package tools

import (
	"strings"
	"unicode/utf8"
)

// SanitizeInputString очищает строку от спецсимволов, маскирует секреты и ограничивает длину.
func SanitizeInputString(value string, key ...string) string {
	// 1. Если ключ передан, проверяем на конфиденциальность
	if len(key) > 0 && key[0] != "" {
		keyLower := strings.ToLower(key[0])

		// Проверка на пароли, токены и секреты
		if strings.Contains(keyLower, "password") ||
			strings.Contains(keyLower, "secret") ||
			strings.Contains(keyLower, "token") ||
			strings.Contains(keyLower, "cvv") ||
			strings.Contains(keyLower, "auth") {
			return "***"
		}

		// Проверка и маскирование Email
		if strings.Contains(keyLower, "email") && strings.Contains(value, "@") {
			parts := strings.Split(value, "@")
			if len(parts) == 2 && len(parts[0]) > 0 {
				return string(parts[0][0]) + "***@" + parts[1]
			}
			return "***"
		}
	}

	// 2. Защита от Log Injection: заменяем переносы строк на пробелы
	replacer := strings.NewReplacer("\n", " ", "\r", " ")
	sanitized := replacer.Replace(value)

	// 3. Ограничение длины в байтах (Защита от DoS)
	const maxLogLength = 500
	if len(sanitized) > maxLogLength {
		// Сдвигаем границу назад, чтобы не разорвать многобайтовый UTF-8 символ:
		// иначе в лог попадёт невалидная последовательность байт.
		end := maxLogLength
		for end > 0 && !utf8.RuneStart(sanitized[end]) {
			end--
		}
		return sanitized[:end] + "... [truncated]"
	}

	return sanitized
}
