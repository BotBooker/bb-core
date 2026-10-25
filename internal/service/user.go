package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/botbooker/bb-core/internal/domain"
	"github.com/botbooker/bb-core/internal/model"
	"github.com/botbooker/bb-core/pkg/observability"
)

// UserEventPublisher — интерфейс для публикации событий пользователей.
// Метод принимает контекст: через него в запись лога добавляется trace_id
// активной трассы (см. pkg/logger.TraceHandler).
type UserEventPublisher interface {
	Publish(ctx context.Context, event string)
}

// События жизненного цикла пользователя: их публикует сервис после успешной
// операции, чтобы подписчики (уведомления, аудит) не зависели от транспорта.
const (
	// EventUserCreated — пользователь создан.
	EventUserCreated = "user.created"
	// EventUserUpdated — профиль пользователя обновлён.
	EventUserUpdated = "user.updated"
	// EventUserDeleted — пользователь удалён.
	EventUserDeleted = "user.deleted"
	// EventGlobalOperatorAssigned — пользователь назначен глобальным оператором.
	EventGlobalOperatorAssigned = "user.global_operator_assigned"
	// EventGlobalOperatorRevoked — с пользователя сняты права глобального
	// оператора.
	EventGlobalOperatorRevoked = "user.global_operator_revoked"
)

// UserService — интерфейс сервиса пользователей.
//
// Каждый метод принимает действующего пользователя (actor): права проверяются
// в сервисе, а не в транспорте, поэтому правила одинаковы для HTTP и gRPC и
// покрываются одними тестами. Ошибка прав — service.ErrForbidden, отсутствие
// действующего пользователя — service.ErrUnauthorized.
type UserService interface {
	// Create создаёт пользователя. Идентификатор (UUIDv4) и роль client
	// проставляет модель, поэтому в API они не задаются.
	Create(ctx context.Context, actor *Principal, in model.CreateUserInput) (*model.User, error)
	// Get возвращает пользователя по идентификатору: свой профиль или, если
	// actor — сотрудник, пользователя своего тенанта в пределах видимости.
	Get(ctx context.Context, actor *Principal, id string) (*model.User, error)
	// List возвращает пользователей в области видимости actor.
	List(ctx context.Context, actor *Principal, page model.Page) ([]*model.User, string, error)
	// Update применяет частичное обновление профиля: свой профиль или, если
	// actor — администратор, любой.
	Update(ctx context.Context, actor *Principal, id string, in model.UpdateUserInput) (*model.User, error)
	// Delete удаляет пользователя: доступно только администратору.
	Delete(ctx context.Context, actor *Principal, id string) error
	// AssignGlobalOperator назначает пользователя глобальным оператором: он
	// сможет администрировать все тенанты. Доступно только суперпользователю.
	AssignGlobalOperator(ctx context.Context, actor *Principal, id string) (*model.User, error)
	// RevokeGlobalOperator снимает права глобального оператора. Доступно только
	// суперпользователю; суперпользователя изменить нельзя.
	RevokeGlobalOperator(ctx context.Context, actor *Principal, id string) error
}

// userService — конкретная реализация, скрыта от внешних пакетов.
type userService struct {
	userRepo       domain.UserRepository
	tenantRepo     domain.TenantRepository
	credentialRepo domain.CredentialRepository
	tokenRepo      domain.TokenRepository
	events         UserEventPublisher
	// uow — транзакционная граница создания и удаления пользователя:
	// профиль, членство, учётные данные и сессии меняются целиком.
	uow domain.UnitOfWork
}

// NewUserService создаёт сервис пользователей.
//
// Помимо профилей сервису нужны членство (созданный сотрудником пользователь
// сразу становится клиентом его тенанта), учётные данные и сессии (удаление
// аккаунта закрывает доступ) и UnitOfWork (общая транзакция сценария).
func NewUserService(
	ctx context.Context,
	userRepo domain.UserRepository,
	tenantRepo domain.TenantRepository,
	credentialRepo domain.CredentialRepository,
	tokenRepo domain.TokenRepository,
	events UserEventPublisher,
	uow domain.UnitOfWork,
) UserService {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "NewUserService")
	defer spanEnd()

	slog.DebugContext(traceCtx, "user service created")

	return &userService{
		userRepo:       userRepo,
		tenantRepo:     tenantRepo,
		credentialRepo: credentialRepo,
		tokenRepo:      tokenRepo,
		events:         events,
		uow:            uow,
	}
}

// principalUser возвращает профиль действующего пользователя.
//
// Отсутствие actor (или профиля в нём) означает, что запрос не прошёл
// аутентификацию: транспорт обязан был ответить 401, но сервис не полагается на
// это и отвечает ErrUnauthorized сам — иначе проверка прав упала бы с паникой.
func principalUser(actor *Principal) (*model.User, error) {
	if actor == nil || actor.User == nil {
		return nil, fmt.Errorf("%w: request has no principal", ErrUnauthorized)
	}

	return actor.User, nil
}

// Create создаёт пользователя от имени actor.
//
// Валидацию входа делает модель (те же правила, что в proto): при ошибке
// пользователь не создаётся и репозиторий не вызывается. Конфликт уникальности
// email или телефона приходит из репозитория как domain.ErrDuplicate.
//
// Создавать пользователей может сотрудник: клиент регистрируется сам
// (POST /auth/register), поэтому роль и пароль через этот метод не задаются.
//
// Профиль и членство в тенанте актора создаются одной транзакцией
// (domain.UnitOfWork): иначе при сбое второй вставки остался бы пользователь,
// занявший email, но не попавший в тенант.
func (s *userService) Create(
	ctx context.Context,
	actor *Principal,
	in model.CreateUserInput,
) (*model.User, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "Create")
	defer spanEnd()

	current, err := principalUser(actor)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	if !actor.CanCreate() {
		return nil, fmt.Errorf("create user: %w: role %q", ErrForbidden, actor.Role)
	}

	now := time.Now()

	user, err := model.NewUser(in, now)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	tenantID := actor.TenantID()
	if tenantID == "" {
		return nil, fmt.Errorf("create user: %w: actor has no tenant", ErrUnauthorized)
	}

	member, err := model.NewTenantMember(tenantID, user.ID, model.RoleClient, false, now)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	err = s.uow.Do(traceCtx, func(txCtx context.Context) error {
		if createErr := s.userRepo.Create(txCtx, user); createErr != nil {
			return createErr
		}

		return s.tenantRepo.AddMember(txCtx, member)
	})
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	slog.InfoContext(traceCtx, "user created",
		"uuid", user.ID, "tenant", tenantID, "actor", current.ID)
	s.publish(traceCtx, EventUserCreated)

	return user, nil
}

// Get возвращает пользователя по идентификатору.
//
// Формат идентификатора проверяется до запроса в базу: PostgreSQL на строку,
// не похожую на UUID, ответил бы ошибкой приведения типа, а не «не найдено».
//
// Область видимости: свой профиль — всегда, глобальный администратор — любой,
// staff — только участник своего тенанта (клиент — при наличии брони, см.
// repository.UserRepo.ListVisible), остальные — только себя.
func (s *userService) Get(ctx context.Context, actor *Principal, id string) (*model.User, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "Get")
	defer spanEnd()

	current, err := principalUser(actor)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	if err := model.ValidateUUID(id); err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	if current.ID == id {
		return current, nil
	}

	if actor.CanAdminAllTenants() {
		user, getErr := s.userRepo.GetByID(traceCtx, id)
		if getErr != nil {
			return nil, fmt.Errorf("get user: %w", getErr)
		}

		return user, nil
	}

	if !actor.IsStaff() {
		return nil, fmt.Errorf("get user: %w: only staff can read other profiles", ErrForbidden)
	}

	user, err := s.userRepo.GetVisible(traceCtx, actor.TenantID(), id)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	return user, nil
}

// List возвращает пользователей в области видимости actor.
func (s *userService) List(
	ctx context.Context,
	actor *Principal,
	page model.Page,
) ([]*model.User, string, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "List")
	defer spanEnd()

	current, err := principalUser(actor)
	if err != nil {
		return nil, "", fmt.Errorf("list users: %w", err)
	}

	switch {
	case actor.CanAdminAllTenants():
		users, listErr := s.userRepo.ListAll(traceCtx, page.Limit+1, page.Offset)
		if listErr != nil {
			return nil, "", fmt.Errorf("list users: %w", listErr)
		}

		return trimUsers(users, page)
	case actor.IsStaff():
		users, listErr := s.userRepo.ListVisible(traceCtx, actor.TenantID(), page.Limit+1, page.Offset)
		if listErr != nil {
			return nil, "", fmt.Errorf("list users: %w", listErr)
		}

		return trimUsers(users, page)
	default:
		// Клиент видит только себя: список из одного элемента, следующей
		// страницы нет.
		if page.Offset > 0 {
			return []*model.User{}, "", nil
		}

		return []*model.User{current}, "", nil
	}
}

// trimUsers применяет окно к списку пользователей.
func trimUsers(users []*model.User, page model.Page) ([]*model.User, string, error) {
	hasMore := len(users) > page.Limit
	if hasMore {
		users = users[:page.Limit]
	}

	return users, page.Next(hasMore), nil
}

// Update применяет частичное обновление профиля.
//
// Сначала запись читается: обновление применяется к ней целиком, поэтому
// непереданные поля сохраняют прежние значения, а UPDATE не создаёт «пустую»
// строку. Отсутствие пользователя даёт database.ErrNotFound без записи в базу.
//
// Менять чужой профиль может только администратор: свой профиль доступен
// клиенту и оператору без дополнительных прав.
func (s *userService) Update(
	ctx context.Context,
	actor *Principal,
	id string,
	in model.UpdateUserInput,
) (*model.User, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "Update")
	defer spanEnd()

	current, err := principalUser(actor)
	if err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}

	user, err := s.Get(traceCtx, actor, id)
	if err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}

	if !actor.CanUpdate(user) {
		return nil, fmt.Errorf("update user: %w: role %q", ErrForbidden, actor.Role)
	}

	if err := user.ApplyUpdate(in, time.Now()); err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}

	if err := s.userRepo.Update(traceCtx, user); err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}

	slog.InfoContext(traceCtx, "user updated", "uuid", user.ID, "actor", current.ID)
	s.publish(traceCtx, EventUserUpdated)

	return user, nil
}

// Delete удаляет аккаунт пользователя.
//
// Удаление аккаунта — операция системного уровня: таблица users общая для всех
// тенантов, поэтому администратор тенанта не может закрыть чужой аккаунт —
// иначе он отнял бы доступ и в остальных тенантах. Исключение участника из
// тенанта — отдельная операция TenantService.RemoveMember.
//
// В одной транзакции: мягкое удаление профиля, деактивация учётных данных и
// отзыв всех сессий. Иначе пароль остался бы действующим, а токены — рабочими
// до истечения срока. Повторное удаление даёт domain.ErrNotFound: клиент
// узнаёт, что удалять было нечего.
func (s *userService) Delete(ctx context.Context, actor *Principal, id string) error {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "Delete")
	defer spanEnd()

	current, err := principalUser(actor)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	if !actor.CanDelete() {
		return fmt.Errorf("delete user: %w: role %q", ErrForbidden, actor.Role)
	}

	if err := model.ValidateUUID(id); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	if !actor.CanAdminAllTenants() {
		return fmt.Errorf("delete user: %w: only global administrators can delete accounts;"+
			" use tenant member removal instead", ErrForbidden)
	}

	now := time.Now()

	err = s.uow.Do(traceCtx, func(txCtx context.Context) error {
		if deleteErr := s.userRepo.Delete(txCtx, id); deleteErr != nil {
			return deleteErr
		}

		if credErr := s.credentialRepo.DeactivateByUser(txCtx, id, now); credErr != nil {
			return credErr
		}

		if _, revokeErr := s.tokenRepo.RevokeByUser(txCtx, id, now); revokeErr != nil {
			return revokeErr
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	slog.InfoContext(traceCtx, "user deleted", "uuid", id, "actor", current.ID)
	s.publish(traceCtx, EventUserDeleted)

	return nil
}

// AssignGlobalOperator назначает пользователя глобальным оператором.
//
// Назначать может только суперпользователь: глобальные операторы управляют
// всеми тенантами, поэтому выдача такого права — исключительно его компетенция.
// Суперпользователя менять нельзя: он уже администрирует все тенанты, и снятие
// флага global operator ничего бы не изменило. Повторное назначение
// идемпотентно.
func (s *userService) AssignGlobalOperator(
	ctx context.Context,
	actor *Principal,
	id string,
) (*model.User, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "AssignGlobalOperator")
	defer spanEnd()

	current, err := principalUser(actor)
	if err != nil {
		return nil, fmt.Errorf("assign global operator: %w", err)
	}

	if !actor.IsSuperuser() {
		return nil, fmt.Errorf("assign global operator: %w: role %q", ErrForbidden, actor.Role)
	}

	if err := model.ValidateUUID(id); err != nil {
		return nil, fmt.Errorf("assign global operator: %w", err)
	}

	user, err := s.userRepo.GetByID(traceCtx, id)
	if err != nil {
		return nil, fmt.Errorf("assign global operator: %w", err)
	}

	if user.IsSuperuser {
		return nil, fmt.Errorf("assign global operator: %w: target is superuser", ErrForbidden)
	}

	if user.IsGlobalOperator {
		return user, nil
	}

	user.IsGlobalOperator = true
	user.UpdatedAt = time.Now().UTC()

	if err := s.userRepo.Update(traceCtx, user); err != nil {
		return nil, fmt.Errorf("assign global operator: %w", err)
	}

	slog.InfoContext(traceCtx, "global operator assigned", "uuid", user.ID, "actor", current.ID)
	s.publish(traceCtx, EventGlobalOperatorAssigned)

	return user, nil
}

// RevokeGlobalOperator снимает права глобального оператора.
//
// Снимать права может только суперпользователь. Суперпользователя менять
// нельзя: его права не зависят от флага global operator. Повторное снятие
// идемпотентно и не пишет в базу.
func (s *userService) RevokeGlobalOperator(ctx context.Context, actor *Principal, id string) error {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "RevokeGlobalOperator")
	defer spanEnd()

	current, err := principalUser(actor)
	if err != nil {
		return fmt.Errorf("revoke global operator: %w", err)
	}

	if !actor.IsSuperuser() {
		return fmt.Errorf("revoke global operator: %w: role %q", ErrForbidden, actor.Role)
	}

	if err := model.ValidateUUID(id); err != nil {
		return fmt.Errorf("revoke global operator: %w", err)
	}

	user, err := s.userRepo.GetByID(traceCtx, id)
	if err != nil {
		return fmt.Errorf("revoke global operator: %w", err)
	}

	if user.IsSuperuser {
		return fmt.Errorf("revoke global operator: %w: target is superuser", ErrForbidden)
	}

	if !user.IsGlobalOperator {
		return nil
	}

	user.IsGlobalOperator = false
	user.UpdatedAt = time.Now().UTC()

	if err := s.userRepo.Update(traceCtx, user); err != nil {
		return fmt.Errorf("revoke global operator: %w", err)
	}

	slog.InfoContext(traceCtx, "global operator revoked", "uuid", user.ID, "actor", current.ID)
	s.publish(traceCtx, EventGlobalOperatorRevoked)

	return nil
}

// publish публикует событие, если шина подключена: сервис не должен падать
// из-за отсутствия подписчиков (в тестах или на раннем этапе сборки).
func (s *userService) publish(ctx context.Context, event string) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "publish")
	defer spanEnd()

	if s.events == nil {
		return
	}

	s.events.Publish(traceCtx, event)
}
