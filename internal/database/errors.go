// Package database описывает единый контракт подключения к базе данных,
// который реализуют драйверы internal/database/postgres и internal/database/sqlite,
// дополняет его менеджером транзакций и переводит ошибки драйверов в доменные.
package database

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	sqlite "modernc.org/sqlite"
)

// Коды ошибок драйверов, которые означают конфликт уникальности.
const (
	// pgUniqueViolation — SQLSTATE нарушения уникальности в PostgreSQL.
	pgUniqueViolation = "23505"
	// sqliteConstraintUnique — SQLITE_CONSTRAINT_UNIQUE.
	sqliteConstraintUnique = 2067
	// sqliteConstraintPrimaryKey — SQLITE_CONSTRAINT_PRIMARYKEY.
	sqliteConstraintPrimaryKey = 1555
)

// Доменные ошибки доступа к данным.
// Проверять их нужно через errors.Is: MapError добавляет к ним контекст драйвера.
var (
	// ErrNotFound — запись не найдена.
	ErrNotFound = errors.New("record not found")
	// ErrDuplicate — нарушено ограничение уникальности (например, email уже занят).
	ErrDuplicate = errors.New("duplicate record")
)

// MapError переводит ошибки драйверов в доменные:
//
//	sql.ErrNoRows                             → ErrNotFound;
//	PostgreSQL 23505, SQLite 2067/1555        → ErrDuplicate.
//
// Остальные ошибки возвращаются как есть: их должен увидеть тот, кто знает
// контекст (транспорт отдаёт их как internal).
func MapError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("%w: %w", ErrNotFound, err)
	case isDuplicate(err):
		return fmt.Errorf("%w: %w", ErrDuplicate, err)
	default:
		return err
	}
}

// isDuplicate сообщает, что ошибка — нарушение уникальности или первичного ключа.
func isDuplicate(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgUniqueViolation
	}

	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		code := sqliteErr.Code()

		return code == sqliteConstraintUnique || code == sqliteConstraintPrimaryKey
	}

	return false
}
