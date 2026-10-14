package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/model"
)

// CredentialDB — интерфейс базы данных, который нужен CredentialRepo.
type CredentialDB interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// CredentialRepo — интерфейс репозитория учётных данных (хешей паролей).
//
// Отдельная таблица (user_credentials), как в ERDB и Шаге 3 маппинга
// (docs/specification/user_schema_mapping.md): профиль читается почти каждым
// запросом, а хеш пароля нужен только при входе и смене пароля.
type CredentialRepo interface {
	// Create сохраняет хеш пароля; повторная вставка — database.ErrDuplicate.
	Create(ctx context.Context, credentials *model.Credentials) error
	// GetByUserID возвращает учётные данные или database.ErrNotFound.
	GetByUserID(ctx context.Context, userID string) (*model.Credentials, error)
	// Update перезаписывает хеш пароля и время его изменения;
	// отсутствие записи — database.ErrNotFound.
	Update(ctx context.Context, credentials *model.Credentials) error
}

// credentialColumns — колонки user_credentials в порядке чтения scanCredentials.
const credentialColumns = "user_id, password_hash, created_at, updated_at"

// credentialRepo — конкретная реализация, скрыта от внешних пакетов.
type credentialRepo struct {
	db CredentialDB
	codec
}

// NewCredentialRepo создаёт репозиторий учётных данных.
func NewCredentialRepo(db CredentialDB, dialect string) CredentialRepo {
	slog.Debug("credential repository created", "dialect", dialect)

	return &credentialRepo{db: db, codec: codec{dialect: dialect}}
}

// Create сохраняет учётные данные.
//
// Хеш пароля считает модель (model.NewCredentials): в репозиторий попадает уже
// готовый хеш, поэтому открытый пароль до базы не доходит.
func (r *credentialRepo) Create(ctx context.Context, credentials *model.Credentials) error {
	const query = "INSERT INTO user_credentials (" + credentialColumns + ") VALUES (?, ?, ?, ?)"

	_, err := r.db.ExecContext(ctx, r.rebind(query),
		credentials.UserID,
		credentials.Hash,
		r.timeArg(credentials.CreatedAt),
		r.timeArg(credentials.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("insert credentials: %w", database.MapError(err))
	}

	return nil
}

// Update перезаписывает хеш пароля и время его изменения.
//
// Используется и сменой пароля, и «прозрачным» обновлением хеша при входе
// (пересчёт устаревшего алгоритма): в обоих случаях меняется только хеш и
// время, поэтому отдельного метода для смены пароля не нужно.
func (r *credentialRepo) Update(ctx context.Context, credentials *model.Credentials) error {
	const query = "UPDATE user_credentials SET password_hash = ?, updated_at = ? WHERE user_id = ?"

	result, err := r.db.ExecContext(ctx, r.rebind(query),
		credentials.Hash,
		r.timeArg(credentials.UpdatedAt),
		credentials.UserID,
	)
	if err != nil {
		return fmt.Errorf("update credentials: %w", database.MapError(err))
	}

	return requireAffected(result, "update credentials")
}

// GetByUserID возвращает учётные данные пользователя — они нужны при входе.
func (r *credentialRepo) GetByUserID(ctx context.Context, userID string) (*model.Credentials, error) {
	const query = "SELECT " + credentialColumns + " FROM user_credentials WHERE user_id = ?"

	credentials, err := scanCredentials(r.db.QueryRowContext(ctx, r.rebind(query), userID))
	if err != nil {
		return nil, fmt.Errorf("select credentials: %w", database.MapError(err))
	}

	return credentials, nil
}

// scanCredentials читает строку user_credentials в доменную модель.
func scanCredentials(row *sql.Row) (*model.Credentials, error) {
	var (
		credentials          model.Credentials
		createdAt, updatedAt nullTime
	)

	err := row.Scan(&credentials.UserID, &credentials.Hash, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}

	credentials.CreatedAt = createdAt.time()
	credentials.UpdatedAt = updatedAt.time()

	return &credentials, nil
}
