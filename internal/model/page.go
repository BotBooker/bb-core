package model

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
)

// Ограничения пагинации: одинаковы для всех списочных ручек.
const (
	// DefaultPageSize — размер страницы, если клиент его не указал.
	DefaultPageSize = 50
	// MaxPageSize — максимальный размер страницы: защита от выгрузки всей базы.
	MaxPageSize = 100
)

// ErrInvalidPageToken — токен следующей страницы не удалось разобрать.
// Токен непрозрачен для клиента, поэтому любое отклонение от формата — ошибка
// запроса, а не «начать с первой страницы»: иначе клиент зациклится.
var ErrInvalidPageToken = errors.New("page token is invalid")

// Page — окно выборки: смещение и размер страницы.
type Page struct {
	// Limit — сколько строк вернуть.
	Limit int
	// Offset — сколько строк пропустить.
	Offset int
}

// NewPage собирает окно выборки по размеру страницы и токену следующей.
//
// Пустой токен означает первую страницу. Размер 0 заменяется значением по
// умолчанию, слишком большой — максимальным: клиент не должен уметь выгрузить
// всю таблицу одним запросом.
func NewPage(size int, token string) (Page, error) {
	limit := size
	if limit <= 0 {
		limit = DefaultPageSize
	}

	if limit > MaxPageSize {
		limit = MaxPageSize
	}

	offset := 0

	if token != "" {
		parsed, err := decodePageToken(token)
		if err != nil {
			return Page{}, err
		}

		offset = parsed
	}

	return Page{Limit: limit, Offset: offset}, nil
}

// Next возвращает токен следующей страницы: пустой, если строк больше нет.
//
// hasMore вычисляет вызывающий: он запрашивает limit+1 строк и смотрит,
// осталась ли лишняя. Токен — смещение в base64url, поэтому клиент не должен
// полагаться на его содержимое.
func (p Page) Next(hasMore bool) string {
	if !hasMore {
		return ""
	}

	return encodePageToken(p.Offset + p.Limit)
}

// decodePageToken разбирает токен следующей страницы.
func decodePageToken(token string) (int, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", ErrInvalidPageToken, err)
	}

	offset, err := strconv.Atoi(string(raw))
	if err != nil || offset < 0 {
		return 0, fmt.Errorf("%w: %q", ErrInvalidPageToken, token)
	}

	return offset, nil
}

// encodePageToken кодирует смещение в непрозрачный токен.
func encodePageToken(offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(offset)))
}
