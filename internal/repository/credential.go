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

// CredentialDB — интерфейс базы данных, который нужен CredentialRepo.
//
// Набор методов совпадает с dbConn: внутри UnitOfWork запрос уходит в
// транзакцию из контекста.
type CredentialDB interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// CredentialRepo — алиас доменного порта CredentialRepository: репозиторий реализует
// контракт учётных данных, объявленный в internal/domain.
type CredentialRepo = domain.CredentialRepository

// credentialColumns — колонки user_credentials в порядке чтения
// scanCredentials. Порядок должен совпадать: рассинхрон ловят тесты.
const credentialColumns = "id, user_id, provider, provider_key, password_hash, provider_meta," +
	" created_at, last_login_at, expires_at, revoked_at, is_active, is_delete"

// credentialRepo — конкретная реализация, скрыта от внешних пакетов.
type credentialRepo struct {
	db CredentialDB
	codec
}

// NewCredentialRepo создаёт репозиторий учётных данных.
func NewCredentialRepo(ctx context.Context, db CredentialDB, dialect string) CredentialRepo {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "NewCredentialRepo")
	defer spanEnd()

	slog.DebugContext(traceCtx, "credential repository created", "dialect", dialect)

	return &credentialRepo{db: db, codec: codec{dialect: dialect}}
}

// Create сохраняет учётные данные.
//
// Хеш пароля считает модель (model.NewCredentials): в репозиторий попадает уже
// готовый хеш, поэтому открытый пароль до базы не доходит.
func (r *credentialRepo) Create(ctx context.Context, credentials *model.Credentials) error {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "Create")
	defer spanEnd()

	const query = "INSERT INTO user_credentials (" + credentialColumns +
		") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"

	if _, err := conn(traceCtx, r.db).ExecContext(traceCtx, r.rebind(query),
		credentials.ID,
		credentials.UserID,
		credentials.Provider,
		credentials.ProviderKey,
		credentials.Hash,
		r.jsonArg(credentials.Meta),
		r.timeArg(credentials.CreatedAt),
		r.nullTimeArg(credentials.LastLoginAt),
		r.timeArg(credentials.ExpiresAt),
		r.nullTimeArg(credentials.RevokedAt),
		credentials.IsActive,
		credentials.IsDelete,
	); err != nil {
		return fmt.Errorf("insert credentials: %w", database.MapError(err))
	}

	return nil
}

// Update перезаписывает изменяемые поля учётных данных.
//
// Используется сменой пароля и «прозрачным» обновлением хеша при входе: в
// обоих случаях меняются хеш и срок действия. Провайдер и его ключ не
// обновляются — они описывают, чьи это учётные данные, а не их состояние.
func (r *credentialRepo) Update(ctx context.Context, credentials *model.Credentials) error {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "Update")
	defer spanEnd()

	const query = "UPDATE user_credentials SET password_hash = ?, provider_meta = ?," +
		" expires_at = ?, last_login_at = ?, revoked_at = ?, is_active = ?, is_delete = ?" +
		" WHERE id = ?"

	result, err := conn(traceCtx, r.db).ExecContext(traceCtx, r.rebind(query),
		credentials.Hash,
		r.jsonArg(credentials.Meta),
		r.timeArg(credentials.ExpiresAt),
		r.nullTimeArg(credentials.LastLoginAt),
		r.nullTimeArg(credentials.RevokedAt),
		credentials.IsActive,
		credentials.IsDelete,
		credentials.ID,
	)
	if err != nil {
		return fmt.Errorf("update credentials: %w", database.MapError(err))
	}

	return requireAffected(result, "update credentials")
}

// GetByUserID возвращает действующие учётные данные пользователя — они нужны
// при входе, смене пароля и проверке срока его действия.
//
// Ищется запись локальной авторизации (провайдер email): остальные провайдеры
// (Telegram, Google, ...) пароля не имеют, а проверяется как раз пароль.
// Если записей несколько (схема этого не запрещает: уникальность задана только
// первичным ключом), берётся последняя созданная — то есть действующая.
func (r *credentialRepo) GetByUserID(ctx context.Context, userID string) (*model.Credentials, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "GetByUserID")
	defer spanEnd()

	const query = "SELECT " + credentialColumns + " FROM user_credentials" +
		" WHERE user_id = ? AND provider = ? AND is_active = true AND is_delete = false" +
		" ORDER BY created_at DESC LIMIT 1"

	// Значение приходит от вызывающего слоя, поэтому проверяется до запроса:
	// невалидный id отклоняется здесь, а не отдаётся драйверу как аргумент.
	if err := model.ValidateUUID(userID); err != nil {
		return nil, fmt.Errorf("select credentials: %w", err)
	}

	return r.getBy(traceCtx, query, userID, string(model.CredentialProviderEmail))
}

// GetByProvider находит учётные данные по паре «провайдер + ключ провайдера».
func (r *credentialRepo) GetByProvider(ctx context.Context, provider, providerKey string) (*model.Credentials, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "GetByProvider")
	defer spanEnd()

	const query = "SELECT " + credentialColumns + " FROM user_credentials" +
		" WHERE provider = ? AND provider_key = ? AND is_active = true AND is_delete = false" +
		" ORDER BY created_at DESC LIMIT 1"

	if provider == "" || providerKey == "" {
		return nil, fmt.Errorf("select credentials: %w: provider and provider_key are required",
			model.ErrInvalidCredential)
	}

	return r.getBy(traceCtx, query, provider, providerKey)
}

// TouchLastLogin отмечает вход по учётным данным.
func (r *credentialRepo) TouchLastLogin(ctx context.Context, id string, at time.Time) error {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "TouchLastLogin")
	defer spanEnd()

	const query = "UPDATE user_credentials SET last_login_at = ? WHERE id = ?"

	result, err := conn(traceCtx, r.db).ExecContext(traceCtx, r.rebind(query), r.timeArg(at), id)
	if err != nil {
		return fmt.Errorf("touch credentials: %w", database.MapError(err))
	}

	return requireAffected(result, "touch credentials")
}

// DeactivateByUser мягко удаляет все учётные данные пользователя.
//
// Вызывается при удалении аккаунта: пароль не должен оставаться действующим.
// Операция идемпотентна — у профиля, созданного сотрудником, учётных данных
// может не быть вовсе, и это не ошибка.
func (r *credentialRepo) DeactivateByUser(ctx context.Context, userID string, at time.Time) error {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "DeactivateByUser")
	defer spanEnd()

	const query = "UPDATE user_credentials SET revoked_at = ?, is_active = false, is_delete = true" +
		" WHERE user_id = ? AND is_active = true AND is_delete = false"

	if _, err := conn(traceCtx, r.db).ExecContext(traceCtx, r.rebind(query), r.timeArg(at), userID); err != nil {
		return fmt.Errorf("deactivate credentials: %w", database.MapError(err))
	}

	return nil
}

// getBy выполняет SELECT одной строки и читает её в модель.
func (r *credentialRepo) getBy(ctx context.Context, query string, args ...any) (*model.Credentials, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "getBy")
	defer spanEnd()

	credentials, err := scanCredentials(traceCtx, conn(traceCtx, r.db).QueryRowContext(traceCtx, r.rebind(query), args...))
	if err != nil {
		return nil, fmt.Errorf("select credentials: %w", database.MapError(err))
	}

	return credentials, nil
}

// scanCredentials читает строку user_credentials в доменную модель.
//
// Время разбирает nullTime: PostgreSQL отдаёт time.Time, SQLite — строку
// RFC3339. Метаданные провайдера принимает nullBytes: JSONB приходит как
// []byte, TEXT — как строка.
func scanCredentials(ctx context.Context, row *sql.Row) (*model.Credentials, error) {
	_, spanEnd := observability.TraceSpan(ctx, "scanCredentials")
	defer spanEnd()

	var (
		credentials          model.Credentials
		meta                 nullBytes
		createdAt, expiresAt nullTime
		lastLogin, revokedAt nullTime
	)

	if err := row.Scan(
		&credentials.ID,
		&credentials.UserID,
		&credentials.Provider,
		&credentials.ProviderKey,
		&credentials.Hash,
		&meta,
		&createdAt,
		&lastLogin,
		&expiresAt,
		&revokedAt,
		&credentials.IsActive,
		&credentials.IsDelete,
	); err != nil {
		return nil, err
	}

	credentials.Meta = meta.value
	credentials.CreatedAt = createdAt.time()
	credentials.LastLoginAt = lastLogin.ptr()
	credentials.ExpiresAt = expiresAt.time()
	credentials.RevokedAt = revokedAt.ptr()

	return &credentials, nil
}
