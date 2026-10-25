package repository

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/domain"
	"github.com/botbooker/bb-core/internal/model"
)

// dbConn — минимум, который нужен репозиторию: пул соединений или транзакция
// Unit of Work. Ему удовлетворяют и database.DB, и *sql.Tx.
type dbConn interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// conn выбирает соединение для запроса: транзакцию из контекста, если репозиторий
// вызван внутри UnitOfWork, иначе — пул.
//
// Без этого правила запросы уходили бы мимо транзакции: use case откатил бы
// изменения, а часть из них уже была бы зафиксирована отдельными соединениями.
func conn(ctx context.Context, db dbConn) dbConn {
	if tx, ok := database.GetTx(ctx); ok {
		return tx
	}

	return db
}

// codec переводит значения модели в вид, который понимает конкретный диалект,
// и подставляет плейсхолдеры: PostgreSQL ждёт $1, $2, ... и нативные типы,
// SQLite — "?" и строки RFC3339 (типа timestamp у него нет).
//
// Репозитории встраивают codec в себя, поэтому запросы и аргументы пишутся
// один раз, а различия диалектов остаются в этих нескольких методах.
type codec struct {
	dialect string
}

// rebind заменяет плейсхолдеры "?" на "$1", "$2", ... для PostgreSQL.
func (c codec) rebind(query string) string {
	if c.dialect != database.DialectPostgres {
		return query
	}

	var builder strings.Builder

	for index, part := range strings.Split(query, "?") {
		if index > 0 {
			fmt.Fprintf(&builder, "$%d", index)
		}

		builder.WriteString(part)
	}

	return builder.String()
}

// timeArg кодирует момент времени: PostgreSQL принимает time.Time (TIMESTAMPTZ),
// SQLite хранит строку RFC3339 в UTC — формат задаёт репозиторий.
func (c codec) timeArg(ts time.Time) any {
	if c.dialect == database.DialectPostgres {
		return ts.UTC()
	}

	return ts.UTC().Format(time.RFC3339Nano)
}

// nullTimeArg кодирует необязательный момент: nil означает NULL.
func (c codec) nullTimeArg(ts *time.Time) any {
	if ts == nil {
		return nil
	}

	return c.timeArg(*ts)
}

// dateArg кодирует календарную дату: PostgreSQL принимает time.Time для DATE,
// SQLite — строку YYYY-MM-DD.
func (c codec) dateArg(date *model.Date) any {
	if date == nil {
		return nil
	}

	if c.dialect == database.DialectPostgres {
		return date.Time()
	}

	return date.String()
}

// jsonArg кодирует JSON-значение (metadata тенанта, provider_meta учётных
// данных): пустое значение означает NULL, иначе передаётся JSON-текст —
// PostgreSQL приводит его к jsonb по типу колонки, SQLite хранит строкой.
func (c codec) jsonArg(data []byte) any {
	if len(data) == 0 {
		return nil
	}

	return string(data)
}

// requireAffected превращает «0 изменённых строк» в ErrNotFound: так UPDATE и
// DELETE отсутствующей строки дают 404, а не выглядят как успех.
func requireAffected(result sql.Result, action string) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: rows affected: %w", action, err)
	}

	if affected == 0 {
		return fmt.Errorf("%w: %s affected 0 rows", domain.ErrNotFound, action)
	}

	return nil
}

// nullTime принимает время в любом виде, который отдают диалекты:
// time.Time из PostgreSQL и строку RFC3339 из SQLite.
type nullTime struct {
	ts    time.Time
	valid bool
}

// Scan реализует sql.Scanner.
func (n *nullTime) Scan(src any) error {
	switch value := src.(type) {
	case nil:
		*n = nullTime{}
	case time.Time:
		n.ts, n.valid = value.UTC(), true
	case string:
		return n.parse(value)
	case []byte:
		return n.parse(string(value))
	default:
		return fmt.Errorf("unsupported time value %T", src)
	}

	return nil
}

// parse разбирает строку RFC3339 — так время хранит SQLite.
func (n *nullTime) parse(value string) error {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return fmt.Errorf("parse time %q: %w", value, err)
	}

	n.ts, n.valid = parsed.UTC(), true

	return nil
}

// time возвращает разобранное время: NULL превращается в нулевой момент.
func (n nullTime) time() time.Time {
	if !n.valid {
		return time.Time{}
	}

	return n.ts
}

// ptr возвращает указатель на время: NULL превращается в nil.
func (n nullTime) ptr() *time.Time {
	if !n.valid {
		return nil
	}

	ts := n.ts

	return &ts
}

// nullBytes принимает JSON-значение в видах, которые отдают диалекты:
// []byte из PostgreSQL (JSONB) и строку из SQLite (TEXT).
type nullBytes struct {
	value []byte
}

// Scan реализует sql.Scanner.
func (n *nullBytes) Scan(src any) error {
	switch value := src.(type) {
	case nil:
		n.value = nil
	case []byte:
		n.value = bytes.Clone(value)
	case string:
		n.value = []byte(value)
	default:
		return fmt.Errorf("unsupported json value %T", src)
	}

	return nil
}
