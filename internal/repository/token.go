package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/model"
)

// TokenDB — интерфейс базы данных, который нужен TokenRepo.
//
// Методы совпадают с методами database.DB, но объявлены узко: репозиторию
// нужны только запросы.
type TokenDB interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// TokenRepo — интерфейс репозитория токенов доступа (сессий).
type TokenRepo interface {
	// Create вставляет выданный токен; конфликт уникальности — database.ErrDuplicate.
	Create(ctx context.Context, token *model.Token) error
	// GetByID возвращает токен по идентификатору (jti) или database.ErrNotFound.
	GetByID(ctx context.Context, id string) (*model.Token, error)
	// TouchLastUsed отмечает, что токен использовали: аудит последней активности.
	TouchLastUsed(ctx context.Context, id string, at time.Time) error
	// Revoke помечает токен отозванным — это и есть выход из системы.
	// Повторный отзыв не ошибка: клиент мог потерять ответ после первого выхода.
	Revoke(ctx context.Context, id string, at time.Time) error
	// DeleteExpired удаляет записи, истёкшие до указанного момента, и возвращает
	// число удалённых: таблица сессий не должна расти бесконечно.
	DeleteExpired(ctx context.Context, user_id string, before time.Time) (int64, error)
}

// authColumns — колонки user_tokens в том порядке, в котором их читает scanToken.
// Порядок должен совпадать: рассинхрон ловят тесты репозитория.
const authColumns = "id, user_id, tenant_id, role_id, issued_at, expires_at, revoked_at, last_used_at"

// tokenRepo — конкретная реализация, скрыта от внешних пакетов.
type tokenRepo struct {
	db TokenDB
	codec
}

// NewTokenRepo создаёт репозиторий токенов.
func NewTokenRepo(db TokenDB, dialect string) TokenRepo {
	slog.Debug("token repository created", "dialect", dialect)

	return &tokenRepo{db: db, codec: codec{dialect: dialect}}
}

// Create вставляет выданную сессию.
//
// Роль записывается ссылкой на справочник roles (role_id): код роли переводит
// модель (model.Role.ID). Так роль токена и роль членства в тенанте смотрят
// на один список значений.
func (r *tokenRepo) Create(ctx context.Context, token *model.Token) error {
	const query = `INSERT INTO user_tokens (id, user_id, tenant_id, role_id, issued_at,` +
		` expires_at, revoked_at, last_used_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	roleID, err := token.Role.ID()
	if err != nil {
		return fmt.Errorf("insert token: %w", err)
	}

	_, err = r.db.ExecContext(ctx, r.rebind(query),
		token.ID,
		token.UserID,
		token.TenantID,
		roleID,
		r.timeArg(token.IssuedAt),
		r.timeArg(token.ExpiresAt),
		r.nullTimeArg(token.RevokedAt),
		r.nullTimeArg(token.LastUsedAt),
	)
	if err != nil {
		return fmt.Errorf("insert token: %w", database.MapError(err))
	}

	return nil
}

// GetByID возвращает токен по идентификатору (jti из JWT).
//
// Проверять подпись токена по базе не нужно: запись нужна, чтобы узнать, не
// отозван ли токен, — это единственное состояние сессии, которого нет в самом JWT.
func (r *tokenRepo) GetByID(ctx context.Context, id string) (*model.Token, error) {
	const query = "SELECT " + authColumns + " FROM user_tokens WHERE id = ?"

	token, err := scanToken(r.db.QueryRowContext(ctx, r.rebind(query), id))
	if err != nil {
		return nil, fmt.Errorf("select token: %w", database.MapError(err))
	}

	return token, nil
}

// TouchLastUsed отмечает использование токена.
func (r *tokenRepo) TouchLastUsed(ctx context.Context, id string, at time.Time) error {
	const query = "UPDATE user_tokens SET last_used_at = ? WHERE id = ?"

	result, err := r.db.ExecContext(ctx, r.rebind(query), r.timeArg(at), id)
	if err != nil {
		return fmt.Errorf("touch token: %w", database.MapError(err))
	}

	return requireAffected(result, "touch token")
}

// Revoke отзывает токен: запись остаётся, поэтому повторный вход с тем же
// токеном отклоняется до истечения срока его жизни.
//
// COALESCE сохраняет время первого выхода: повторный запрос на выход ничего не
// меняет, но и ошибкой не считается — клиент мог потерять ответ на первый, а
// для аудита важно, когда сессия закончилась в действительности.
func (r *tokenRepo) Revoke(ctx context.Context, id string, at time.Time) error {
	const query = "UPDATE user_tokens SET revoked_at = COALESCE(revoked_at, ?) WHERE id = ?"

	result, err := r.db.ExecContext(ctx, r.rebind(query), r.timeArg(at), id)
	if err != nil {
		return fmt.Errorf("revoke token: %w", database.MapError(err))
	}

	return requireAffected(result, "revoke token")
}

// DeleteExpired удаляет просроченные токены: у сессии с истёкшим сроком смысла
// уже нет, а таблица растёт с каждым входом в систему.
func (r *tokenRepo) DeleteExpired(ctx context.Context, user_id string, before time.Time) (int64, error) {
	const query = "DELETE FROM user_tokens WHERE user_id = ? AND expires_at < ?"

	result, err := r.db.ExecContext(ctx, r.rebind(query), user_id, r.timeArg(before))
	if err != nil {
		return 0, fmt.Errorf("delete expired tokens: %w", database.MapError(err))
	}

	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("delete expired tokens: rows affected: %w", err)
	}

	return deleted, nil
}

// scanToken читает строку user_tokens в доменную модель.
//
// Время разбирает nullTime: PostgreSQL отдаёт time.Time, SQLite — строку.
// Роль приходит числовым role_id: код роли даёт model.ParseRoleID, поэтому
// неизвестный id (справочник и код разошлись) — ошибка чтения, а не пустая роль.
func scanToken(row *sql.Row) (*model.Token, error) {
	var (
		token                 model.Token
		roleID                int16
		issuedAt, expiresAt   nullTime
		revokedAt, lastUsedAt nullTime
	)

	err := row.Scan(
		&token.ID,
		&token.UserID,
		&token.TenantID,
		&roleID,
		&issuedAt,
		&expiresAt,
		&revokedAt,
		&lastUsedAt,
	)
	if err != nil {
		return nil, err
	}

	role, err := model.ParseRoleID(roleID)
	if err != nil {
		return nil, fmt.Errorf("scan token: %w", err)
	}

	token.Role = role
	token.IssuedAt = issuedAt.time()
	token.ExpiresAt = expiresAt.time()
	token.RevokedAt = revokedAt.ptr()
	token.LastUsedAt = lastUsedAt.ptr()

	return &token, nil
}
