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
	// now — часы сервиса: поле, а не прямой вызов time.Now, чтобы тесты не
	// зависели от реального времени.
	now func() time.Time
}

// NewBootstrapService создаёт сервис первичной инициализации.
func NewBootstrapService(userRepo AuthUserRepository, credentialRepo AuthCredentialRepository) BootstrapService {
	slog.Debug("bootstrap service created")

	return &bootstrapService{userRepo: userRepo, credentialRepo: credentialRepo, now: time.Now}
}

// EnsureSuperuser создаёт первого суперпользователя, если его ещё нет.
//
// Наличие проверяется по email: повторный запуск bootstrap — обычная ситуация
// (например, при раскатке), и он не должен ни падать, ни сбрасывать пароль уже
// созданного администратора. Гонку двух экземпляров разрешает уникальный индекс
// email: проигравший вставку считает суперпользователя созданным.
func (s *bootstrapService) EnsureSuperuser(ctx context.Context, in BootstrapInput) (*model.User, bool, error) {
	if err := validateBootstrapInput(in); err != nil {
		return nil, false, err
	}

	email := model.NormalizeEmail(in.Email)

	existing, err := s.userRepo.GetByEmail(ctx, email)

	switch {
	case err == nil:
		slog.InfoContext(ctx, "superuser already exists", "uuid", existing.ID, "email", existing.Email)

		return existing, false, nil
	case errors.Is(err, database.ErrNotFound):
		// Пользователя нет — создаём его ниже.
	default:
		return nil, false, fmt.Errorf("bootstrap superuser: %w", err)
	}

	now := s.now()

	user, err := model.NewSuperuser(model.CreateUserInput{
		Email:     in.Email,
		FirstName: in.FirstName,
		LastName:  in.LastName,
	}, now)
	if err != nil {
		return nil, false, fmt.Errorf("bootstrap superuser: %w", err)
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		if errors.Is(err, database.ErrDuplicate) {
			// Параллельный bootstrap успел создать того же пользователя:
			// считаем задачу выполненной и возвращаем существующего.
			return s.ensureSuperuserAfterRace(ctx, email)
		}

		return nil, false, fmt.Errorf("bootstrap superuser: %w", err)
	}

	credentials, err := model.NewCredentials(user.ID, in.Password, now)
	if err == nil {
		err = s.credentialRepo.Create(ctx, credentials)
	}

	if err != nil {
		// Пользователь без пароля бесполезен и занял бы email: откатываем его
		// (soft delete), чтобы bootstrap можно было повторить.
		s.rollback(ctx, user.ID)

		return nil, false, fmt.Errorf("bootstrap superuser: %w", err)
	}

	slog.InfoContext(ctx, "superuser created", "uuid", user.ID, "email", user.Email)

	return user, true, nil
}

// ensureSuperuserAfterRace читает пользователя, которого создал параллельный
// экземпляр bootstrap, и возвращает его как уже существующего.
func (s *bootstrapService) ensureSuperuserAfterRace(ctx context.Context, email string) (*model.User, bool, error) {
	existing, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		return nil, false, fmt.Errorf("bootstrap superuser: %w", err)
	}

	slog.InfoContext(ctx, "superuser created concurrently", "uuid", existing.ID, "email", existing.Email)

	return existing, false, nil
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
