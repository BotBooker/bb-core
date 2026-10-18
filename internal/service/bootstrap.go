package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/model"
)

// ErrInvalidBootstrap — не заданы обязательные данные первого суперпользователя.
// Это ошибка конфигурации (переменные окружения), а не запроса пользователя.
var ErrInvalidBootstrap = errors.New("bootstrap configuration is invalid")

// BootstrapInput — данные первого суперпользователя.
//
// Пароль приходит из окружения (config.Bootstrap) и в открытом виде нигде не
// сохраняется: в базу попадает только его хеш (model.NewCredentials).
type BootstrapInput struct {
	Email     string
	Password  string
	FirstName string
	LastName  string
}

// BootstrapService — первичная инициализация системы.
type BootstrapService interface {
	// EnsureSuperuser создаёт первого суперпользователя, если его ещё нет.
	// Второе возвращаемое значение — признак того, что пользователь создан
	// (false означает, что он уже существовал). Повторный вызов безопасен.
	EnsureSuperuser(ctx context.Context, in BootstrapInput) (*model.User, bool, error)
}

// bootstrapService — конкретная реализация, скрыта от внешних пакетов.
type bootstrapService struct {
	userRepo       AuthUserRepository
	credentialRepo AuthCredentialRepository
	tenantRepo     AuthTenantRepository
	// passwordTTL — срок жизни пароля (config.Auth().PasswordTTL): попадает в
	// expires_at учётных данных суперпользователя.
	passwordTTL time.Duration
	// now — часы сервиса: поле, а не прямой вызов time.Now, чтобы тесты не
	// зависели от реального времени.
	now func() time.Time
}

// NewBootstrapService создаёт сервис первичной инициализации.
func NewBootstrapService(
	ctx context.Context,
	userRepo AuthUserRepository,
	credentialRepo AuthCredentialRepository,
	tenantRepo AuthTenantRepository,
	passwordTTL time.Duration,
) BootstrapService {
	slog.DebugContext(ctx, "bootstrap service created")

	return &bootstrapService{
		userRepo:       userRepo,
		credentialRepo: credentialRepo,
		tenantRepo:     tenantRepo,
		passwordTTL:    passwordTTL,
		now:            time.Now,
	}
}

// EnsureSuperuser создаёт первого суперпользователя, если его ещё нет.
//
// Наличие проверяется по email: повторный запуск bootstrap — обычная ситуация
// (например, при раскатке), и он не должен ни падать, ни сбрасывать пароль уже
// созданного администратора. Системный флаг is_superuser при этом гарантируется:
// профиль, созданный до появления флага, получает права на все тенанты. Гонку
// двух экземпляров разрешает уникальный индекс email: проигравший вставку
// считает суперпользователя созданным и тоже выставляет флаг.
func (s *bootstrapService) EnsureSuperuser(ctx context.Context, in BootstrapInput) (*model.User, bool, error) {
	if err := validateBootstrapInput(in); err != nil {
		return nil, false, err
	}

	email := model.NormalizeEmail(in.Email)

	existing, err := s.userRepo.GetByEmail(ctx, email)

	switch {
	case err == nil:
		// Пользователь уже есть: повторный bootstrap не сбрасывает пароль, но
		// гарантирует системный флаг суперпользователя (он мог быть создан до
		// появления is_superuser).
		return s.markSuperuser(ctx, existing)
	case errors.Is(err, database.ErrNotFound):
		// Пользователя нет — создаём его ниже.
	default:
		return nil, false, fmt.Errorf("bootstrap superuser: %w", err)
	}

	now := s.now()

	user, err := model.NewUser(model.CreateUserInput{
		Email:     in.Email,
		FirstName: in.FirstName,
		LastName:  in.LastName,
	}, now)
	if err != nil {
		return nil, false, fmt.Errorf("bootstrap superuser: %w", err)
	}

	// Суперпользователь — системный уровень доступа: он управляет всеми
	// объектами и тенантами, поэтому флаг ставится вместе с профилем, а не
	// через членство в тенанте.
	user.IsSuperuser = true

	if err := s.userRepo.Create(ctx, user); err != nil {
		if errors.Is(err, database.ErrDuplicate) {
			// Параллельный bootstrap успел создать того же пользователя:
			// считаем задачу выполненной и возвращаем существующего.
			return s.ensureSuperuserAfterRace(ctx, email)
		}

		return nil, false, fmt.Errorf("bootstrap superuser: %w", err)
	}

	credentials, err := model.NewCredentials(user.ID, user.Email, in.Password, s.passwordTTL, now)
	if err == nil {
		err = s.credentialRepo.Create(ctx, credentials)
	}

	if err == nil {
		// Роль в профиле не хранится: администратором суперпользователь
		// становится через членство в личном тенанте (tenant_members.role_id).
		err = s.createAdminTenant(ctx, user, now)
	}

	if err != nil {
		// Пользователь без пароля или без тенанта бесполезен и занял бы email:
		// откатываем его (soft delete), чтобы bootstrap можно было повторить.
		s.rollback(ctx, user.ID)

		return nil, false, fmt.Errorf("bootstrap superuser: %w", err)
	}

	slog.InfoContext(ctx, "superuser created", "uuid", user.ID, "email", user.Email)

	return user, true, nil
}

// createAdminTenant создаёт личный тенант суперпользователя и членство в нём с
// ролью admin.
//
// Тенант нужен, потому что сессия действует в рамках одного тенанта
// (user_tokens.tenant_id NOT NULL), а роль — атрибут членства: без членства
// суперпользователь не смог бы войти. Права на все тенанты даёт системный флаг
// is_superuser, а роль admin в личном тенанте делает bootstrap-администратора
// полноценным администратором и без него. Владельцем тенанта становится он сам
// (is_owner = true).
func (s *bootstrapService) createAdminTenant(ctx context.Context, user *model.User, now time.Time) error {
	tenant, err := model.NewTenant(user.FullName(), personalTenantSlug(user.ID), now)
	if err != nil {
		return fmt.Errorf("create superuser tenant: %w", err)
	}

	if err := s.tenantRepo.Create(ctx, tenant); err != nil {
		return fmt.Errorf("create superuser tenant: %w", err)
	}

	member, err := model.NewTenantMember(tenant.ID, user.ID, model.RoleAdmin, true, now)
	if err != nil {
		return fmt.Errorf("create superuser membership: %w", err)
	}

	if err := s.tenantRepo.AddMember(ctx, member); err != nil {
		return fmt.Errorf("create superuser membership: %w", err)
	}

	slog.InfoContext(ctx, "superuser tenant created", "tenant", tenant.ID, "uuid", user.ID)

	return nil
}

// ensureSuperuserAfterRace читает пользователя, которого создал параллельный
// экземпляр bootstrap, и возвращает его как уже существующего.
func (s *bootstrapService) ensureSuperuserAfterRace(ctx context.Context, email string) (*model.User, bool, error) {
	existing, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		return nil, false, fmt.Errorf("bootstrap superuser: %w", err)
	}

	slog.InfoContext(ctx, "superuser created concurrently", "uuid", existing.ID, "email", existing.Email)

	return s.markSuperuser(ctx, existing)
}

// markSuperuser гарантирует системный флаг суперпользователя у найденного
// профиля.
//
// Нужен для идемпотентности: bootstrap мог выполняться до появления колонки
// is_superuser, а также проиграть гонку другому экземпляру, который успел
// создать профиль. Пароль и членство при этом не меняются.
func (s *bootstrapService) markSuperuser(ctx context.Context, user *model.User) (*model.User, bool, error) {
	if user.IsSuperuser {
		slog.InfoContext(ctx, "superuser already exists", "uuid", user.ID, "email", user.Email)

		return user, false, nil
	}

	user.IsSuperuser = true
	user.UpdatedAt = s.now().UTC()

	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, false, fmt.Errorf("bootstrap superuser: %w", err)
	}

	slog.InfoContext(ctx, "superuser promoted", "uuid", user.ID, "email", user.Email)

	return user, false, nil
}

// rollback убирает пользователя после неудачного создания учётных данных.
// Ошибку логируем: она означает, что остался администратор без пароля.
func (s *bootstrapService) rollback(ctx context.Context, id string) {
	if err := s.userRepo.Delete(ctx, id); err != nil {
		slog.ErrorContext(ctx, "failed to roll back superuser creation", "uuid", id, "err", err)
	}
}

// validateBootstrapInput проверяет обязательные данные суперпользователя.
// Email и пароль задают переменными окружения, поэтому их отсутствие — ошибка
// конфигурации, а не пустой запрос.
func validateBootstrapInput(in BootstrapInput) error {
	if strings.TrimSpace(in.Email) == "" {
		return fmt.Errorf("%w: superuser email is not set", ErrInvalidBootstrap)
	}

	if in.Password == "" {
		return fmt.Errorf("%w: superuser password is not set", ErrInvalidBootstrap)
	}

	return nil
}
