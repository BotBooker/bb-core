package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/model"
	"github.com/botbooker/bb-core/pkg/observability"
)

// TenantRepository — интерфейс репозитория тенантов для TenantService.
type TenantRepository interface {
	CreateWithOwner(ctx context.Context, tenant *model.Tenant, member *model.TenantMember) error
	GetByID(ctx context.Context, id string) (*model.Tenant, error)
	List(ctx context.Context, limit, offset int) ([]*model.Tenant, error)
	UpdateStatus(ctx context.Context, id string, status model.TenantStatus, now time.Time) error
	AddMember(ctx context.Context, member *model.TenantMember) error
	GetMember(ctx context.Context, tenantID, userID string) (*model.TenantMember, error)
	ListMemberships(ctx context.Context, userID string) ([]*model.TenantMembership, error)
	ListMembers(ctx context.Context, tenantID string, limit, offset int) ([]*model.TenantMemberView, error)
	GetMemberView(ctx context.Context, tenantID, userID string) (*model.TenantMemberView, error)
	UpdateMemberRole(ctx context.Context, tenantID, userID string, role model.Role) error
	RemoveMember(ctx context.Context, tenantID, userID string) error
	TransferOwnership(ctx context.Context, tenantID, userID string) error
}

// TenantUserRepository — интерфейс репозитория пользователей для TenantService.
type TenantUserRepository interface {
	GetByID(ctx context.Context, id string) (*model.User, error)
}

// TenantSessionRepository — интерфейс отзыва сессий для TenantService.
type TenantSessionRepository interface {
	RevokeByTenant(ctx context.Context, tenantID string, at time.Time) (int64, error)
	RevokeByUserTenant(ctx context.Context, userID, tenantID string, at time.Time) (int64, error)
}

// События управления тенантами.
const (
	// EventTenantCreated — тенант создан.
	EventTenantCreated = "tenant.created"
	// EventTenantStatusChanged — статус тенанта изменён.
	EventTenantStatusChanged = "tenant.status_changed"
	// EventTenantMemberAdded — участник добавлен в тенант.
	EventTenantMemberAdded = "tenant.member_added"
	// EventTenantMemberRemoved — участник исключён из тенанта.
	EventTenantMemberRemoved = "tenant.member_removed"
	// EventTenantMemberRoleChanged — роль участника изменена.
	EventTenantMemberRoleChanged = "tenant.member_role_changed"
	// EventTenantOwnershipTransferred — владение тенантом передано.
	EventTenantOwnershipTransferred = "tenant.ownership_transferred"
)

// TenantService — управление тенантами и их участниками.
//
// Права проверяются здесь, а не в транспорте: администратор тенанта управляет
// только своим тенантом, глобальные администраторы — всеми. Клиент видит
// тенант, в котором состоит, но не управляет им.
type TenantService interface {
	// Create создаёт тенант; создатель становится владельцем с ролью admin.
	// Доступно только глобальным администраторам.
	Create(ctx context.Context, actor *Principal, in model.CreateTenantInput) (*model.Tenant, error)
	// Get возвращает тенант участнику или глобальному администратору.
	Get(ctx context.Context, actor *Principal, tenantID string) (*model.Tenant, error)
	// List возвращает тенанты: глобальным администраторам — все, остальным —
	// только те, в которых они состоят.
	List(ctx context.Context, actor *Principal, page model.Page) ([]*model.Tenant, string, error)
	// Delete мягко удаляет тенант и отзывает его сессии.
	Delete(ctx context.Context, actor *Principal, tenantID string) error
	// UpdateStatus меняет статус тенанта; приостановка и удаление отзывают
	// сессии. Доступно только глобальным администраторам.
	UpdateStatus(
		ctx context.Context,
		actor *Principal,
		tenantID string,
		status model.TenantStatus,
	) (*model.Tenant, error)
	// ListMembers возвращает участников тенанта: staff тенанта и глобальные
	// администраторы.
	ListMembers(
		ctx context.Context,
		actor *Principal,
		tenantID string,
		page model.Page,
	) ([]*model.TenantMemberView, string, error)
	// AddMember добавляет пользователя в тенант.
	AddMember(
		ctx context.Context,
		actor *Principal,
		tenantID, userID string,
		role model.Role,
	) (*model.TenantMemberView, error)
	// RemoveMember исключает участника; владельца удалить нельзя.
	RemoveMember(ctx context.Context, actor *Principal, tenantID, userID string) error
	// ChangeMemberRole меняет роль участника; роль владельца меняется только
	// передачей владения.
	ChangeMemberRole(
		ctx context.Context,
		actor *Principal,
		tenantID, userID string,
		role model.Role,
	) (*model.TenantMemberView, error)
	// TransferOwnership передаёт владение другому участнику.
	TransferOwnership(
		ctx context.Context,
		actor *Principal,
		tenantID, userID string,
	) (*model.TenantMemberView, error)
}

// tenantService — конкретная реализация, скрыта от внешних пакетов.
type tenantService struct {
	tenants TenantRepository
	users   TenantUserRepository
	tokens  TenantSessionRepository
	events  AuthEventPublisher
	now     func() time.Time
}

// NewTenantService создаёт сервис управления тенантами.
func NewTenantService(
	ctx context.Context,
	tenants TenantRepository,
	users TenantUserRepository,
	tokens TenantSessionRepository,
	events AuthEventPublisher,
) TenantService {
	slog.DebugContext(ctx, "tenant service created")

	return &tenantService{tenants: tenants, users: users, tokens: tokens, events: events, now: time.Now}
}

// Create создаёт тенант и членство владельца.
func (s *tenantService) Create(
	ctx context.Context,
	actor *Principal,
	in model.CreateTenantInput,
) (*model.Tenant, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "CreateTenant")
	defer spanEnd()

	if actor == nil || actor.User == nil {
		return nil, fmt.Errorf("create tenant: %w", ErrUnauthorized)
	}

	if !actor.CanAdminAllTenants() {
		return nil, fmt.Errorf("create tenant: %w: only global administrators can create tenants", ErrForbidden)
	}

	now := s.now()
	tenant, err := model.NewTenant(in.Name, in.Slug, now)
	if err != nil {
		return nil, fmt.Errorf("create tenant: %w", err)
	}

	tenant.Metadata, err = in.MetadataJSON()
	if err != nil {
		return nil, fmt.Errorf("create tenant: %w", err)
	}

	member, err := model.NewTenantMember(tenant.ID, actor.User.ID, model.RoleAdmin, true, now)
	if err != nil {
		return nil, fmt.Errorf("create tenant: %w", err)
	}

	if err := s.tenants.CreateWithOwner(traceCtx, tenant, member); err != nil {
		return nil, fmt.Errorf("create tenant: %w", err)
	}

	slog.InfoContext(traceCtx, "tenant created",
		"tenant", tenant.ID, "slug", tenant.Slug, "actor", actor.User.ID)
	s.publish(traceCtx, EventTenantCreated)

	return tenant, nil
}

// Get возвращает тенант участнику или глобальному администратору.
func (s *tenantService) Get(ctx context.Context, actor *Principal, tenantID string) (*model.Tenant, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "GetTenant")
	defer spanEnd()

	if err := s.requireMemberOrGlobal(traceCtx, actor, tenantID); err != nil {
		return nil, fmt.Errorf("get tenant: %w", err)
	}

	tenant, err := s.tenants.GetByID(traceCtx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get tenant: %w", err)
	}

	if tenant.Deleted() && !actor.CanAdminAllTenants() {
		return nil, fmt.Errorf("get tenant: %w", database.ErrNotFound)
	}

	return tenant, nil
}

// List возвращает доступные актору тенанты.
func (s *tenantService) List(
	ctx context.Context,
	actor *Principal,
	page model.Page,
) ([]*model.Tenant, string, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "ListTenants")
	defer spanEnd()

	if actor == nil || actor.User == nil {
		return nil, "", fmt.Errorf("list tenants: %w", ErrUnauthorized)
	}

	if actor.CanAdminAllTenants() {
		tenants, err := s.tenants.List(traceCtx, page.Limit+1, page.Offset)
		if err != nil {
			return nil, "", fmt.Errorf("list tenants: %w", err)
		}

		next := page.Next(len(tenants) > page.Limit)
		if len(tenants) > page.Limit {
			tenants = tenants[:page.Limit]
		}

		return tenants, next, nil
	}

	memberships, err := s.tenants.ListMemberships(traceCtx, actor.User.ID)
	if err != nil {
		return nil, "", fmt.Errorf("list tenants: %w", err)
	}

	tenants := make([]*model.Tenant, 0, len(memberships))
	for _, membership := range memberships {
		tenant := membership.Tenant
		tenants = append(tenants, &tenant)
	}

	// Список членств не пагинируется репозиторием: тенантов у пользователя
	// обычно единицы. Окно применяется здесь, чтобы контракт был единым.
	start := min(page.Offset, len(tenants))
	end := min(start+page.Limit, len(tenants))

	window := tenants[start:end]

	return window, page.Next(end < len(tenants)), nil
}

// Delete мягко удаляет тенант и отзывает его сессии.
func (s *tenantService) Delete(ctx context.Context, actor *Principal, tenantID string) error {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "DeleteTenant")
	defer spanEnd()

	if actor == nil || actor.User == nil {
		return fmt.Errorf("delete tenant: %w", ErrUnauthorized)
	}

	if !actor.CanAdminTenant(tenantID) {
		return fmt.Errorf("delete tenant: %w", ErrForbidden)
	}

	tenant, err := s.tenants.GetByID(traceCtx, tenantID)
	if err != nil {
		return fmt.Errorf("delete tenant: %w", err)
	}

	if tenant.Deleted() {
		return nil
	}

	if err := s.tenants.UpdateStatus(traceCtx, tenantID, model.TenantStatusDeleted, s.now()); err != nil {
		return fmt.Errorf("delete tenant: %w", err)
	}

	s.revokeTenantSessions(traceCtx, tenantID)

	slog.InfoContext(traceCtx, "tenant deleted", "tenant", tenantID, "actor", actor.User.ID)
	s.publish(traceCtx, EventTenantStatusChanged)

	return nil
}

// UpdateStatus меняет статус тенанта.
func (s *tenantService) UpdateStatus(
	ctx context.Context,
	actor *Principal,
	tenantID string,
	status model.TenantStatus,
) (*model.Tenant, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "UpdateTenantStatus")
	defer spanEnd()

	if actor == nil || actor.User == nil {
		return nil, fmt.Errorf("update tenant status: %w", ErrUnauthorized)
	}

	if !actor.CanAdminAllTenants() {
		return nil, fmt.Errorf("update tenant status: %w: only global administrators can change tenant status", ErrForbidden)
	}

	if !status.Valid() {
		return nil, fmt.Errorf("update tenant status: %w: status %q", model.ErrInvalidTenant, status)
	}

	if _, err := s.tenants.GetByID(traceCtx, tenantID); err != nil {
		return nil, fmt.Errorf("update tenant status: %w", err)
	}

	if err := s.tenants.UpdateStatus(traceCtx, tenantID, status, s.now()); err != nil {
		return nil, fmt.Errorf("update tenant status: %w", err)
	}

	if status != model.TenantStatusActive {
		s.revokeTenantSessions(traceCtx, tenantID)
	}

	tenant, err := s.tenants.GetByID(traceCtx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("update tenant status: %w", err)
	}

	slog.InfoContext(traceCtx, "tenant status changed",
		"tenant", tenantID, "status", status.String(), "actor", actor.User.ID)
	s.publish(traceCtx, EventTenantStatusChanged)

	return tenant, nil
}

// ListMembers возвращает участников тенанта.
func (s *tenantService) ListMembers(
	ctx context.Context,
	actor *Principal,
	tenantID string,
	page model.Page,
) ([]*model.TenantMemberView, string, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "ListTenantMembers")
	defer spanEnd()

	if err := s.requireStaffOrGlobal(traceCtx, actor, tenantID); err != nil {
		return nil, "", fmt.Errorf("list tenant members: %w", err)
	}

	members, err := s.tenants.ListMembers(traceCtx, tenantID, page.Limit+1, page.Offset)
	if err != nil {
		return nil, "", fmt.Errorf("list tenant members: %w", err)
	}

	next := page.Next(len(members) > page.Limit)
	if len(members) > page.Limit {
		members = members[:page.Limit]
	}

	return members, next, nil
}

// AddMember добавляет пользователя в тенант.
func (s *tenantService) AddMember(
	ctx context.Context,
	actor *Principal,
	tenantID, userID string,
	role model.Role,
) (*model.TenantMemberView, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "AddTenantMember")
	defer spanEnd()

	if _, err := s.requireAdminTenant(traceCtx, actor, tenantID); err != nil {
		return nil, fmt.Errorf("add tenant member: %w", err)
	}

	if !role.Valid() {
		return nil, fmt.Errorf("add tenant member: %w: role %q", model.ErrInvalidRole, role)
	}

	if _, err := s.users.GetByID(traceCtx, userID); err != nil {
		return nil, fmt.Errorf("add tenant member: %w", err)
	}

	member, err := model.NewTenantMember(tenantID, userID, role, false, s.now())
	if err != nil {
		return nil, fmt.Errorf("add tenant member: %w", err)
	}

	if err := s.tenants.AddMember(traceCtx, member); err != nil {
		return nil, fmt.Errorf("add tenant member: %w", err)
	}

	view, err := s.tenants.GetMemberView(traceCtx, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("add tenant member: %w", err)
	}

	slog.InfoContext(traceCtx, "tenant member added",
		"tenant", tenantID, "uuid", userID, "role", role.String(), "actor", actor.User.ID)
	s.publish(traceCtx, EventTenantMemberAdded)

	return view, nil
}

// RemoveMember исключает участника из тенанта.
func (s *tenantService) RemoveMember(
	ctx context.Context,
	actor *Principal,
	tenantID, userID string,
) error {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "RemoveTenantMember")
	defer spanEnd()

	if _, err := s.requireAdminTenant(traceCtx, actor, tenantID); err != nil {
		return fmt.Errorf("remove tenant member: %w", err)
	}

	member, err := s.tenants.GetMember(traceCtx, tenantID, userID)
	if err != nil {
		return fmt.Errorf("remove tenant member: %w", err)
	}

	if member.IsOwner {
		return fmt.Errorf("remove tenant member: %w: tenant owner cannot be removed", ErrForbidden)
	}

	if err := s.tenants.RemoveMember(traceCtx, tenantID, userID); err != nil {
		return fmt.Errorf("remove tenant member: %w", err)
	}

	s.revokeMemberSessions(traceCtx, tenantID, userID)

	slog.InfoContext(traceCtx, "tenant member removed",
		"tenant", tenantID, "uuid", userID, "actor", actor.User.ID)
	s.publish(traceCtx, EventTenantMemberRemoved)

	return nil
}

// ChangeMemberRole меняет роль участника.
func (s *tenantService) ChangeMemberRole(
	ctx context.Context,
	actor *Principal,
	tenantID, userID string,
	role model.Role,
) (*model.TenantMemberView, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "ChangeTenantMemberRole")
	defer spanEnd()

	if _, err := s.requireAdminTenant(traceCtx, actor, tenantID); err != nil {
		return nil, fmt.Errorf("change tenant member role: %w", err)
	}

	if !role.Valid() {
		return nil, fmt.Errorf("change tenant member role: %w: role %q", model.ErrInvalidRole, role)
	}

	member, err := s.tenants.GetMember(traceCtx, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("change tenant member role: %w", err)
	}

	if member.IsOwner {
		return nil, fmt.Errorf("change tenant member role: %w: owner role changes only by ownership transfer", ErrForbidden)
	}

	if err := s.tenants.UpdateMemberRole(traceCtx, tenantID, userID, role); err != nil {
		return nil, fmt.Errorf("change tenant member role: %w", err)
	}

	// Роль читается на каждом запросе, но старые сессии могли закешировать
	// решение в клиенте: отзываем их, чтобы права применились немедленно.
	s.revokeMemberSessions(traceCtx, tenantID, userID)

	view, err := s.tenants.GetMemberView(traceCtx, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("change tenant member role: %w", err)
	}

	slog.InfoContext(traceCtx, "tenant member role changed",
		"tenant", tenantID, "uuid", userID, "role", role.String(), "actor", actor.User.ID)
	s.publish(traceCtx, EventTenantMemberRoleChanged)

	return view, nil
}

// TransferOwnership передаёт владение тенантом.
func (s *tenantService) TransferOwnership(
	ctx context.Context,
	actor *Principal,
	tenantID, userID string,
) (*model.TenantMemberView, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "TransferTenantOwnership")
	defer spanEnd()

	if actor == nil || actor.User == nil {
		return nil, fmt.Errorf("transfer tenant ownership: %w", ErrUnauthorized)
	}

	if !actor.CanAdminTenant(tenantID) {
		return nil, fmt.Errorf("transfer tenant ownership: %w: forbidden", ErrForbidden)
	}

	if !actor.CanAdminAllTenants() {
		current, err := s.tenants.GetMember(traceCtx, tenantID, actor.User.ID)
		if err != nil {
			return nil, fmt.Errorf("transfer tenant ownership: %w", err)
		}

		if !current.IsOwner {
			return nil, fmt.Errorf("transfer tenant ownership: %w: only the tenant owner can transfer ownership", ErrForbidden)
		}
	}

	if _, err := s.tenants.GetMember(traceCtx, tenantID, userID); err != nil {
		return nil, fmt.Errorf("transfer tenant ownership: %w", err)
	}

	if err := s.tenants.TransferOwnership(traceCtx, tenantID, userID); err != nil {
		return nil, fmt.Errorf("transfer tenant ownership: %w", err)
	}

	s.revokeMemberSessions(traceCtx, tenantID, actor.User.ID)
	s.revokeMemberSessions(traceCtx, tenantID, userID)

	view, err := s.tenants.GetMemberView(traceCtx, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("transfer tenant ownership: %w", err)
	}

	slog.InfoContext(traceCtx, "tenant ownership transferred",
		"tenant", tenantID, "from", actor.User.ID, "to", userID)
	s.publish(traceCtx, EventTenantOwnershipTransferred)

	return view, nil
}

// requireMemberOrGlobal проверяет, что актор состоит в тенанте или является
// глобальным администратором.
func (s *tenantService) requireMemberOrGlobal(ctx context.Context, actor *Principal, tenantID string) error {
	if actor == nil || actor.User == nil {
		return ErrUnauthorized
	}

	if actor.CanAdminAllTenants() || actor.TenantID() == tenantID {
		return nil
	}

	if _, err := s.tenants.GetMember(ctx, tenantID, actor.User.ID); err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return ErrForbidden
		}

		return err
	}

	return nil
}

// requireAdminTenant проверяет права администратора тенанта и возвращает его.
func (s *tenantService) requireAdminTenant(
	ctx context.Context,
	actor *Principal,
	tenantID string,
) (*model.Tenant, error) {
	if actor == nil || actor.User == nil {
		return nil, ErrUnauthorized
	}

	if !actor.CanAdminTenant(tenantID) {
		return nil, ErrForbidden
	}

	tenant, err := s.tenants.GetByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	if tenant.Deleted() {
		return nil, database.ErrNotFound
	}

	return tenant, nil
}

// revokeTenantSessions отзывает все сессии тенанта; ошибка не отменяет
// административное действие, но логируется.
func (s *tenantService) revokeTenantSessions(ctx context.Context, tenantID string) {
	if _, err := s.tokens.RevokeByTenant(ctx, tenantID, s.now()); err != nil {
		slog.WarnContext(ctx, "failed to revoke tenant sessions", "tenant", tenantID, "err", err)
	}
}

// revokeMemberSessions отзывает сессии участника в тенанте.
func (s *tenantService) revokeMemberSessions(ctx context.Context, tenantID, userID string) {
	if _, err := s.tokens.RevokeByUserTenant(ctx, userID, tenantID, s.now()); err != nil {
		slog.WarnContext(ctx, "failed to revoke tenant member sessions",
			"tenant", tenantID, "uuid", userID, "err", err)
	}
}

// publish публикует событие, если шина подключена.
func (s *tenantService) publish(ctx context.Context, event string) {
	if s.events == nil {
		return
	}

	s.events.Publish(ctx, event)
}

// requireStaffOrGlobal проверяет, что актор — staff тенанта или глобальный
// администратор.
func (s *tenantService) requireStaffOrGlobal(_ context.Context, actor *Principal, tenantID string) error {
	if actor == nil || actor.User == nil {
		return ErrUnauthorized
	}

	if actor.CanAdminAllTenants() {
		return nil
	}

	if !actor.IsStaff() || actor.TenantID() != tenantID {
		return ErrForbidden
	}

	return nil
}
