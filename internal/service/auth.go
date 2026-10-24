package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/botbooker/bb-core/internal/domain"
	"github.com/botbooker/bb-core/internal/model"
)

// Ошибки авторизации: транспорт переводит их в 401 и 403.
//
// Тексты намеренно не уточняют, что именно не подошло: по ответам нельзя
// выяснить, зарегистрирован адрес или нет, и есть ли у пользователя пароль.
var (
	// ErrUnauthorized — действующего токена нет: он не передан, подпись не
	// сходится, срок истёк, сессия отозвана или пользователь удалён.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrInvalidCredentials — email и пароль не совпали.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrForbidden — пользователь известен, но прав на операцию нет.
	ErrForbidden = errors.New("forbidden")
	// ErrPasswordExpired — срок действия пароля истёк: вход разрешён только для
	// смены пароля. Значение совпадает с model.ErrPasswordExpired — сервис не
	// заводит второй сигнал для того же состояния, а транспорт переводит его в
	// 403 с признаком password_expired.
	ErrPasswordExpired = model.ErrPasswordExpired
)

// _dummyPasswordHash — хеш случайного пароля, которого нет ни у одного
// пользователя.
//
// Нужен, чтобы выравнивать время ответа на вход: если пользователя с таким
// email нет (или у него не задан пароль), проверка пароля всё равно
// выполняется. Иначе по задержке ответа было бы видно, какие адреса
// зарегистрированы. Пароль этого хеша нигде не сохранён, поэтому проверка по
// нему всегда даёт ErrPasswordMismatch.
// #nosec G101
const _dummyPasswordHash = "argon2id$v=19$m=19456,t=2,p=1$vnTtR/4LeIqz3A2E/cJJPQ$PK+j63/BT4gsmwv+pe819IAXaPFuDho4W/TS0xbdYrg"

// AuthTokenCodec подписывает и проверяет токены. Реализация — internal/jwt:
// сервису не важно, как устроен формат, ему нужны подпись и разбор.
type AuthTokenCodec interface {
	Sign(token *model.Token) (string, error)
	Parse(raw string, now time.Time) (*model.Token, error)
}

// AuthEventPublisher — публикация событий авторизации.
// Метод принимает контекст: через него в запись лога добавляется trace_id
// активной трассы (см. pkg/logger.TraceHandler).
type AuthEventPublisher interface {
	Publish(ctx context.Context, event string)
}

// События авторизации: их публикует сервис после успешной операции, чтобы
// подписчики (аудит, уведомления) не зависели от транспорта.
const (
	// EventUserRegistered — пользователь зарегистрировался сам.
	EventUserRegistered = "user.registered"
	// EventUserLoggedIn — пользователь вошёл в систему.
	EventUserLoggedIn = "user.logged_in"
	// EventUserLoggedOut — пользователь вышел из системы.
	EventUserLoggedOut = "user.logged_out"
	// EventUserPasswordChanged — пользователь сменил пароль.
	EventUserPasswordChanged = "user.password_changed"
)

// tokenTouchInterval — как часто обновляется время последнего использования
// токена: писать в базу на каждый запрос незачем, а точности в час для аудита
// достаточно.
const tokenTouchInterval = time.Hour

// Session — выданный доступ: подписанный токен и его запись в базе.
type Session struct {
	// AccessToken — подписанный JWT: его клиент присылает в Authorization.
	AccessToken string
	// Token — запись сессии: по ней проверяются отзыв и срок действия.
	Token *model.Token
	// Tenant — тенант сессии: нужен для ответа клиенту (slug и название).
	Tenant *model.Tenant
	// Role — роль пользователя в тенанте сессии.
	Role model.Role
}

// AuthService — интерфейс сервиса авторизации.
type AuthService interface {
	// Register создаёт пользователя с паролем и сразу выдаёт ему токен:
	// отдельный вход после регистрации не нужен.
	Register(ctx context.Context, in model.RegisterInput) (*model.User, *Session, error)
	// Login проверяет email и пароль и выдаёт токен.
	//
	// Если срок действия пароля истёк, токен не выдаётся: возвращается
	// ErrPasswordExpired, и пользователь может только сменить пароль.
	// tenantSlug выбирает тенант; пустое значение — самое раннее членство.
	Login(ctx context.Context, email, password, tenantSlug string) (*Session, error)
	// ChangePassword меняет пароль пользователя и выдаёт новую сессию.
	//
	// Личность подтверждается текущим паролем, а не токеном: сменить пароль
	// нужно и тем, у кого срок действия пароля истёк, — а им токен не выдаётся.
	ChangePassword(ctx context.Context, email, currentPassword, newPassword, tenantSlug string) (*Session, error)
	// SwitchTenant выдаёт новую сессию в тенанте tenantSlug, проверяя членство.
	// Старый токен не отзывается: для полного выхода есть LogoutAll.
	SwitchTenant(ctx context.Context, actor *Principal, tenantSlug string) (*Session, error)
	// ListMyTenants возвращает действующие членства пользователя.
	ListMyTenants(ctx context.Context, actor *Principal) ([]*model.TenantMembership, error)
	// LogoutAll отзывает все сессии пользователя во всех тенантах.
	LogoutAll(ctx context.Context, actor *Principal) error
	// Logout отзывает токен — это и есть выход из системы.
	Logout(ctx context.Context, accessToken string) error
	// Authenticate проверяет токен и возвращает действующего пользователя.
	Authenticate(ctx context.Context, accessToken string) (*Principal, error)
}

// authService — конкретная реализация, скрыта от внешних пакетов.
type authService struct {
	userRepo       domain.UserRepository
	credentialRepo domain.CredentialRepository
	tokenRepo      domain.TokenRepository
	tenantRepo     domain.TenantRepository
	codec          AuthTokenCodec
	events         AuthEventPublisher
	// hasher — порт хеширования паролей: открытый пароль в домен не попадает.
	hasher domain.PasswordHasher
	// uow — транзакционная граница регистрации и смены пароля: пользователь,
	// учётные данные, членство и сессия создаются целиком или не создаются.
	uow domain.UnitOfWork
	// ttl — срок жизни выдаваемых токенов.
	ttl time.Duration
	// passwordTTL — срок жизни пароля: по его истечении вход превращается в
	// принудительную смену пароля. Значение <= 0 отключает проверку.
	passwordTTL time.Duration
	// now — часы сервиса: поле, а не прямой вызов time.Now, чтобы тесты
	// проверяли истечение срока и не зависели от реального времени.
	now func() time.Time
}

// NewAuthService создаёт сервис авторизации.
//
// ttl — срок жизни выдаваемых токенов (config.Auth().TokenTTL): сервис не
// читает конфигурацию сам, её передаёт слой сборки приложения.
// passwordTTL — срок жизни пароля (config.Auth().PasswordTTL): он записывается
// в expires_at учётных данных, поэтому после его истечения вход превращается в
// принудительную смену пароля.
//
// hasher и uow — порты домена: реализация хеширования (pkg/password) и
// транзакционная граница (адаптер над менеджером транзакций) подставляются в
// DI, поэтому сервис не зависит от инфраструктуры.
func NewAuthService(
	ctx context.Context,
	userRepo domain.UserRepository,
	credentialRepo domain.CredentialRepository,
	tokenRepo domain.TokenRepository,
	tenantRepo domain.TenantRepository,
	codec AuthTokenCodec,
	events AuthEventPublisher,
	hasher domain.PasswordHasher,
	uow domain.UnitOfWork,
	ttl time.Duration,
	passwordTTL time.Duration,
) AuthService {
	slog.DebugContext(ctx, "auth service created", "token_ttl", ttl.String(), "password_ttl_enabled", passwordTTL > 0)

	return &authService{
		userRepo:       userRepo,
		credentialRepo: credentialRepo,
		tokenRepo:      tokenRepo,
		tenantRepo:     tenantRepo,
		codec:          codec,
		events:         events,
		hasher:         hasher,
		uow:            uow,
		ttl:            ttl,
		passwordTTL:    passwordTTL,
		now:            time.Now,
	}
}

// Register создаёт пользователя с паролем и выдаёт ему токен.
//
// Весь сценарий выполняется в одной транзакции (domain.UnitOfWork): профиль,
// учётные данные, членство в тенанте и сессия создаются целиком или не
// создаются вовсе. Раньше шаги компенсировались вручную, и сбой на последнем
// оставлял занятый email, пароль без сессии и «висячий» личный тенант.
//
// Самостоятельная регистрация даёт личный тенант: сессия действует в рамках
// одного тенанта (user_tokens.tenant_id), а роль — атрибут членства
// (tenant_members.role_id), поэтому без тенанта токен выдать нечем. В своём
// тенанте пользователь — владелец с ролью client: сотрудником он становится,
// только если его роль в тенанте повысит администратор (или bootstrap).
func (s *authService) Register(ctx context.Context, in model.RegisterInput) (*model.User, *Session, error) {
	now := s.now()

	// Вход проверяется целиком до первой записи: из-за невалидного пароля
	// пользователь создаваться не должен — иначе транзакцию пришлось бы
	// откатывать из-за ошибки валидации.
	if err := in.Validate(); err != nil {
		return nil, nil, fmt.Errorf("register user: %w", err)
	}

	user, err := model.NewUser(in.CreateUserInput, now)
	if err != nil {
		return nil, nil, fmt.Errorf("register user: %w", err)
	}

	// Хеш считается до транзакции: Argon2id занимает ~20 мс и не должен
	// удерживать соединение с БД и блокировки строк.
	hash, err := s.hasher.Hash(in.Password)
	if err != nil {
		return nil, nil, fmt.Errorf("register user: %w", err)
	}

	credentials, err := model.NewCredentials(user.ID, user.Email, hash, s.passwordTTL, now)
	if err != nil {
		return nil, nil, fmt.Errorf("register user: %w", err)
	}

	var (
		tenant  *model.Tenant
		session *Session
	)

	err = s.uow.Do(ctx, func(txCtx context.Context) error {
		if createErr := s.userRepo.Create(txCtx, user); createErr != nil {
			return createErr
		}

		if createErr := s.credentialRepo.Create(txCtx, credentials); createErr != nil {
			return createErr
		}

		resolvedTenant, member, tenantErr := s.registrationTenant(txCtx, user, in.TenantSlug, now)
		if tenantErr != nil {
			return tenantErr
		}

		tenant = resolvedTenant

		session, err = s.issue(txCtx, user, tenant, member, now)

		return err
	})
	if err != nil {
		return nil, nil, fmt.Errorf("register user: %w", err)
	}

	slog.InfoContext(ctx, "user registered", "uuid", user.ID, "tenant", tenant.ID)
	s.publish(ctx, EventUserRegistered)

	return user, session, nil
}

// Login проверяет email и пароль и выдаёт токен.
//
// И «пользователя нет», и «пароль не тот» дают одну ошибку:
// ErrInvalidCredentials. Так по ответу нельзя перебрать адреса, а время ответа
// в обоих случаях одинаковое — проверка пароля выполняется всегда.
func (s *authService) Login(ctx context.Context, email, password, tenantSlug string) (*Session, error) {
	now := s.now()

	user, err := s.userRepo.GetByEmail(ctx, model.NormalizeEmail(email))
	if errors.Is(err, domain.ErrNotFound) {
		_ = s.hasher.Verify(password, _dummyPasswordHash)

		return nil, ErrInvalidCredentials
	}

	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}

	credentials, err := s.credentialRepo.GetByUserID(ctx, user.ID)
	if errors.Is(err, domain.ErrNotFound) {
		// Пользователь есть, пароля нет: например, профиль завёл сотрудник.
		// Для клиента это тот же отказ, что и неверный пароль.
		_ = s.hasher.Verify(password, _dummyPasswordHash)

		return nil, ErrInvalidCredentials
	}

	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}

	if err := s.hasher.Verify(password, credentials.Hash); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidCredentials, err)
	}

	// Просроченный пароль блокирует вход: токен не выдаётся, пока пароль не
	// сменён. Проверка стоит после проверки пароля, поэтому по ответу нельзя
	// узнать о пользователе больше, чем он сам сообщил.
	if credentials.Expired(now) {
		expiredAt := credentials.ExpiresAt.Format(time.RFC3339)
		slog.InfoContext(ctx, "user password expired", "uuid", user.ID, "expired_at", expiredAt)

		return nil, fmt.Errorf("%w: password of user %s expired at %s", ErrPasswordExpired, user.ID, expiredAt)
	}

	// Устаревший хеш обновляем на лету: пользователь ввёл правильный пароль,
	// и переход на новый алгоритм не должен требовать от него действий.
	s.upgradeHash(ctx, credentials, password)

	tenant, member, err := s.sessionTenant(ctx, user, tenantSlug, now)
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}

	s.touchLastLogin(ctx, user, now)
	s.touchCredentialLogin(ctx, credentials, now)
	s.removeExpiredTokens(ctx, user.ID, now)

	session, err := s.issue(ctx, user, tenant, member, now)
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}

	slog.InfoContext(ctx, "user logged in", "uuid", user.ID, "tenant", tenant.ID)
	s.publish(ctx, EventUserLoggedIn)

	return session, nil
}

// ChangePassword меняет пароль пользователя и выдаёт новую сессию.
//
// Личность подтверждается текущим паролем: у пользователя с просроченным
// паролем токена нет, поэтому предъявить он может только сам пароль. После
// успешной смены срок действия пароля начинается заново, все ранее выданные
// сессии отзываются (иначе украденный токен продолжал бы работать до 30 дней),
// а пользователь сразу получает новую сессию.
//
// Обновление учётных данных, отзыв сессий и выдача новой выполняются в одной
// транзакции: частичный результат оставил бы пользователя либо без пароля, либо
// со старыми сессиями.
func (s *authService) ChangePassword(
	ctx context.Context,
	email, currentPassword, newPassword, tenantSlug string,
) (*Session, error) {
	now := s.now()

	user, err := s.userRepo.GetByEmail(ctx, model.NormalizeEmail(email))
	if errors.Is(err, domain.ErrNotFound) {
		_ = s.hasher.Verify(currentPassword, _dummyPasswordHash)

		return nil, ErrInvalidCredentials
	}

	if err != nil {
		return nil, fmt.Errorf("change password: %w", err)
	}

	credentials, err := s.credentialRepo.GetByUserID(ctx, user.ID)
	if errors.Is(err, domain.ErrNotFound) {
		// Пользователь есть, пароля нет: подтвердить личность нечем.
		_ = s.hasher.Verify(currentPassword, _dummyPasswordHash)

		return nil, ErrInvalidCredentials
	}

	if err != nil {
		return nil, fmt.Errorf("change password: %w", err)
	}

	if err := s.hasher.Verify(currentPassword, credentials.Hash); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidCredentials, err)
	}

	// Новый пароль проходит те же требования, что и при регистрации; проверка
	// идёт до записи, чтобы неверный пароль не оставил пользователя без входа.
	// Смена пароля перезаписывает срок его действия (expires_at).
	if err := model.ValidatePassword(newPassword); err != nil {
		return nil, fmt.Errorf("change password: %w", err)
	}

	hash, err := s.hasher.Hash(newPassword)
	if err != nil {
		return nil, fmt.Errorf("change password: %w", err)
	}

	if err := credentials.SetPasswordHash(hash, s.passwordTTL, now); err != nil {
		return nil, fmt.Errorf("change password: %w", err)
	}

	var session *Session

	err = s.uow.Do(ctx, func(txCtx context.Context) error {
		if updateErr := s.credentialRepo.Update(txCtx, credentials); updateErr != nil {
			return updateErr
		}

		// Отзыв всех сессий — часть смены пароля, а не отдельное действие:
		// иначе атакующий с украденным токеном сохранил бы доступ.
		if _, revokeErr := s.tokenRepo.RevokeByUser(txCtx, user.ID, now); revokeErr != nil {
			return fmt.Errorf("revoke sessions: %w", revokeErr)
		}

		// Вход после смены: срок действия пароля сброшен, а новая сессия
		// выдаётся сразу — иначе пользователю пришлось бы входить повторно.
		tenant, member, tenantErr := s.sessionTenant(txCtx, user, tenantSlug, now)
		if tenantErr != nil {
			return tenantErr
		}

		s.touchLastLogin(txCtx, user, now)
		s.removeExpiredTokens(txCtx, user.ID, now)

		issued, issueErr := s.issue(txCtx, user, tenant, member, now)
		session = issued

		return issueErr
	})
	if err != nil {
		return nil, fmt.Errorf("change password: %w", err)
	}

	slog.InfoContext(ctx, "user password changed", "uuid", user.ID)
	s.publish(ctx, EventUserPasswordChanged)

	return session, nil
}

// Logout отзывает сессию, по которой пришёл запрос.
//
// Токен разбирается заново: отзывается запись в базе, поэтому нужен её
// идентификатор (jti). Истёкший или подделанный токен отзывать нечего — в этом
// случае возвращается ErrUnauthorized.
func (s *authService) Logout(ctx context.Context, accessToken string) error {
	token, err := s.codec.Parse(accessToken, s.now())
	if err != nil {
		return fmt.Errorf("logout: %w: %w", ErrUnauthorized, err)
	}

	if err := s.tokenRepo.Revoke(ctx, token.ID, s.now()); err != nil {
		return fmt.Errorf("logout: %w", err)
	}

	slog.InfoContext(ctx, "user logged out")
	s.publish(ctx, EventUserLoggedOut)

	return nil
}

// SwitchTenant выдаёт новую сессию в тенанте tenantSlug.
//
// Проверяется членство в целевом тенанте и его статус: переключиться в чужой
// или приостановленный тенант нельзя. Старый токен по умолчанию не отзывается
// (ADR 0001 §10): клиент может хранить несколько сессий, а полный выход делает
// LogoutAll.
func (s *authService) SwitchTenant(
	ctx context.Context,
	actor *Principal,
	tenantSlug string,
) (*Session, error) {
	if actor == nil || actor.User == nil {
		return nil, fmt.Errorf("switch tenant: %w", ErrUnauthorized)
	}

	now := s.now()

	tenant, member, err := s.membershipBySlug(ctx, tenantSlug, actor.User.ID)
	if err != nil {
		return nil, fmt.Errorf("switch tenant: %w", err)
	}

	session, err := s.issue(ctx, actor.User, tenant, member, now)
	if err != nil {
		return nil, fmt.Errorf("switch tenant: %w", err)
	}

	slog.InfoContext(ctx, "tenant switched", "uuid", actor.User.ID, "tenant", tenant.ID, "slug", tenant.Slug)

	return session, nil
}

// ListMyTenants возвращает действующие членства пользователя.
//
// Список нужен клиенту, чтобы выбрать tenant_slug для входа или переключения:
// без него пользователь с несколькими тенантами не знает, куда идти.
func (s *authService) ListMyTenants(
	ctx context.Context,
	actor *Principal,
) ([]*model.TenantMembership, error) {
	if actor == nil || actor.User == nil {
		return nil, fmt.Errorf("list my tenants: %w", ErrUnauthorized)
	}

	memberships, err := s.tenantRepo.ListMemberships(ctx, actor.User.ID)
	if err != nil {
		return nil, fmt.Errorf("list my tenants: %w", err)
	}

	return memberships, nil
}

// LogoutAll отзывает все сессии пользователя во всех тенантах.
//
// Нужен после компрометации токена и при уходе пользователя: выход из одной
// сессии не мешает остальным устройствам работать дальше.
func (s *authService) LogoutAll(ctx context.Context, actor *Principal) error {
	if actor == nil || actor.User == nil {
		return fmt.Errorf("logout all: %w", ErrUnauthorized)
	}

	revoked, err := s.tokenRepo.RevokeByUser(ctx, actor.User.ID, s.now())
	if err != nil {
		return fmt.Errorf("logout all: %w", err)
	}

	slog.InfoContext(ctx, "user logged out everywhere", "uuid", actor.User.ID, "sessions", revoked)
	s.publish(ctx, EventUserLoggedOut)

	return nil
}

// Authenticate проверяет токен и возвращает действующего пользователя.
//
// Проверок три. Первая — подпись и срок действия: их проверяет кодек, не
// обращаясь к базе. Вторая — состояние сессии: не отозвана ли она. Вторая нужна
// именно для выхода из системы, потому что подпись отозвать нельзя. Третья —
// роль: она берётся из членства в тенанте сессии (tenant_members.role_id), а не
// из подписи, поэтому изменение роли действует без ожидания истечения уже
// выданных токенов.
//
// Если членство удалено, сессия отклоняется: снимок роли из токена не даёт
// доступ после исключения из тенанта (ADR 0001 §13).
func (s *authService) Authenticate(ctx context.Context, accessToken string) (*Principal, error) {
	now := s.now()

	if accessToken == "" {
		return nil, fmt.Errorf("%w: token is not provided", ErrUnauthorized)
	}

	parsed, err := s.codec.Parse(accessToken, now)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnauthorized, err)
	}

	session, err := s.tokenRepo.GetByID(ctx, parsed.ID)
	if errors.Is(err, domain.ErrNotFound) {
		// Подпись верна, а записи нет: сессию удалили (например, чистка
		// просроченных) или токен выдан другим секретом.
		return nil, fmt.Errorf("%w: session %s is not found", ErrUnauthorized, parsed.ID)
	}

	if err != nil {
		return nil, fmt.Errorf("authenticate: %w", err)
	}

	if parsed.UserID != session.UserID {
		// Токен подписан корректно, но ссылается на другую сессию: доверять
		// пользователю из него нельзя.
		return nil, fmt.Errorf("%w: token %s does not match its session", ErrUnauthorized, parsed.ID)
	}

	if err := session.Valid(now); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnauthorized, err)
	}

	// Статус тенанта проверяется на каждом запросе: приостановка и удаление
	// должны закрывать доступ сразу, не дожидаясь истечения 30-дневного токена.
	tenant, err := s.tenantRepo.GetByID(ctx, session.TenantID)
	if err != nil || !tenant.Active() {
		s.revokeSessionOnDenied(ctx, session, "tenant is not active")

		if errors.Is(err, domain.ErrNotFound) {
			return nil, fmt.Errorf("%w: tenant %s is not found", ErrUnauthorized, session.TenantID)
		}

		if err != nil {
			return nil, fmt.Errorf("authenticate: %w", err)
		}

		return nil, fmt.Errorf("%w: tenant %s is not active", ErrUnauthorized, session.TenantID)
	}

	user, err := s.userRepo.GetByID(ctx, session.UserID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("%w: user %s is not found", ErrUnauthorized, session.UserID)
	}

	if err != nil {
		return nil, fmt.Errorf("authenticate: %w", err)
	}

	role, err := s.sessionRole(ctx, session)
	if err != nil {
		return nil, err
	}

	s.touchToken(ctx, session, now)

	return &Principal{User: user, Token: session, Role: role}, nil
}

// revokeSessionOnDenied отзывает сессию, доступ по которой больше не разрешён.
// Ошибку логируем: отзыв — страховка, доступ уже закрыт проверкой.
func (s *authService) revokeSessionOnDenied(ctx context.Context, session *model.Token, reason string) {
	slog.WarnContext(ctx, "session revoked: access denied",
		"reason", reason, "jti", session.ID, "tenant", session.TenantID, "uuid", session.UserID)

	if err := s.tokenRepo.Revoke(ctx, session.ID, s.now()); err != nil {
		slog.WarnContext(ctx, "failed to revoke denied session", "jti", session.ID, "err", err)
	}
}

// sessionRole определяет действующую роль сессии.
//
// Источник роли — членство в тенанте сессии: роль в профиле не хранится, а
// снимок в токене нужен для аудита. Если членства уже нет, сессия отклоняется:
// доступ не должен сохраняться по снимку роли, когда пользователя исключили из
// тенанта (ADR 0001 §13).
func (s *authService) sessionRole(
	ctx context.Context,
	session *model.Token,
) (model.Role, error) {
	member, err := s.tenantRepo.GetMember(ctx, session.TenantID, session.UserID)

	switch {
	case err == nil:
		return member.Role, nil
	case errors.Is(err, domain.ErrNotFound):
		s.revokeSessionOnDenied(ctx, session, "tenant membership is not found")

		return "", fmt.Errorf("%w: membership in tenant %s is not found", ErrUnauthorized, session.TenantID)
	default:
		return "", fmt.Errorf("authenticate: %w", err)
	}
}

// issue выдаёт сессию: сначала запись в базе, затем подпись.
//
// Порядок именно такой: неподписанная запись безвредна (без валидного JWT её
// никто не найдёт), а подписанный токен без записи работать не должен — по нему
// нечего проверять при аутентификации и нечего отзывать при выходе.
//
// Сессия действует в рамках тенанта членства, а роль фиксируется в токене
// снимком на момент выдачи (для аудита): если администратор изменит роль
// позже, действующая роль всё равно будет взята из tenant_members.
func (s *authService) issue(
	ctx context.Context,
	user *model.User,
	tenant *model.Tenant,
	member *model.TenantMember,
	now time.Time,
) (*Session, error) {
	token, err := model.NewToken(user, member.TenantID, member.Role, s.ttl, now)
	if err != nil {
		return nil, err
	}

	if err := s.tokenRepo.Create(ctx, token); err != nil {
		return nil, err
	}

	accessToken, err := s.codec.Sign(token)
	if err != nil {
		return nil, err
	}

	return &Session{AccessToken: accessToken, Token: token, Tenant: tenant, Role: member.Role}, nil
}

// sessionTenant возвращает тенант и членство для новой сессии.
//
// tenantSlug пустой — обратная совместимость: берётся самое раннее действующее
// членство, а если его нет, создаётся личный тенант. Непустой slug — явный
// выбор тенанта: он должен быть действующим, а пользователь — его участником.
func (s *authService) sessionTenant(
	ctx context.Context,
	user *model.User,
	tenantSlug string,
	now time.Time,
) (*model.Tenant, *model.TenantMember, error) {
	if model.NormalizeSlug(tenantSlug) != "" {
		return s.membershipBySlug(ctx, tenantSlug, user.ID)
	}

	member, err := s.tenantRepo.MemberOfUser(ctx, user.ID)
	switch {
	case err == nil:
		tenant, tenantErr := s.tenantRepo.GetByID(ctx, member.TenantID)
		if tenantErr != nil {
			return nil, nil, fmt.Errorf("resolve tenant membership: %w", tenantErr)
		}

		return tenant, member, nil
	case errors.Is(err, domain.ErrNotFound):
		return s.createPersonalTenant(ctx, user, now)
	default:
		return nil, nil, fmt.Errorf("resolve tenant membership: %w", err)
	}
}

// membershipBySlug возвращает членство в тенанте, выбранном по slug.
//
// Ошибки не уточняют, чего нет — тенанта или членства: slug публичен, но состав
// чужих тенантов наружу не отдаётся. Приостановленный тенант доступа не даёт.
func (s *authService) membershipBySlug(
	ctx context.Context,
	slug, userID string,
) (*model.Tenant, *model.TenantMember, error) {
	membership, err := s.tenantRepo.GetMemberBySlug(ctx, slug, userID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil, fmt.Errorf("%w: tenant %q is not available for the user", ErrForbidden, model.NormalizeSlug(slug))
	}

	if err != nil {
		return nil, nil, fmt.Errorf("resolve tenant membership: %w", err)
	}

	return &membership.Tenant, &membership.Member, nil
}

// registrationTenant создаёт членство нового пользователя в тенанте ссылки или
// личный тенант, если ссылки нет.
//
// Регистрация по ссылке НЕ создаёт личный тенант: пользователь сразу становится
// клиентом тенанта, по ссылке которого пришёл (ADR 0001 §14.3). Роль client —
// минимальная: повысить её может администратор тенанта.
func (s *authService) registrationTenant(
	ctx context.Context,
	user *model.User,
	tenantSlug string,
	now time.Time,
) (*model.Tenant, *model.TenantMember, error) {
	if model.NormalizeSlug(tenantSlug) == "" {
		return s.createPersonalTenant(ctx, user, now)
	}

	tenant, err := s.tenantRepo.GetBySlug(ctx, tenantSlug)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil, fmt.Errorf("%w: tenant %q is not found", ErrForbidden, model.NormalizeSlug(tenantSlug))
	}

	if err != nil {
		return nil, nil, fmt.Errorf("register in tenant: %w", err)
	}

	if !tenant.Active() {
		return nil, nil, fmt.Errorf("%w: tenant %q is not active", ErrForbidden, tenant.Slug)
	}

	member, err := model.NewTenantMember(tenant.ID, user.ID, model.RoleClient, false, now)
	if err != nil {
		return nil, nil, fmt.Errorf("register in tenant: %w", err)
	}

	if err := s.tenantRepo.AddMember(ctx, member); err != nil {
		return nil, nil, fmt.Errorf("register in tenant: %w", err)
	}

	slog.InfoContext(ctx, "user joined tenant by link", "uuid", user.ID, "tenant", tenant.ID)

	return tenant, member, nil
}

// createPersonalTenant создаёт личный тенант пользователя и членство в нём.
//
// Владелец тенанта — сам пользователь с ролью client: сотрудником (оператором,
// менеджером или администратором) он становится только по решению
// администратора своего тенанта, поэтому самостоятельная регистрация не даёт
// прав на чужие профили. Slug строится из идентификатора пользователя: он
// стабилен, уникален и не зависит от имени, которое пользователь может сменить.
//
// Тенант и членство создаются одной транзакцией: иначе при сбое остался бы
// тенант без владельца.
func (s *authService) createPersonalTenant(
	ctx context.Context,
	user *model.User,
	now time.Time,
) (*model.Tenant, *model.TenantMember, error) {
	slug := personalTenantSlug(user.ID)

	tenant, err := model.NewTenant(user.FullName(), slug, now)
	if err != nil {
		return nil, nil, fmt.Errorf("create personal tenant: %w", err)
	}

	member, err := model.NewTenantMember(tenant.ID, user.ID, model.RoleClient, true, now)
	if err != nil {
		return nil, nil, fmt.Errorf("create personal tenant: %w", err)
	}

	if err := s.tenantRepo.CreateWithOwner(ctx, tenant, member); err != nil {
		return nil, nil, fmt.Errorf("create personal tenant: %w", err)
	}

	slog.InfoContext(ctx, "personal tenant created", "tenant", tenant.ID, "uuid", user.ID)

	return tenant, member, nil
}

// personalTenantSlug строит slug личного тенанта из идентификатора владельца:
// «tenant-<uuid>». Идентификатор уникален и не меняется, поэтому slug не
// конфликтует с чужими тенантами и переживает смену имени пользователя.
func personalTenantSlug(userID string) string {
	return "tenant-" + userID
}

// upgradeHash пересчитывает устаревший хеш пароля и сохраняет его.
//
// Это не смена пароля: время последнего изменения (UpdatedAt) не трогается,
// поэтому срок действия пароля не сбрасывается. Ошибку логируем — она не
// должна мешать входу, ведь пароль уже проверен.
func (s *authService) upgradeHash(ctx context.Context, credentials *model.Credentials, password string) {
	if !s.hasher.NeedsRehash(credentials.Hash) {
		return
	}

	hash, err := s.hasher.Hash(password)
	if err != nil {
		slog.WarnContext(ctx, "failed to rehash password", "uuid", credentials.UserID, "err", err)

		return
	}

	if err := credentials.SetHash(hash); err != nil {
		slog.WarnContext(ctx, "failed to rehash password", "uuid", credentials.UserID, "err", err)

		return
	}

	if err := s.credentialRepo.Update(ctx, credentials); err != nil {
		slog.WarnContext(ctx, "failed to persist rehashed password", "uuid", credentials.UserID, "err", err)
	}
}

// touchCredentialLogin отмечает вход по учётным данным: в user_credentials
// хранится время последнего входа именно этим провайдером (ERDB 2.4). Ошибка не
// отменяет вход: это аудит, а не условие доступа.
func (s *authService) touchCredentialLogin(ctx context.Context, credentials *model.Credentials, now time.Time) {
	if err := s.credentialRepo.TouchLastLogin(ctx, credentials.ID, now); err != nil {
		slog.WarnContext(ctx, "failed to update credentials last login", "uuid", credentials.UserID, "err", err)

		return
	}

	credentials.TouchLastLogin(now)
}

// touchLastLogin фиксирует время входа в профиле пользователя.
// Ошибка не отменяет вход: это аудит, а не условие доступа.
func (s *authService) touchLastLogin(ctx context.Context, user *model.User, now time.Time) {
	user.SetLastLogin(now)

	if err := s.userRepo.Update(ctx, user); err != nil {
		slog.WarnContext(ctx, "failed to update last login", "uuid", user.ID, "err", err)
	}
}

// touchToken обновляет время последнего использования токена, но не чаще
// tokenTouchInterval: писать в базу на каждый запрос незачем.
func (s *authService) touchToken(ctx context.Context, token *model.Token, now time.Time) {
	if token.LastUsedAt != nil && now.Sub(*token.LastUsedAt) < tokenTouchInterval {
		return
	}

	if err := s.tokenRepo.TouchLastUsed(ctx, token.ID, now); err != nil {
		slog.WarnContext(ctx, "failed to update token activity", "jti", token.ID, "err", err)

		return
	}

	token.Touch(now)
}

// removeExpiredTokens удаляет просроченные сессии: без планировщика таблица
// росла бы с каждым входом. Ошибка не влияет на вход — мусор уберётся при
// следующей попытке.
func (s *authService) removeExpiredTokens(ctx context.Context, id string, now time.Time) {
	deleted, err := s.tokenRepo.DeleteExpired(ctx, id, now)
	if err != nil {
		slog.WarnContext(ctx, "failed to remove expired tokens", "err", err)

		return
	}

	if deleted > 0 {
		slog.DebugContext(ctx, "expired tokens removed", "count", deleted)
	}
}

// publish публикует событие, если шина подключена: сервис не должен падать
// из-за отсутствия подписчиков (в тестах или на раннем этапе сборки).
func (s *authService) publish(ctx context.Context, event string) {
	if s.events == nil {
		return
	}

	s.events.Publish(ctx, event)
}
