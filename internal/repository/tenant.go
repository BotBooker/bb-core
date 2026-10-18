package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/model"
	"github.com/botbooker/bb-core/pkg/observability"
)

// TenantDB — интерфейс базы данных, который нужен TenantRepo.
//
// Методы совпадают с методами database.DB, но объявлены узко: репозиторию
// нужны только запросы и транзакции для операций «тенант + владелец» и
// передачи владения.
type TenantDB interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// TenantRepo — интерфейс репозитория тенантов и членства в них.
//
// Роль пользователя — атрибут членства (tenant_members.role_id), поэтому
// именно здесь находится источник действующей роли: профиль (users) её не
// хранит, а сессия (user_tokens.role_id) держит только снимок на момент выдачи.
type TenantRepo interface {
	// Create вставляет тенант; конфликт уникальности slug — database.ErrDuplicate.
	Create(ctx context.Context, tenant *model.Tenant) error
	// CreateWithOwner вставляет тенант и членство владельца в одной транзакции.
	CreateWithOwner(ctx context.Context, tenant *model.Tenant, member *model.TenantMember) error
	// GetByID возвращает тенант по идентификатору или database.ErrNotFound.
	GetByID(ctx context.Context, id string) (*model.Tenant, error)
	// GetBySlug возвращает тенант по slug или database.ErrNotFound.
	GetBySlug(ctx context.Context, slug string) (*model.Tenant, error)
	// List возвращает тенанты (кроме удалённых) с пагинацией.
	List(ctx context.Context, limit, offset int) ([]*model.Tenant, error)
	// ListActive возвращает действующие тенанты с пагинацией.
	ListActive(ctx context.Context, limit, offset int) ([]*model.Tenant, error)
	// UpdateStatus меняет статус тенанта; отсутствие строки — database.ErrNotFound.
	UpdateStatus(ctx context.Context, id string, status model.TenantStatus, now time.Time) error
	// AddMember добавляет пользователя в тенант; повторное членство —
	// database.ErrDuplicate, несуществующий тенант — нарушение внешнего ключа.
	AddMember(ctx context.Context, member *model.TenantMember) error
	// GetMember возвращает членство пользователя в тенанте или database.ErrNotFound.
	GetMember(ctx context.Context, tenantID, userID string) (*model.TenantMember, error)
	// GetMemberBySlug возвращает членство в действующем тенанте по slug или
	// database.ErrNotFound.
	GetMemberBySlug(ctx context.Context, slug, userID string) (*model.TenantMembership, error)
	// MemberOfUser возвращает членство пользователя в действующем тенанте,
	// которое используется для сессии, или database.ErrNotFound.
	MemberOfUser(ctx context.Context, userID string) (*model.TenantMember, error)
	// ListMemberships возвращает действующие членства пользователя вместе с
	// тенантами: это список «мои тенанты» для входа и переключения.
	ListMemberships(ctx context.Context, userID string) ([]*model.TenantMembership, error)
	// ListMembers возвращает участников тенанта с профилями пользователей.
	ListMembers(ctx context.Context, tenantID string, limit, offset int) ([]*model.TenantMemberView, error)
	// GetMemberView возвращает участника тенанта вместе с профилем.
	GetMemberView(ctx context.Context, tenantID, userID string) (*model.TenantMemberView, error)
	// UpdateMemberRole меняет роль участника; отсутствие строки — database.ErrNotFound.
	UpdateMemberRole(ctx context.Context, tenantID, userID string, role model.Role) error
	// RemoveMember удаляет членство; отсутствие строки — database.ErrNotFound.
	RemoveMember(ctx context.Context, tenantID, userID string) error
	// TransferOwnership передаёт владение тенантом участнику в одной транзакции.
	// Целевой участник должен существовать; отсутствие — database.ErrNotFound.
	TransferOwnership(ctx context.Context, tenantID, userID string) error
}

// tenantColumns — колонки tenants в порядке чтения scanTenant.
const tenantColumns = "id, name, slug, status, metadata, created_at, updated_at"

// memberColumns — колонки tenant_members в порядке чтения scanTenantMember.
const memberColumns = "tenant_id, user_id, role_id, is_owner, joined_at"

// memberViewColumns — колонки участника вместе с профилем пользователя.
const memberViewColumns = "m.tenant_id, m.user_id, m.role_id, m.is_owner, m.joined_at," +
	" u.email, u.first_name, u.last_name"

// memberViewFrom — источник строк участника: членство и профиль.
const memberViewFrom = " FROM tenant_members m JOIN users u ON u.id = m.user_id"

// tenantRepo — конкретная реализация, скрыта от внешних пакетов.
type tenantRepo struct {
	db TenantDB
	codec
}

// NewTenantRepo создаёт репозиторий тенантов.
func NewTenantRepo(ctx context.Context, db TenantDB, dialect string) TenantRepo {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "NewTenantRepo")
	defer spanEnd()

	slog.DebugContext(traceCtx, "tenant repository created", "dialect", dialect)

	return &tenantRepo{db: db, codec: codec{dialect: dialect}}
}

// Create вставляет тенант.
func (r *tenantRepo) Create(ctx context.Context, tenant *model.Tenant) error {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "Create")
	defer spanEnd()

	const query = `INSERT INTO tenants (id, name, slug, status, metadata, created_at, updated_at)` +
		` VALUES (?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(traceCtx, r.rebind(query),
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

// CreateWithOwner вставляет тенант и членство владельца в одной транзакции.
//
// Нужно, чтобы не осталось тенанта без владельца: частичный уникальный индекс
// tenant_members_owner_idx допускает ровно одного владельца, но не требует его
// наличия. Ошибка на второй вставке откатывает первую.
func (r *tenantRepo) CreateWithOwner(
	ctx context.Context,
	tenant *model.Tenant,
	member *model.TenantMember,
) error {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "CreateWithOwner")
	defer spanEnd()

	roleID, err := member.Role.ID()
	if err != nil {
		return fmt.Errorf("insert tenant with owner: %w", err)
	}

	tx, err := r.db.BeginTx(traceCtx, nil)
	if err != nil {
		return fmt.Errorf("insert tenant with owner: begin transaction: %w", err)
	}

	committed := false

	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	const tenantQuery = `INSERT INTO tenants (id, name, slug, status, metadata, created_at, updated_at)` +
		` VALUES (?, ?, ?, ?, ?, ?, ?)`

	if _, err := tx.ExecContext(traceCtx, r.rebind(tenantQuery),
		tenant.ID,
		tenant.Name,
		tenant.Slug,
		string(tenant.Status),
		r.jsonArg(tenant.Metadata),
		r.timeArg(tenant.CreatedAt),
		r.timeArg(tenant.UpdatedAt),
	); err != nil {
		return fmt.Errorf("insert tenant with owner: %w", database.MapError(err))
	}

	const memberQuery = `INSERT INTO tenant_members (tenant_id, user_id, role_id, is_owner, joined_at)` +
		` VALUES (?, ?, ?, ?, ?)`

	if _, err := tx.ExecContext(traceCtx, r.rebind(memberQuery),
		member.TenantID,
		member.UserID,
		roleID,
		member.IsOwner,
		r.timeArg(member.JoinedAt),
	); err != nil {
		return fmt.Errorf("insert tenant with owner: %w", database.MapError(err))
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("insert tenant with owner: commit: %w", err)
	}

	committed = true

	return nil
}

// GetByID возвращает тенант по идентификатору.
func (r *tenantRepo) GetByID(ctx context.Context, id string) (*model.Tenant, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "GetByID")
	defer spanEnd()

	const query = "SELECT " + tenantColumns + " FROM tenants WHERE id = ?"

	// Значение приходит от вызывающего слоя, поэтому проверяется до запроса:
	// невалидный id отклоняется здесь, а не отдаётся драйверу как аргумент.
	if err := model.ValidateUUID(id); err != nil {
		return nil, fmt.Errorf("select tenant: %w", err)
	}

	return scanTenant(r.db.QueryRowContext(traceCtx, r.rebind(query), id))
}

// GetBySlug возвращает тенант по slug: он уникален (tenants_slug_idx).
func (r *tenantRepo) GetBySlug(ctx context.Context, slug string) (*model.Tenant, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "GetBySlug")
	defer spanEnd()

	const query = "SELECT " + tenantColumns + " FROM tenants WHERE slug = ?"

	return scanTenant(r.db.QueryRowContext(traceCtx, r.rebind(query), model.NormalizeSlug(slug)))
}

// List возвращает тенанты (кроме удалённых) с пагинацией.
func (r *tenantRepo) List(ctx context.Context, limit, offset int) ([]*model.Tenant, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "List")
	defer spanEnd()

	return r.listTenants(traceCtx, "", limit, offset)
}

// ListActive возвращает действующие тенанты с пагинацией.
func (r *tenantRepo) ListActive(ctx context.Context, limit, offset int) ([]*model.Tenant, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "ListActive")
	defer spanEnd()

	return r.listTenants(traceCtx, string(model.TenantStatusActive), limit, offset)
}

// listTenants — общий запрос списка тенантов: status пустой означает «кроме
// удалённых».
func (r *tenantRepo) listTenants(
	ctx context.Context,
	status string,
	limit, offset int,
) ([]*model.Tenant, error) {
	query := "SELECT " + tenantColumns + " FROM tenants"
	args := make([]any, 0, 3)

	if status == "" {
		query += " WHERE status <> ?"
		args = append(args, string(model.TenantStatusDeleted))
	} else {
		query += " WHERE status = ?"
		args = append(args, status)
	}

	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	// query собирается из констант, значения передаются параметрами: срабатывание
	// правила go_gosec_sql_concat_sqli ложное.
	// bearer:disable go_gosec_sql_concat_sqli
	rows, err := r.db.QueryContext(ctx, r.rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("select tenants: %w", database.MapError(err))
	}
	defer func() { _ = rows.Close() }()

	tenants := make([]*model.Tenant, 0, limit)

	for rows.Next() {
		tenant, err := scanTenant(rows)
		if err != nil {
			return nil, err
		}

		tenants = append(tenants, tenant)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("select tenants: %w", database.MapError(err))
	}

	return tenants, nil
}

// UpdateStatus меняет статус тенанта.
func (r *tenantRepo) UpdateStatus(
	ctx context.Context,
	id string,
	status model.TenantStatus,
	now time.Time,
) error {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "UpdateStatus")
	defer spanEnd()

	if err := model.ValidateUUID(id); err != nil {
		return fmt.Errorf("update tenant status: %w", err)
	}

	if !status.Valid() {
		return fmt.Errorf("update tenant status: %w: status %q", model.ErrInvalidTenant, status)
	}

	const query = "UPDATE tenants SET status = ?, updated_at = ? WHERE id = ?"

	result, err := r.db.ExecContext(traceCtx, r.rebind(query), string(status), r.timeArg(now), id)
	if err != nil {
		return fmt.Errorf("update tenant status: %w", database.MapError(err))
	}

	return requireAffected(result, "update tenant status")
}

// AddMember добавляет пользователя в тенант.
//
// Роль записывается ссылкой на справочник roles (role_id): код роли переводит
// модель (model.Role.ID), поэтому в базу не может попасть роль, которой нет в
// справочнике.
func (r *tenantRepo) AddMember(ctx context.Context, member *model.TenantMember) error {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "AddMember")
	defer spanEnd()

	const query = "INSERT INTO tenant_members (" + memberColumns + ") VALUES (?, ?, ?, ?, ?)"

	roleID, err := member.Role.ID()
	if err != nil {
		return fmt.Errorf("insert tenant member: %w", err)
	}

	_, err = r.db.ExecContext(traceCtx, r.rebind(query),
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
	traceCtx, spanEnd := observability.TraceSpan(ctx, "GetMember")
	defer spanEnd()

	const query = "SELECT " + memberColumns + " FROM tenant_members" +
		" WHERE tenant_id = ? AND user_id = ?"

	if err := model.ValidateUUID(tenantID); err != nil {
		return nil, fmt.Errorf("select tenant member: %w", err)
	}

	if err := model.ValidateUUID(userID); err != nil {
		return nil, fmt.Errorf("select tenant member: %w", err)
	}

	return scanTenantMember(r.db.QueryRowContext(traceCtx, r.rebind(query), tenantID, userID))
}

// GetMemberBySlug возвращает членство в действующем тенанте по slug.
//
// Нужно входу с tenant_slug: slug публичен, но сессия выдаётся только участнику.
// Приостановленный или удалённый тенант членства не даёт.
func (r *tenantRepo) GetMemberBySlug(
	ctx context.Context,
	slug, userID string,
) (*model.TenantMembership, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "GetMemberBySlug")
	defer spanEnd()

	const query = "SELECT t.id, t.name, t.slug, t.status, t.metadata, t.created_at, t.updated_at," +
		" m.tenant_id, m.user_id, m.role_id, m.is_owner, m.joined_at" +
		" FROM tenants t JOIN tenant_members m ON m.tenant_id = t.id" +
		" WHERE t.slug = ? AND m.user_id = ? AND t.status = ?"

	if err := model.ValidateUUID(userID); err != nil {
		return nil, fmt.Errorf("select tenant membership: %w", err)
	}

	normalized := model.NormalizeSlug(slug)
	if normalized == "" {
		return nil, fmt.Errorf("select tenant membership: %w: empty slug", database.ErrNotFound)
	}

	return scanMembership(r.db.QueryRowContext(traceCtx, r.rebind(query),
		normalized, userID, string(model.TenantStatusActive)))
}

// ListMemberships возвращает действующие членства пользователя вместе с тенантами.
func (r *tenantRepo) ListMemberships(
	ctx context.Context,
	userID string,
) ([]*model.TenantMembership, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "ListMemberships")
	defer spanEnd()

	const query = "SELECT t.id, t.name, t.slug, t.status, t.metadata, t.created_at, t.updated_at," +
		" m.tenant_id, m.user_id, m.role_id, m.is_owner, m.joined_at" +
		" FROM tenant_members m JOIN tenants t ON t.id = m.tenant_id" +
		" WHERE m.user_id = ? AND t.status = ? ORDER BY m.joined_at ASC"

	if err := model.ValidateUUID(userID); err != nil {
		return nil, fmt.Errorf("select tenant memberships: %w", err)
	}

	rows, err := r.db.QueryContext(traceCtx, r.rebind(query), userID, string(model.TenantStatusActive))
	if err != nil {
		return nil, fmt.Errorf("select tenant memberships: %w", database.MapError(err))
	}
	defer func() { _ = rows.Close() }()

	memberships := make([]*model.TenantMembership, 0, 1)

	for rows.Next() {
		membership, err := scanMembership(rows)
		if err != nil {
			return nil, err
		}

		memberships = append(memberships, membership)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("select tenant memberships: %w", database.MapError(err))
	}

	return memberships, nil
}

// MemberOfUser возвращает членство пользователя в действующем тенанте.
//
// Это «тенант по умолчанию» для сессии: у пользователя тенантов может быть
// несколько, а токен действует ровно в одном (user_tokens.tenant_id), поэтому
// берётся самое раннее членство в тенанте со статусом active. Сессия в
// приостановленном или удалённом тенанте не выдаётся, поэтому такие членства
// JOIN не возвращает.
func (r *tenantRepo) MemberOfUser(ctx context.Context, userID string) (*model.TenantMember, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "MemberOfUser")
	defer spanEnd()

	const query = "SELECT m.tenant_id, m.user_id, m.role_id, m.is_owner, m.joined_at" +
		" FROM tenant_members m JOIN tenants t ON t.id = m.tenant_id" +
		" WHERE m.user_id = ? AND t.status = ? ORDER BY m.joined_at ASC LIMIT 1"

	if err := model.ValidateUUID(userID); err != nil {
		return nil, fmt.Errorf("select tenant member: %w", err)
	}

	return scanTenantMember(r.db.QueryRowContext(traceCtx, r.rebind(query),
		userID, string(model.TenantStatusActive)))
}

// ListMembers возвращает участников тенанта с профилями пользователей.
func (r *tenantRepo) ListMembers(
	ctx context.Context,
	tenantID string,
	limit, offset int,
) ([]*model.TenantMemberView, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "ListMembers")
	defer spanEnd()

	if err := model.ValidateUUID(tenantID); err != nil {
		return nil, fmt.Errorf("select tenant members: %w", err)
	}

	query := "SELECT " + memberViewColumns + memberViewFrom +
		" WHERE m.tenant_id = ? ORDER BY m.is_owner DESC, m.joined_at ASC LIMIT ? OFFSET ?"

	rows, err := r.db.QueryContext(traceCtx, r.rebind(query), tenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("select tenant members: %w", database.MapError(err))
	}
	defer func() { _ = rows.Close() }()

	members := make([]*model.TenantMemberView, 0, limit)

	for rows.Next() {
		member, err := scanTenantMemberView(rows)
		if err != nil {
			return nil, err
		}

		members = append(members, member)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("select tenant members: %w", database.MapError(err))
	}

	return members, nil
}

// GetMemberView возвращает участника тенанта вместе с профилем.
func (r *tenantRepo) GetMemberView(
	ctx context.Context,
	tenantID, userID string,
) (*model.TenantMemberView, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "GetMemberView")
	defer spanEnd()

	if err := model.ValidateUUID(tenantID); err != nil {
		return nil, fmt.Errorf("select tenant member: %w", err)
	}

	if err := model.ValidateUUID(userID); err != nil {
		return nil, fmt.Errorf("select tenant member: %w", err)
	}

	query := "SELECT " + memberViewColumns + memberViewFrom +
		" WHERE m.tenant_id = ? AND m.user_id = ?"

	return scanTenantMemberView(r.db.QueryRowContext(traceCtx, r.rebind(query), tenantID, userID))
}

// UpdateMemberRole меняет роль участника.
func (r *tenantRepo) UpdateMemberRole(
	ctx context.Context,
	tenantID, userID string,
	role model.Role,
) error {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "UpdateMemberRole")
	defer spanEnd()

	roleID, err := role.ID()
	if err != nil {
		return fmt.Errorf("update tenant member role: %w", err)
	}

	const query = "UPDATE tenant_members SET role_id = ? WHERE tenant_id = ? AND user_id = ?"

	result, err := r.db.ExecContext(traceCtx, r.rebind(query), roleID, tenantID, userID)
	if err != nil {
		return fmt.Errorf("update tenant member role: %w", database.MapError(err))
	}

	return requireAffected(result, "update tenant member role")
}

// RemoveMember удаляет членство.
func (r *tenantRepo) RemoveMember(ctx context.Context, tenantID, userID string) error {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "RemoveMember")
	defer spanEnd()

	const query = "DELETE FROM tenant_members WHERE tenant_id = ? AND user_id = ?"

	result, err := r.db.ExecContext(traceCtx, r.rebind(query), tenantID, userID)
	if err != nil {
		return fmt.Errorf("delete tenant member: %w", database.MapError(err))
	}

	return requireAffected(result, "delete tenant member")
}

// TransferOwnership передаёт владение тенантом участнику.
//
// Порядок важен: сначала снимается флаг у текущего владельца, затем ставится
// новому — иначе частичный уникальный индекс tenant_members_owner_idx отклонит
// вторую строку с is_owner = true. Новый владелец получает роль admin: владелец
// без прав администратора не смог бы управлять тенантом.
func (r *tenantRepo) TransferOwnership(ctx context.Context, tenantID, userID string) error {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "TransferOwnership")
	defer spanEnd()

	if err := model.ValidateUUID(tenantID); err != nil {
		return fmt.Errorf("transfer ownership: %w", err)
	}

	if err := model.ValidateUUID(userID); err != nil {
		return fmt.Errorf("transfer ownership: %w", err)
	}

	tx, err := r.db.BeginTx(traceCtx, nil)
	if err != nil {
		return fmt.Errorf("transfer ownership: begin transaction: %w", err)
	}

	committed := false

	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	const memberQuery = "SELECT role_id FROM tenant_members WHERE tenant_id = ? AND user_id = ?"

	var roleID int16

	if err := tx.QueryRowContext(traceCtx, r.rebind(memberQuery), tenantID, userID).Scan(&roleID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("transfer ownership: %w", database.ErrNotFound)
		}

		return fmt.Errorf("transfer ownership: %w", database.MapError(err))
	}

	const clearQuery = "UPDATE tenant_members SET is_owner = false WHERE tenant_id = ? AND is_owner = true"

	if _, err := tx.ExecContext(traceCtx, r.rebind(clearQuery), tenantID); err != nil {
		return fmt.Errorf("transfer ownership: %w", database.MapError(err))
	}

	adminID, err := model.RoleAdmin.ID()
	if err != nil {
		return fmt.Errorf("transfer ownership: %w", err)
	}

	const promoteQuery = "UPDATE tenant_members SET is_owner = true, role_id = ?" +
		" WHERE tenant_id = ? AND user_id = ?"

	if _, err := tx.ExecContext(traceCtx, r.rebind(promoteQuery), adminID, tenantID, userID); err != nil {
		return fmt.Errorf("transfer ownership: %w", database.MapError(err))
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("transfer ownership: commit: %w", err)
	}

	committed = true

	return nil
}

// scanner — общий контракт *sql.Row и *sql.Rows: оба умеют Scan.
type scanner interface {
	Scan(dest ...any) error
}

// scanTenant читает строку tenants в доменную модель.
func scanTenant(row scanner) (*model.Tenant, error) {
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
func scanTenantMember(row scanner) (*model.TenantMember, error) {
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

// scanMembership читает тенант и членство из одной строки JOIN.
func scanMembership(row scanner) (*model.TenantMembership, error) {
	var (
		membership           model.TenantMembership
		status               string
		metadata             nullBytes
		createdAt, updatedAt nullTime
		roleID               int16
		joinedAt             nullTime
	)

	err := row.Scan(
		&membership.Tenant.ID,
		&membership.Tenant.Name,
		&membership.Tenant.Slug,
		&status,
		&metadata,
		&createdAt,
		&updatedAt,
		&membership.Member.TenantID,
		&membership.Member.UserID,
		&roleID,
		&membership.Member.IsOwner,
		&joinedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("select tenant membership: %w", database.MapError(err))
	}

	membership.Tenant.Status = model.TenantStatus(status)
	membership.Tenant.Metadata = metadata.value
	membership.Tenant.CreatedAt = createdAt.time()
	membership.Tenant.UpdatedAt = updatedAt.time()

	membership.Member.Role, err = model.ParseRoleID(roleID)
	if err != nil {
		return nil, fmt.Errorf("select tenant membership: %w", err)
	}

	membership.Member.JoinedAt = joinedAt.time()

	return &membership, nil
}

// scanTenantMemberView читает участника вместе с профилем пользователя.
func scanTenantMemberView(row scanner) (*model.TenantMemberView, error) {
	var (
		member   model.TenantMemberView
		roleID   int16
		joinedAt nullTime
	)

	err := row.Scan(
		&member.TenantID,
		&member.UserID,
		&roleID,
		&member.IsOwner,
		&joinedAt,
		&member.Email,
		&member.FirstName,
		&member.LastName,
	)
	if err != nil {
		return nil, fmt.Errorf("select tenant member: %w", database.MapError(err))
	}

	member.Role, err = model.ParseRoleID(roleID)
	if err != nil {
		return nil, fmt.Errorf("select tenant member: %w", err)
	}

	member.JoinedAt = joinedAt.time()

	return &member, nil
}
