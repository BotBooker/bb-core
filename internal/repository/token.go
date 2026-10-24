package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/domain"
	"github.com/botbooker/bb-core/internal/model"
	"github.com/botbooker/bb-core/pkg/observability"
)

// TokenDB — интерфейс базы данных, который нужен TokenRepo.
//
// Набор методов совпадает с dbConn: внутри UnitOfWork запрос уходит в
// транзакцию из контекста.
type TokenDB interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// TokenRepo — алиас доменного порта TokenRepository: репозиторий реализует
// контракт сессий, объявленный в internal/domain.
type TokenRepo = domain.TokenRepository

// authColumns — колонки user_tokens в том порядке, в котором их читает scanToken.
// Порядок должен совпадать: рассинхрон ловят тесты репозитория.
const authColumns = "id, user_id, tenant_id, role_id, issued_at, expires_at, revoked_at, last_used_at"

// tokenRepo — конкретная реализация, скрыта от внешних пакетов.
type tokenRepo struct {
	db TokenDB
	codec
}

// NewTokenRepo создаёт репозиторий токенов.
func NewTokenRepo(ctx context.Context, db TokenDB, dialect string) TokenRepo {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "NewTokenRepo")
	defer spanEnd()

	slog.DebugContext(traceCtx, "token repository created", "dialect", dialect)

	return &tokenRepo{db: db, codec: codec{dialect: dialect}}
}

// Create вставляет выданную сессию.
//
// Роль записывается ссылкой на справочник roles (role_id): код роли переводит
// модель (model.Role.ID). Так роль токена и роль членства в тенанте смотрят
// на один список значений.
func (r *tokenRepo) Create(ctx context.Context, token *model.Token) error {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "Create")
	defer spanEnd()

	const query = `INSERT INTO user_tokens (id, user_id, tenant_id, role_id, issued_at,` +
		` expires_at, revoked_at, last_used_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	roleID, err := token.Role.ID()
	if err != nil {
		return fmt.Errorf("insert token: %w", err)
	}

	if _, err = conn(traceCtx, r.db).ExecContext(traceCtx, r.rebind(query),
		token.ID,
		token.UserID,
		token.TenantID,
		roleID,
		r.timeArg(token.IssuedAt),
		r.timeArg(token.ExpiresAt),
		r.nullTimeArg(token.RevokedAt),
		r.nullTimeArg(token.LastUsedAt),
	); err != nil {
		return fmt.Errorf("insert token: %w", database.MapError(err))
	}

	return nil
}

// GetByID возвращает токен по идентификатору (jti из JWT).
//
// Проверять подпись токена по базе не нужно: запись нужна, чтобы узнать, не
// отозван ли токен, — это единственное состояние сессии, которого нет в самом JWT.
func (r *tokenRepo) GetByID(ctx context.Context, id string) (*model.Token, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "GetByID")
	defer spanEnd()

	const query = "SELECT " + authColumns + " FROM user_tokens WHERE id = ?"

	token, err := scanToken(conn(traceCtx, r.db).QueryRowContext(traceCtx, r.rebind(query), id))
	if err != nil {
		return nil, fmt.Errorf("select token: %w", database.MapError(err))
	}

	return token, nil
}

// TouchLastUsed отмечает использование токена.
func (r *tokenRepo) TouchLastUsed(ctx context.Context, id string, at time.Time) error {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "TouchLastUsed")
	defer spanEnd()

	const query = "UPDATE user_tokens SET last_used_at = ? WHERE id = ?"

	result, err := conn(traceCtx, r.db).ExecContext(traceCtx, r.rebind(query), r.timeArg(at), id)
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
	traceCtx, spanEnd := observability.TraceSpan(ctx, "Revoke")
	defer spanEnd()

	const query = "UPDATE user_tokens SET revoked_at = COALESCE(revoked_at, ?) WHERE id = ?"

	result, err := conn(traceCtx, r.db).ExecContext(traceCtx, r.rebind(query), r.timeArg(at), id)
	if err != nil {
		return fmt.Errorf("revoke token: %w", database.MapError(err))
	}

	return requireAffected(result, "revoke token")
}

// DeleteExpired удаляет просроченные токены: у сессии с истёкшим сроком смысла
// уже нет, а таблица растёт с каждым входом в систему.
// RevokeByUser отзывает все действующие сессии пользователя.
func (r *tokenRepo) RevokeByUser(ctx context.Context, userID string, at time.Time) (int64, error) {
	return r.revokeWhere(ctx, "user_id = ?", "revoke user tokens", userID, at)
}

// RevokeByTenant отзывает все действующие сессии тенанта.
func (r *tokenRepo) RevokeByTenant(ctx context.Context, tenantID string, at time.Time) (int64, error) {
	return r.revokeWhere(ctx, "tenant_id = ?", "revoke tenant tokens", tenantID, at)
}

// RevokeByUserTenant отзывает действующие сессии пользователя в тенанте.
func (r *tokenRepo) RevokeByUserTenant(
	ctx context.Context,
	userID, tenantID string,
	at time.Time,
) (int64, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "RevokeByUserTenant")
	defer spanEnd()

	const query = "UPDATE user_tokens SET revoked_at = COALESCE(revoked_at, ?)" +
		" WHERE user_id = ? AND tenant_id = ? AND revoked_at IS NULL"

	action := "revoke user tenant tokens"

	result, err := conn(traceCtx, r.db).ExecContext(traceCtx, r.rebind(query), r.timeArg(at), userID, tenantID)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", action, database.MapError(err))
	}

	return rowsAffected(result, action)
}

// revokeWhere отзывает действующие сессии по одному условию.
func (r *tokenRepo) revokeWhere(
	ctx context.Context,
	condition, action, value string,
	at time.Time,
) (int64, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "RevokeByCondition")
	defer spanEnd()

	query := "UPDATE user_tokens SET revoked_at = COALESCE(revoked_at, ?)" +
		" WHERE " + condition + " AND revoked_at IS NULL"

	result, err := conn(traceCtx, r.db).ExecContext(traceCtx, r.rebind(query), r.timeArg(at), value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", action, database.MapError(err))
	}

	return rowsAffected(result, action)
}

// rowsAffected возвращает число изменённых строк: 0 — не ошибка, сессий могло
// не быть вовсе.
func rowsAffected(result sql.Result, action string) (int64, error) {
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("%s: rows affected: %w", action, err)
	}

	return affected, nil
}

func (r *tokenRepo) DeleteExpired(ctx context.Context, user_id string, before time.Time) (int64, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "DeleteExpired")
	defer spanEnd()

	const query = "DELETE FROM user_tokens WHERE user_id = ? AND expires_at < ?"

	result, err := conn(traceCtx, r.db).ExecContext(traceCtx, r.rebind(query), user_id, r.timeArg(before))
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
