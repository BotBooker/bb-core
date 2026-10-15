package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/botbooker/bb-core/internal/database"
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

// AuthUserRepository — интерфейс репозитория пользователей для AuthService.
type AuthUserRepository interface {
	Create(ctx context.Context, user *model.User) error
	GetByID(ctx context.Context, id string) (*model.User, error)
	GetByEmail(ctx context.Context, email string) (*model.User, error)
	Update(ctx context.Context, user *model.User) error
	Delete(ctx context.Context, id string) error
}

// AuthCredentialRepository — интерфейс учётных данных (user_credentials).
type AuthCredentialRepository interface {
	Create(ctx context.Context, credentials *model.Credentials) error
	GetByUserID(ctx context.Context, userID string) (*model.Credentials, error)
	// Update перезаписывает изменяемые поля учётных данных (хеш пароля и срок
	// его действия — смена пароля и «прозрачное» обновление устаревшего хеша).
	Update(ctx context.Context, credentials *model.Credentials) error
	// TouchLastLogin отмечает вход по учётным данным (аудит).
	TouchLastLogin(ctx context.Context, id string, at time.Time) error
}

// AuthTenantRepository — интерфейс репозитория тенантов и членства.
//
// Нужен сервису авторизации, потому что роль — атрибут членства в тенанте
// (tenant_members.role_id), а сессия действует в рамках одного тенанта:
// без членства выдать токен с ролью нельзя.
type AuthTenantRepository interface {
	// Create создаёт тенант.
	Create(ctx context.Context, tenant *model.Tenant) error
	// AddMember добавляет пользователя в тенант.
	AddMember(ctx context.Context, member *model.TenantMember) error
	// GetMember возвращает членство в тенанте или database.ErrNotFound.
	GetMember(ctx context.Context, tenantID, userID string) (*model.TenantMember, error)
	// MemberOfUser возвращает членство в действующем тенанте или
	// database.ErrNotFound — это тенант по умолчанию для новой сессии.
	MemberOfUser(ctx context.Context, userID string) (*model.TenantMember, error)
}

// AuthTokenRepository — интерфейс репозитория сессий.
type AuthTokenRepository interface {
	Create(ctx context.Context, token *model.Token) error
	GetByID(ctx context.Context, id string) (*model.Token, error)
	TouchLastUsed(ctx context.Context, id string, at time.Time) error
	Revoke(ctx context.Context, id string, at time.Time) error
	DeleteExpired(ctx context.Context, id string, before time.Time) (int64, error)
}

// AuthTokenCodec подписывает и проверяет токены. Реализация — internal/jwt:
// сервису не важно, как устроен формат, ему нужны подпись и разбор.
type AuthTokenCodec interface {
	Sign(token *model.Token) (string, error)
	Parse(raw string, now time.Time) (*model.Token, error)
}

// AuthEventPublisher — публикация событий авторизации.
type AuthEventPublisher interface {
	Publish(event string)
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
	Login(ctx context.Context, email, password string) (*Session, error)
	// ChangePassword меняет пароль пользователя и выдаёт новую сессию.
	//
	// Личность подтверждается текущим паролем, а не токеном: сменить пароль
	// нужно и тем, у кого срок действия пароля истёк, — а им токен не выдаётся.
	ChangePassword(ctx context.Context, email, currentPassword, newPassword string) (*Session, error)
	// Logout отзывает токен — это и есть выход из системы.
	Logout(ctx context.Context, accessToken string) error
	// Authenticate проверяет токен и возвращает действующего пользователя.
	Authenticate(ctx context.Context, accessToken string) (*Principal, error)
}

// authService — конкретная реализация, скрыта от внешних пакетов.
type authService struct {
	userRepo       AuthUserRepository
	credentialRepo AuthCredentialRepository
	tokenRepo      AuthTokenRepository
	tenantRepo     AuthTenantRepository
	codec          AuthTokenCodec
	events         AuthEventPublisher
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
func NewAuthService(
	userRepo AuthUserRepository,
	credentialRepo AuthCredentialRepository,
	tokenRepo AuthTokenRepository,
	tenantRepo AuthTenantRepository,
	codec AuthTokenCodec,
	events AuthEventPublisher,
	ttl time.Duration,
	passwordTTL time.Duration,
) AuthService {
	slog.Debug("auth service created", "token_ttl", ttl.String(), "password_ttl_enabled", passwordTTL > 0)

	return &authService{
		userRepo:       userRepo,
		credentialRepo: credentialRepo,
		tokenRepo:      tokenRepo,
		tenantRepo:     tenantRepo,
		codec:          codec,
		events:         events,
		ttl:            ttl,
		passwordTTL:    passwordTTL,
		now:            time.Now,
	}
}

// Register создаёт пользователя с паролем и выдаёт ему токен.
//
// Порядок операций: пользователь → пароль → членство в тенанте → токен.
// Между операциями нет общей транзакции (репозитории работают каждый со своим
// подключением), поэтому неудачное сохранение пароля откатывается вручную:
// иначе остался бы пользователь, занявший email, войти в которого невозможно.
//
// Самостоятельная регистрация даёт личный тенант: сессия действует в рамках
// одного тенанта (user_tokens.tenant_id), а роль — атрибут членства
// (tenant_members.role_id), поэтому без тенанта токен выдать нечем. В своём
// тенанте пользователь — владелец с ролью client: сотрудником он становится,
// только если его роль в тенанте повысит администратор (или bootstrap).
func (s *authService) Register(ctx context.Context, in model.RegisterInput) (*model.User, *Session, error) {
	now := s.now()

	// Вход проверяется целиком до первой записи: из-за невалидного пароля
	// пользователь создаваться не должен — иначе его пришлось бы удалять.
	if err := in.Validate(); err != nil {
		return nil, nil, fmt.Errorf("register user: %w", err)
	}

	user, err := model.NewUser(in.CreateUserInput, now)
	if err != nil {
		return nil, nil, fmt.Errorf("register user: %w", err)
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, nil, fmt.Errorf("register user: %w", err)
	}

	credentials, err := model.NewCredentials(user.ID, user.Email, in.Password, s.passwordTTL, now)
	if err == nil {
		err = s.credentialRepo.Create(ctx, credentials)
	}

	if err != nil {
		s.removeUser(ctx, user.ID)

		return nil, nil, fmt.Errorf("register user: %w", err)
	}

	member, err := s.createPersonalTenant(ctx, user, now)
	if err != nil {
		s.removeUser(ctx, user.ID)

		return nil, nil, fmt.Errorf("register user: %w", err)
	}

	session, err := s.issue(ctx, user, member, now)
	if err != nil {
		return nil, nil, fmt.Errorf("register user: %w", err)
	}

	slog.InfoContext(ctx, "user registered", "uuid", user.ID, "tenant", member.TenantID)
	s.publish(EventUserRegistered)

	return user, session, nil
}

// Login проверяет email и пароль и выдаёт токен.
//
// И «пользователя нет», и «пароль не тот» дают одну ошибку:
// ErrInvalidCredentials. Так по ответу нельзя перебрать адреса, а время ответа
// в обоих случаях одинаковое — проверка пароля выполняется всегда.
func (s *authService) Login(ctx context.Context, email, password string) (*Session, error) {
	now := s.now()

	user, err := s.userRepo.GetByEmail(ctx, model.NormalizeEmail(email))
	if errors.Is(err, database.ErrNotFound) {
		_ = model.VerifyPassword(password, _dummyPasswordHash)

		return nil, ErrInvalidCredentials
	}

	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}

	credentials, err := s.credentialRepo.GetByUserID(ctx, user.ID)
	if errors.Is(err, database.ErrNotFound) {
		// Пользователь есть, пароля нет: например, профиль завёл сотрудник.
		// Для клиента это тот же отказ, что и неверный пароль.
		_ = model.VerifyPassword(password, _dummyPasswordHash)

		return nil, ErrInvalidCredentials
	}

	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}

	if err := credentials.Verify(password); err != nil {
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

	member, err := s.membership(ctx, user, now)
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}

	s.touchLastLogin(ctx, user, now)
	s.touchCredentialLogin(ctx, credentials, now)
	s.removeExpiredTokens(ctx, user.ID, now)

	session, err := s.issue(ctx, user, member, now)
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}

	slog.InfoContext(ctx, "user logged in", "uuid", user.ID, "tenant", member.TenantID)
	s.publish(EventUserLoggedIn)

	return session, nil
}

// ChangePassword меняет пароль пользователя и выдаёт новую сессию.
//
// Личность подтверждается текущим паролем: у пользователя с просроченным
// паролем токена нет, поэтому предъявить он может только сам пароль. После
// успешной смены срок действия пароля начинается заново (UpdatedAt), а
// пользователь сразу получает токен и может работать.
func (s *authService) ChangePassword(
	ctx context.Context,
	email, currentPassword, newPassword string,
) (*Session, error) {
	now := s.now()

	user, err := s.userRepo.GetByEmail(ctx, model.NormalizeEmail(email))
	if errors.Is(err, database.ErrNotFound) {
		_ = model.VerifyPassword(currentPassword, _dummyPasswordHash)

		return nil, ErrInvalidCredentials
	}

	if err != nil {
		return nil, fmt.Errorf("change password: %w", err)
	}

	credentials, err := s.credentialRepo.GetByUserID(ctx, user.ID)
	if errors.Is(err, database.ErrNotFound) {
		// Пользователь есть, пароля нет: подтвердить личность нечем.
		_ = model.VerifyPassword(currentPassword, _dummyPasswordHash)

		return nil, ErrInvalidCredentials
	}

	if err != nil {
		return nil, fmt.Errorf("change password: %w", err)
	}

	if err := credentials.Verify(currentPassword); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidCredentials, err)
	}

	// Новый пароль проходит те же требования, что и при регистрации; проверка
	// идёт до записи, чтобы неверный пароль не оставил пользователя без входа.
	// Смена пароля перезаписывает срок его действия (expires_at).
	if err := credentials.SetPassword(newPassword, s.passwordTTL, now); err != nil {
		return nil, fmt.Errorf("change password: %w", err)
	}

	if err := s.credentialRepo.Update(ctx, credentials); err != nil {
		return nil, fmt.Errorf("change password: %w", err)
	}

	// Вход после смены: срок действия пароля сброшен, а новая сессия выдаётся
	// сразу — иначе пользователю пришлось бы входить повторно.
	member, err := s.membership(ctx, user, now)
	if err != nil {
		return nil, fmt.Errorf("change password: %w", err)
	}

	s.touchLastLogin(ctx, user, now)
	s.removeExpiredTokens(ctx, user.ID, now)

	session, err := s.issue(ctx, user, member, now)
	if err != nil {
		return nil, fmt.Errorf("change password: %w", err)
	}

	slog.InfoContext(ctx, "user password changed", "uuid", user.ID)
	s.publish(EventUserPasswordChanged)

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
	s.publish(EventUserLoggedOut)

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
// Если членство удалено, остаётся снимок роли из токена (claim role): сессия не
// «переключается» на другую роль молча, а администратор может отозвать её
// отдельно (см. user_tokens.revoked_at).
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
	if errors.Is(err, database.ErrNotFound) {
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

	user, err := s.userRepo.GetByID(ctx, session.UserID)
	if errors.Is(err, database.ErrNotFound) {
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

// sessionRole определяет действующую роль сессии.
//
// Источник роли — членство в тенанте сессии: роль в профиле не хранится, а
// снимок в токене нужен для аудита. Если членства уже нет, роль берётся из
// снимка, поэтому доступ не «прыгает» на другую роль.
func (s *authService) sessionRole(ctx context.Context, session *model.Token) (model.Role, error) {
	member, err := s.tenantRepo.GetMember(ctx, session.TenantID, session.UserID)

	switch {
	case err == nil:
		return member.Role, nil
	case errors.Is(err, database.ErrNotFound):
		slog.WarnContext(ctx, "tenant membership is not found, the session role snapshot is used",
			"jti", session.ID, "tenant", session.TenantID, "uuid", session.UserID)

		return session.Role, nil
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

	return &Session{AccessToken: accessToken, Token: token}, nil
}

// membership возвращает членство пользователя в действующем тенанте, а если его
// нет — создаёт личный тенант с пользователем-владельцем.
//
// Тенант нужен всегда: сессия действует в одном тенанте (user_tokens.tenant_id
// NOT NULL), а роль — атрибут членства. Профиль, созданный сотрудником
// (POST /users), члена тенанта не имеет, поэтому вход по нему тоже заводит
// личный тенант — иначе такой пользователь не смог бы войти.
func (s *authService) membership(ctx context.Context, user *model.User, now time.Time) (*model.TenantMember, error) {
	member, err := s.tenantRepo.MemberOfUser(ctx, user.ID)
	switch {
	case err == nil:
		return member, nil
	case errors.Is(err, database.ErrNotFound):
		return s.createPersonalTenant(ctx, user, now)
	default:
		return nil, fmt.Errorf("resolve tenant membership: %w", err)
	}
}

// createPersonalTenant создаёт личный тенант пользователя и членство в нём.
//
// Владелец тенанта — сам пользователь с ролью client: сотрудником (оператором,
// менеджером или администратором) он становится только по решению
// администратора своего тенанта, поэтому самостоятельная регистрация не даёт
// прав на чужие профили. Slug строится из идентификатора пользователя: он
// стабилен, уникален и не зависит от имени, которое пользователь может сменить.
func (s *authService) createPersonalTenant(
	ctx context.Context,
	user *model.User,
	now time.Time,
) (*model.TenantMember, error) {
	slug := personalTenantSlug(user.ID)

	tenant, err := model.NewTenant(user.FullName(), slug, now)
	if err != nil {
		return nil, fmt.Errorf("create personal tenant: %w", err)
	}

	if err := s.tenantRepo.Create(ctx, tenant); err != nil {
		return nil, fmt.Errorf("create personal tenant: %w", err)
	}

	member, err := model.NewTenantMember(tenant.ID, user.ID, model.RoleClient, true, now)
	if err != nil {
		return nil, fmt.Errorf("create personal tenant: %w", err)
	}

	if err := s.tenantRepo.AddMember(ctx, member); err != nil {
		return nil, fmt.Errorf("create personal tenant: %w", err)
	}

	slog.InfoContext(ctx, "personal tenant created", "tenant", tenant.ID, "uuid", user.ID)

	return member, nil
}

// personalTenantSlug строит slug личного тенанта из идентификатора владельца:
// «tenant-<uuid>». Идентификатор уникален и не меняется, поэтому slug не
// конфликтует с чужими тенантами и переживает смену имени пользователя.
func personalTenantSlug(userID string) string {
	return "tenant-" + userID
}

// removeUser убирает пользователя после неудачной регистрации.
//
// Ошибку логируем: она означает, что аккаунт остался без пароля, и это нужно
// увидеть, а не потерять.
func (s *authService) removeUser(ctx context.Context, id string) {
	if err := s.userRepo.Delete(ctx, id); err != nil {
		slog.ErrorContext(ctx, "failed to roll back registration", "uuid", id, "err", err)
	}
}

// upgradeHash пересчитывает устаревший хеш пароля и сохраняет его.
//
// Это не смена пароля: время последнего изменения (UpdatedAt) не трогается,
// поэтому срок действия пароля не сбрасывается. Ошибку логируем — она не
// должна мешать входу, ведь пароль уже проверен.
func (s *authService) upgradeHash(ctx context.Context, credentials *model.Credentials, password string) {
	if !credentials.NeedsRehash() {
		return
	}

	if err := credentials.Rehash(password); err != nil {
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
func (s *authService) publish(event string) {
	if s.events == nil {
		return
	}

	s.events.Publish(event)
}
