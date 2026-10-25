package model

import (
	"crypto/rand"
	"fmt"
	"strings"
)

// idLength — длина канонического UUID (8-4-4-4-12).
const idLength = 36

// uuidHyphens — позиции дефисов в каноническом представлении UUID.
var uuidHyphens = map[int]struct{}{8: {}, 13: {}, 18: {}, 23: {}}

// NewID возвращает случайный идентификатор — UUID версии 4 в каноническом виде.
//
// Генерация на stdlib (crypto/rand): домену запрещены сторонние библиотеки
// (spec §1.1). Формат совпадает с типом uuid в PostgreSQL, поэтому один и тот
// же идентификатор работает в обоих диалектах.
func NewID() string {
	var value [16]byte

	if _, err := rand.Read(value[:]); err != nil {
		// crypto/rand.Read не возвращает ошибку с Go 1.24: сбой означает, что
		// система не может выдать случайность, и продолжать работу нельзя.
		panic(fmt.Errorf("generate id: %w", err))
	}

	value[6] = (value[6] & 0x0f) | 0x40 // версия 4
	value[8] = (value[8] & 0x3f) | 0x80 // вариант RFC 4122

	return fmt.Sprintf("%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}

// ValidateUUID проверяет, что строка — валидный UUID (любой версии).
//
// Экспортируется для слоёв, которые принимают идентификатор от пользователя:
// так невалидный id отклоняется до запроса в базу, где PostgreSQL ответил бы
// ошибкой приведения типа вместо понятного 400.
func ValidateUUID(value string) error { return validateUUID(value) }

// validateUUID проверяет каноническую форму UUID: 36 символов, дефисы на
// позициях 8/13/18/23, остальные — шестнадцатеричные цифры. Версия не
// ограничивается: proto-правило uuid её не проверяет, а создаём мы UUIDv4.
// В PostgreSQL тот же формат проверяет тип колонки id (uuid).
func validateUUID(value string) error {
	if len(value) != idLength || !isUUIDHex(value) {
		return fmt.Errorf("%w: %q", ErrInvalidUUID, value)
	}

	return nil
}

// isUUIDHex проверяет формат канонического UUID без разбора версии.
func isUUIDHex(value string) bool {
	for index := range len(value) {
		if _, ok := uuidHyphens[index]; ok {
			if value[index] != '-' {
				return false
			}

			continue
		}

		if !isHexDigit(value[index]) {
			return false
		}
	}

	return true
}

// isHexDigit сообщает, что байт — шестнадцатеричная цифра.
func isHexDigit(char byte) bool {
	switch {
	case char >= '0' && char <= '9':
		return true
	case char >= 'a' && char <= 'f':
		return true
	case char >= 'A' && char <= 'F':
		return true
	default:
		return false
	}
}

// sanitizeForError готовит пользовательское значение к тексту ошибки: убирает
// управляющие символы (переводы строк и т.п.) и ограничивает длину.
//
// Текст ошибки валидации уходит клиенту и в логи, поэтому значение из запроса
// нельзя вставлять как есть (spec §2.1). Домен делает это на stdlib: сторонние
// пакеты ему недоступны.
func sanitizeForError(value string) string {
	const limit = 64

	sanitized := strings.Map(func(char rune) rune {
		if char < ' ' || char == 0x7f {
			return ' '
		}

		return char
	}, strings.TrimSpace(value))

	if runes := []rune(sanitized); len(runes) > limit {
		return string(runes[:limit]) + "..."
	}

	return sanitized
}
