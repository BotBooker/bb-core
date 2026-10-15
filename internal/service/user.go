package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/botbooker/bb-core/internal/model"
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
type UserEventPublisher interface {
	Publish(event string)
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
}

// userService — конкретная реализация, скрыта от внешних пакетов.
type userService struct {
	userRepo UserRepository
	events   UserEventPublisher
}

// NewUserService создаёт сервис пользователей.
func NewUserService(userRepo UserRepository, events UserEventPublisher) UserService {
	slog.Debug("user service created")

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

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	slog.InfoContext(ctx, "user created", "uuid", user.ID, "actor", current.ID)
	s.publish(EventUserCreated)

	return user, nil
}

// Get возвращает пользователя по идентификатору.
//
// Формат идентификатора проверяется до запроса в базу: PostgreSQL на строку,
// не похожую на UUID, ответил бы ошибкой приведения типа, а не «не найдено».
// Чужой профиль доступен только сотруднику (оператору или администратору).
func (s *userService) Get(ctx context.Context, actor *Principal, id string) (*model.User, error) {
	if _, err := principalUser(actor); err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	if err := model.ValidateUUID(id); err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	user, err := s.userRepo.GetByID(ctx, id)
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
	current, err := principalUser(actor)
	if err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}

	user, err := s.Get(ctx, actor, id)
	if err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}

	if !actor.CanUpdate(user) {
		return nil, fmt.Errorf("update user: %w: role %q", ErrForbidden, actor.Role)
	}

	if err := user.ApplyUpdate(in, time.Now()); err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}

	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}

	slog.InfoContext(ctx, "user updated", "uuid", user.ID, "actor", current.ID)
	s.publish(EventUserUpdated)

	return user, nil
}

// Delete удаляет пользователя. Повторное удаление даёт database.ErrNotFound:
// клиент узнаёт, что удалять было нечего.
//
// Удалять может только администратор, поэтому запись заранее не читается:
// отдельный SELECT не дал бы ничего, а лишний запрос в базу — дал.
func (s *userService) Delete(ctx context.Context, actor *Principal, id string) error {
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

	if err := s.userRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	slog.InfoContext(ctx, "user deleted", "uuid", id, "actor", current.ID)
	s.publish(EventUserDeleted)

	return nil
}

// publish публикует событие, если шина подключена: сервис не должен падать
// из-за отсутствия подписчиков (в тестах или на раннем этапе сборки).
func (s *userService) publish(event string) {
	if s.events == nil {
		return
	}

	s.events.Publish(event)
}
