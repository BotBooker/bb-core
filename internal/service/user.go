package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/botbooker/bb-core/internal/model"
	"github.com/botbooker/bb-core/pkg/observability"
)

// UserRepository — интерфейс репозитория пользователей для UserService.
//
// Объявлен узко: сервису нужны только эти операции, хотя репозиторий умеет
// больше (например, поиск по email для аутентификации).
type UserRepository interface {
	Create(ctx context.Context, user *model.User) error
	GetByID(ctx context.Context, id string) (*model.User, error)
	Update(ctx context.Context, user *model.User) error
	Delete(ctx context.Context, id string) error
}

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
	// actor — сотрудник, любой.
	Get(ctx context.Context, actor *Principal, id string) (*model.User, error)
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
	userRepo UserRepository
	events   UserEventPublisher
}

// NewUserService создаёт сервис пользователей.
func NewUserService(ctx context.Context, userRepo UserRepository, events UserEventPublisher) UserService {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "NewUserService")
	defer spanEnd()

	slog.DebugContext(traceCtx, "user service created")

	return &userService{userRepo: userRepo, events: events}
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
// email или телефона приходит из репозитория как database.ErrDuplicate.
//
// Создавать пользователей может сотрудник: клиент регистрируется сам
// (POST /auth/register), поэтому роль и пароль через этот метод не задаются.
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
	user, err := model.NewUser(in, time.Now())
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	if err := s.userRepo.Create(traceCtx, user); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	slog.InfoContext(traceCtx, "user created", "uuid", user.ID, "actor", current.ID)
	s.publish(traceCtx, EventUserCreated)

	return user, nil
}

// Get возвращает пользователя по идентификатору.
//
// Формат идентификатора проверяется до запроса в базу: PostgreSQL на строку,
// не похожую на UUID, ответил бы ошибкой приведения типа, а не «не найдено».
// Чужой профиль доступен только сотруднику (оператору или администратору).
func (s *userService) Get(ctx context.Context, actor *Principal, id string) (*model.User, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "Get")
	defer spanEnd()

	if _, err := principalUser(actor); err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	if err := model.ValidateUUID(id); err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	user, err := s.userRepo.GetByID(traceCtx, id)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	if !actor.CanRead(user) {
		return nil, fmt.Errorf("get user: %w: role %q", ErrForbidden, actor.Role)
	}

	return user, nil
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

// Delete удаляет пользователя. Повторное удаление даёт database.ErrNotFound:
// клиент узнаёт, что удалять было нечего.
//
// Удалять может только администратор, поэтому запись заранее не читается:
// отдельный SELECT не дал бы ничего, а лишний запрос в базу — дал.
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

	if err := s.userRepo.Delete(traceCtx, id); err != nil {
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
