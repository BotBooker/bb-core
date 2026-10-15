package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/model"
)

// TenantDB — интерфейс базы данных, который нужен TenantRepo.
//
// Методы совпадают с методами database.DB, но объявлены узко: репозиторию
// нужны только запросы, а менеджер транзакций и закрытие пула — нет.
type TenantDB interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// TenantRepo — интерфейс репозитория тенантов и членства в них.
//
// Роль пользователя — атрибут членства (tenant_members.role_id), поэтому
// именно здесь находится источник действующей роли: профиль (users) её не
// хранит, а сессия (user_tokens.role_id) держит только снимок на момент выдачи.
type TenantRepo interface {
	// Create вставляет тенант; конфликт уникальности slug — database.ErrDuplicate.
	Create(ctx context.Context, tenant *model.Tenant) error
	// GetByID возвращает тенант по идентификатору или database.ErrNotFound.
	GetByID(ctx context.Context, id string) (*model.Tenant, error)
	// GetBySlug возвращает тенант по slug или database.ErrNotFound.
	GetBySlug(ctx context.Context, slug string) (*model.Tenant, error)
	// AddMember добавляет пользователя в тенант; повторное членство —
	// database.ErrDuplicate, несуществующий тенант — нарушение внешнего ключа.
	AddMember(ctx context.Context, member *model.TenantMember) error
	// GetMember возвращает членство пользователя в тенанте или database.ErrNotFound.
	GetMember(ctx context.Context, tenantID, userID string) (*model.TenantMember, error)
	// MemberOfUser возвращает членство пользователя в действующем тенанте,
	// которое используется для сессии, или database.ErrNotFound.
	MemberOfUser(ctx context.Context, userID string) (*model.TenantMember, error)
}

// tenantColumns — колонки tenants в порядке чтения scanTenant.
const tenantColumns = "id, name, slug, status, metadata, created_at, updated_at"

// memberColumns — колонки tenant_members в порядке чтения scanTenantMember.
const memberColumns = "tenant_id, user_id, role_id, is_owner, joined_at"

// tenantRepo — конкретная реализация, скрыта от внешних пакетов.
type tenantRepo struct {
	db TenantDB
	codec
}

// NewTenantRepo создаёт репозиторий тенантов.
func NewTenantRepo(db TenantDB, dialect string) TenantRepo {
	slog.Debug("tenant repository created", "dialect", dialect)

	return &tenantRepo{db: db, codec: codec{dialect: dialect}}
}

// Create вставляет тенант.
func (r *tenantRepo) Create(ctx context.Context, tenant *model.Tenant) error {
	const query = `INSERT INTO tenants (id, name, slug, status, metadata, created_at, updated_at)` +
		` VALUES (?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, r.rebind(query),
		tenant.ID,
		tenant.Name,
		tenant.Slug,
		string(tenant.Status),
		r.jsonArg(tenant.Metadata),
		r.timeArg(tenant.CreatedAt),
		r.timeArg(tenant.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("insert tenant: %w", database.MapError(err))
	}

	return nil
}

// GetByID возвращает тенант по идентификатору.
func (r *tenantRepo) GetByID(ctx context.Context, id string) (*model.Tenant, error) {
	const query = "SELECT " + tenantColumns + " FROM tenants WHERE id = ?"

	// Значение приходит от вызывающего слоя, поэтому проверяется до запроса:
	// невалидный id отклоняется здесь, а не отдаётся драйверу как аргумент.
	if err := model.ValidateUUID(id); err != nil {
		return nil, fmt.Errorf("select tenant: %w", err)
	}

	return scanTenant(r.db.QueryRowContext(ctx, r.rebind(query), id))
}

// GetBySlug возвращает тенант по slug: он уникален (tenants_slug_idx).
func (r *tenantRepo) GetBySlug(ctx context.Context, slug string) (*model.Tenant, error) {
	const query = "SELECT " + tenantColumns + " FROM tenants WHERE slug = ?"

	return scanTenant(r.db.QueryRowContext(ctx, r.rebind(query), model.NormalizeSlug(slug)))
}

// AddMember добавляет пользователя в тенант.
//
// Роль записывается ссылкой на справочник roles (role_id): код роли переводит
// модель (model.Role.ID), поэтому в базу не может попасть роль, которой нет в
// справочнике.
func (r *tenantRepo) AddMember(ctx context.Context, member *model.TenantMember) error {
	const query = "INSERT INTO tenant_members (" + memberColumns + ") VALUES (?, ?, ?, ?, ?)"

	roleID, err := member.Role.ID()
	if err != nil {
		return fmt.Errorf("insert tenant member: %w", err)
	}

	_, err = r.db.ExecContext(ctx, r.rebind(query),
		member.TenantID,
		member.UserID,
		roleID,
		member.IsOwner,
		r.timeArg(member.JoinedAt),
	)
	if err != nil {
		return fmt.Errorf("insert tenant member: %w", database.MapError(err))
	}

	return nil
}

// GetMember возвращает членство пользователя в тенанте.
func (r *tenantRepo) GetMember(ctx context.Context, tenantID, userID string) (*model.TenantMember, error) {
	const query = "SELECT " + memberColumns + " FROM tenant_members" +
		" WHERE tenant_id = ? AND user_id = ?"

	if err := model.ValidateUUID(tenantID); err != nil {
		return nil, fmt.Errorf("select tenant member: %w", err)
	}

	if err := model.ValidateUUID(userID); err != nil {
		return nil, fmt.Errorf("select tenant member: %w", err)
	}

	return scanTenantMember(r.db.QueryRowContext(ctx, r.rebind(query), tenantID, userID))
}

// MemberOfUser возвращает членство пользователя в действующем тенанте.
//
// Это «тенант по умолчанию» для сессии: у пользователя тенантов может быть
// несколько, а токен действует ровно в одном (user_tokens.tenant_id), поэтому
// берётся самое раннее членство в тенанте со статусом active. Сессия в
// приостановленном или удалённом тенанте не выдаётся, поэтому такие членства
// JOIN не возвращает.
func (r *tenantRepo) MemberOfUser(ctx context.Context, userID string) (*model.TenantMember, error) {
	const query = "SELECT m.tenant_id, m.user_id, m.role_id, m.is_owner, m.joined_at" +
		" FROM tenant_members m JOIN tenants t ON t.id = m.tenant_id" +
		" WHERE m.user_id = ? AND t.status = ? ORDER BY m.joined_at ASC LIMIT 1"

	if err := model.ValidateUUID(userID); err != nil {
		return nil, fmt.Errorf("select tenant member: %w", err)
	}

	return scanTenantMember(r.db.QueryRowContext(ctx, r.rebind(query),
		userID, string(model.TenantStatusActive)))
}

// scanTenant читает строку tenants в доменную модель.
func scanTenant(row *sql.Row) (*model.Tenant, error) {
	var (
		tenant               model.Tenant
		status               string
		metadata             nullBytes
		createdAt, updatedAt nullTime
	)

	err := row.Scan(
		&tenant.ID,
		&tenant.Name,
		&tenant.Slug,
		&status,
		&metadata,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("select tenant: %w", database.MapError(err))
	}

	tenant.Status = model.TenantStatus(status)
	tenant.Metadata = metadata.value
	tenant.CreatedAt = createdAt.time()
	tenant.UpdatedAt = updatedAt.time()

	return &tenant, nil
}

// scanTenantMember читает строку tenant_members в доменную модель.
//
// Роль приходит числовым role_id: код роли даёт model.ParseRoleID, поэтому
// неизвестный id (справочник и код разошлись) — ошибка чтения, а не пустая роль.
func scanTenantMember(row *sql.Row) (*model.TenantMember, error) {
	var (
		member   model.TenantMember
		roleID   int16
		joinedAt nullTime
	)

	err := row.Scan(&member.TenantID, &member.UserID, &roleID, &member.IsOwner, &joinedAt)
	if err != nil {
		return nil, fmt.Errorf("select tenant member: %w", database.MapError(err))
	}

	role, err := model.ParseRoleID(roleID)
	if err != nil {
		return nil, fmt.Errorf("select tenant member: %w", err)
	}

	member.Role = role
	member.JoinedAt = joinedAt.time()

	return &member, nil
}
