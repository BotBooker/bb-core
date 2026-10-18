package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/model"
)

const (
	// testTokenTTL — срок жизни токена в тестах: значение не влияет на проверки,
	// они смотрят на выданную запись, а не на конкретное число.
	testTokenTTL = 24 * time.Hour
	// testPasswordTTL — срок жизни пароля в тестах (90 дней).
	testPasswordTTL = 90 * 24 * time.Hour
	// testPassword — пароль тестовых учётных данных.
	testPassword = "correct horse battery staple"
	// testPasswordHash — хеш testPassword в прежнем формате (PBKDF2), посчитанный
	// заранее: сам он стоит ~0,2 с, и платить их в каждом тесте незачем (своё
	// хеширование проверяется в тестах модели). Формат намеренно устаревший:
	// так тесты входа заодно проверяют и совместимость, и пересчёт хеша.
	testPasswordHash = "pbkdf2_sha256$210000$dQIjm0wTireOlDo8WomRug$+qvx5cWbyYARbq061iEy9aedaWfstobFn25SQIDvZbc"
	// testCredentialID — идентификатор учётных данных в тестах.
	testCredentialID = "4f0c8f0e-9f60-4e0e-9d5f-1f6f1f6f1f6f"
	// testTenantID — тенант сессии в тестах: роль и тенант приходят из членства.
	testTenantID = "0f0c8f0e-9f60-4e0e-9d5f-1f6f1f6f1f6f"
	// testEmail — адрес тестового пользователя: он же ключ провайдера email.
	testEmail = "ivan@example.com"
)

// testNow — фиксированное «сейчас» сервиса авторизации: так проверки не зависят
// от реального времени.
var testNow = time.Date(2026, time.October, 12, 10, 0, 0, 0, time.UTC)

// mockAuthUserRepository — мок репозитория пользователей для AuthService.
type mockAuthUserRepository struct {
	createFunc     func(ctx context.Context, user *model.User) error
	getByIDFunc    func(ctx context.Context, id string) (*model.User, error)
	getByEmailFunc func(ctx context.Context, email string) (*model.User, error)
	updateFunc     func(ctx context.Context, user *model.User) error
	deleteFunc     func(ctx context.Context, id string) error
}

func (m *mockAuthUserRepository) Create(ctx context.Context, user *model.User) error {
	if m.createFunc != nil {
		return m.createFunc(ctx, user)
	}

	return nil
}

func (m *mockAuthUserRepository) GetByID(ctx context.Context, id string) (*model.User, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}

	return nil, database.ErrNotFound
}

func (m *mockAuthUserRepository) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	if m.getByEmailFunc != nil {
		return m.getByEmailFunc(ctx, email)
	}

	return nil, database.ErrNotFound
}

func (m *mockAuthUserRepository) Update(ctx context.Context, user *model.User) error {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, user)
	}

	return nil
}

func (m *mockAuthUserRepository) Delete(ctx context.Context, id string) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, id)
	}

	return nil
}

// mockAuthCredentialRepository — мок учётных данных.
type mockAuthCredentialRepository struct {
	createFunc      func(ctx context.Context, credentials *model.Credentials) error
	getByUserIDFunc func(ctx context.Context, userID string) (*model.Credentials, error)
	updateFunc      func(ctx context.Context, credentials *model.Credentials) error
	touchFunc       func(ctx context.Context, id string, at time.Time) error
}

func (m *mockAuthCredentialRepository) Create(ctx context.Context, credentials *model.Credentials) error {
	if m.createFunc != nil {
		return m.createFunc(ctx, credentials)
	}

	return nil
}

func (m *mockAuthCredentialRepository) GetByUserID(ctx context.Context, userID string) (*model.Credentials, error) {
	if m.getByUserIDFunc != nil {
		return m.getByUserIDFunc(ctx, userID)
	}

	return nil, database.ErrNotFound
}

func (m *mockAuthCredentialRepository) Update(ctx context.Context, credentials *model.Credentials) error {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, credentials)
	}

	return nil
}

func (m *mockAuthCredentialRepository) TouchLastLogin(ctx context.Context, id string, at time.Time) error {
	if m.touchFunc != nil {
		return m.touchFunc(ctx, id, at)
	}

	return nil
}

// mockAuthTenantRepository — мок репозитория тенантов и членства.
//
// По умолчанию членства нет (ErrNotFound): так тесты, которым тенант не важен,
// проходят через ветку создания личного тенанта.
type mockAuthTenantRepository struct {
	createFunc          func(ctx context.Context, tenant *model.Tenant) error
	createWithOwnerFunc func(ctx context.Context, tenant *model.Tenant, member *model.TenantMember) error
	getByIDFunc         func(ctx context.Context, id string) (*model.Tenant, error)
	getBySlugFunc       func(ctx context.Context, slug string) (*model.Tenant, error)
	addMemberFunc       func(ctx context.Context, member *model.TenantMember) error
	getMemberFunc       func(ctx context.Context, tenantID, userID string) (*model.TenantMember, error)
	getMemberBySlugFunc func(ctx context.Context, slug, userID string) (*model.TenantMembership, error)
	memberOfUserFunc    func(ctx context.Context, userID string) (*model.TenantMember, error)
	listMembershipsFunc func(ctx context.Context, userID string) ([]*model.TenantMembership, error)
}

func (m *mockAuthTenantRepository) Create(ctx context.Context, tenant *model.Tenant) error {
	if m.createFunc != nil {
		return m.createFunc(ctx, tenant)
	}

	return nil
}

func (m *mockAuthTenantRepository) CreateWithOwner(
	ctx context.Context,
	tenant *model.Tenant,
	member *model.TenantMember,
) error {
	if m.createWithOwnerFunc != nil {
		return m.createWithOwnerFunc(ctx, tenant, member)
	}

	return nil
}

func (m *mockAuthTenantRepository) GetByID(ctx context.Context, id string) (*model.Tenant, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}

	// По умолчанию тенант сессии действующий: тесты аутентификации, которым
	// статус не важен, не должны настраивать ещё и тенант.
	return &model.Tenant{ID: id, Status: model.TenantStatusActive}, nil
}

func (m *mockAuthTenantRepository) GetBySlug(ctx context.Context, slug string) (*model.Tenant, error) {
	if m.getBySlugFunc != nil {
		return m.getBySlugFunc(ctx, slug)
	}

	return nil, database.ErrNotFound
}

func (m *mockAuthTenantRepository) GetMemberBySlug(
	ctx context.Context,
	slug, userID string,
) (*model.TenantMembership, error) {
	if m.getMemberBySlugFunc != nil {
		return m.getMemberBySlugFunc(ctx, slug, userID)
	}

	return nil, database.ErrNotFound
}

func (m *mockAuthTenantRepository) ListMemberships(
	ctx context.Context,
	userID string,
) ([]*model.TenantMembership, error) {
	if m.listMembershipsFunc != nil {
		return m.listMembershipsFunc(ctx, userID)
	}

	return nil, nil
}

func (m *mockAuthTenantRepository) AddMember(ctx context.Context, member *model.TenantMember) error {
	if m.addMemberFunc != nil {
		return m.addMemberFunc(ctx, member)
	}

	return nil
}

func (m *mockAuthTenantRepository) GetMember(ctx context.Context, tenantID, userID string) (*model.TenantMember, error) {
	if m.getMemberFunc != nil {
		return m.getMemberFunc(ctx, tenantID, userID)
	}

	return nil, database.ErrNotFound
}

func (m *mockAuthTenantRepository) MemberOfUser(ctx context.Context, userID string) (*model.TenantMember, error) {
	if m.memberOfUserFunc != nil {
		return m.memberOfUserFunc(ctx, userID)
	}

	return nil, database.ErrNotFound
}

// mockAuthTokenRepository — мок репозитория сессий.
type mockAuthTokenRepository struct {
	createFunc        func(ctx context.Context, token *model.Token) error
	getByIDFunc       func(ctx context.Context, id string) (*model.Token, error)
	touchFunc         func(ctx context.Context, id string, at time.Time) error
	revokeFunc        func(ctx context.Context, id string, at time.Time) error
	revokeByUserFunc  func(ctx context.Context, userID string, at time.Time) (int64, error)
	deleteExpiredFunc func(ctx context.Context, id string, before time.Time) (int64, error)
}

func (m *mockAuthTokenRepository) Create(ctx context.Context, token *model.Token) error {
	if m.createFunc != nil {
		return m.createFunc(ctx, token)
	}

	return nil
}

func (m *mockAuthTokenRepository) GetByID(ctx context.Context, id string) (*model.Token, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}

	return nil, database.ErrNotFound
}

func (m *mockAuthTokenRepository) TouchLastUsed(ctx context.Context, id string, at time.Time) error {
	if m.touchFunc != nil {
		return m.touchFunc(ctx, id, at)
	}

	return nil
}

func (m *mockAuthTokenRepository) Revoke(ctx context.Context, id string, at time.Time) error {
	if m.revokeFunc != nil {
		return m.revokeFunc(ctx, id, at)
	}

	return nil
}

func (m *mockAuthTokenRepository) RevokeByUser(
	ctx context.Context,
	userID string,
	at time.Time,
) (int64, error) {
	if m.revokeByUserFunc != nil {
		return m.revokeByUserFunc(ctx, userID, at)
	}

	return 0, nil
}

func (m *mockAuthTokenRepository) DeleteExpired(ctx context.Context, id string, before time.Time) (int64, error) {
	if m.deleteExpiredFunc != nil {
		return m.deleteExpiredFunc(ctx, id, before)
	}

	return 0, nil
}

// mockTokenCodec — мок подписи и разбора токенов: сервису важно, что он вызвал
// подпись и что разбор вернул, а не как устроен JWT.
type mockTokenCodec struct {
	signFunc  func(token *model.Token) (string, error)
	parseFunc func(raw string, now time.Time) (*model.Token, error)
}

func (m *mockTokenCodec) Sign(token *model.Token) (string, error) {
	if m.signFunc != nil {
		return m.signFunc(token)
	}

	return "signed." + token.ID, nil
}

func (m *mockTokenCodec) Parse(raw string, now time.Time) (*model.Token, error) {
	if m.parseFunc != nil {
		return m.parseFunc(raw, now)
	}

	return nil, errors.New("unexpected Parse() call")
}

// mockAuthEventPublisher запоминает опубликованные события.
type mockAuthEventPublisher struct {
	events []string
}

func (m *mockAuthEventPublisher) Publish(_ context.Context, event string) {
	m.events = append(m.events, event)
}

// authMocks — набор моков сервиса авторизации: тест заполняет только те методы,
// которые его интересуют.
type authMocks struct {
	users       *mockAuthUserRepository
	credentials *mockAuthCredentialRepository
	tokens      *mockAuthTokenRepository
	tenants     *mockAuthTenantRepository
	codec       *mockTokenCodec
	events      *mockAuthEventPublisher
}

// newAuthMocks создаёт моки с поведением по умолчанию.
func newAuthMocks() *authMocks {
	return &authMocks{
		users:       &mockAuthUserRepository{},
		credentials: &mockAuthCredentialRepository{},
		tokens:      &mockAuthTokenRepository{},
		tenants:     &mockAuthTenantRepository{},
		codec:       &mockTokenCodec{},
		events:      &mockAuthEventPublisher{},
	}
}

// newTestAuthService создаёт сервис с моками и фиксированными часами.
func newTestAuthService(t *testing.T, m *authMocks) *authService {
	t.Helper()

	created := NewAuthService(context.Background(), m.users, m.credentials, m.tokens, m.tenants, m.codec, m.events,
		testTokenTTL, testPasswordTTL)

	svc, ok := created.(*authService)
	if !ok {
		t.Fatalf("NewAuthService() returned %T, want *authService", created)
	}

	svc.now = func() time.Time { return testNow }

	return svc
}

// testAuthUser — пользователь, которого «нашёл» репозиторий. Роли у профиля нет:
// она живёт в членстве (tenant_members.role_id).
func testAuthUser(t *testing.T) *model.User {
	t.Helper()

	user, err := model.NewUser(model.CreateUserInput{
		Email:       testEmail,
		FirstName:   "Иван",
		LastName:    "Иванов",
		PhoneNumber: "+79990000001",
	}, testNow)
	if err != nil {
		t.Fatalf("model.NewUser() unexpected error = %v", err)
	}

	return user
}

// testMember собирает членство пользователя в тенанте с заданной ролью: именно
// отсюда сервис берёт тенант сессии и роль в нём.
func testMember(userID string, role model.Role) *model.TenantMember {
	return &model.TenantMember{
		TenantID: testTenantID,
		UserID:   userID,
		Role:     role,
		JoinedAt: testNow,
	}
}

// testCredentials — учётные данные с известным паролем и устаревшим (PBKDF2)
// хешем: так тесты входа проверяют и совместимость, и пересчёт хеша.
func testCredentials(userID string) *model.Credentials {
	return &model.Credentials{
		ID:          testCredentialID,
		UserID:      userID,
		Provider:    model.CredentialProviderEmail,
		ProviderKey: testEmail,
		Hash:        testPasswordHash,
		CreatedAt:   testNow,
		ExpiresAt:   testNow.Add(testPasswordTTL),
		IsActive:    true,
	}
}

func TestAuthService_Register(t *testing.T) {
	t.Run("создаёт пользователя, пароль, личный тенант и сессию", func(t *testing.T) {
		var (
			storedUser        *model.User
			storedCredentials *model.Credentials
			storedToken       *model.Token
			storedTenant      *model.Tenant
			storedMember      *model.TenantMember
		)

		m := newAuthMocks()
		m.users.createFunc = func(_ context.Context, user *model.User) error {
			storedUser = user

			return nil
		}
		m.credentials.createFunc = func(_ context.Context, credentials *model.Credentials) error {
			storedCredentials = credentials

			return nil
		}
		// Членства у нового пользователя нет: сервис создаёт личный тенант
		// вместе с членством владельца одной транзакцией.
		m.tenants.createWithOwnerFunc = func(
			_ context.Context,
			tenant *model.Tenant,
			member *model.TenantMember,
		) error {
			storedTenant = tenant
			storedMember = member

			return nil
		}
		m.tokens.createFunc = func(_ context.Context, token *model.Token) error {
			storedToken = token

			return nil
		}

		svc := newTestAuthService(t, m)

		user, session, err := svc.Register(context.Background(), model.RegisterInput{
			CreateUserInput: testCreateInput(),
			Password:        testPassword,
		})
		if err != nil {
			t.Fatalf("Register() unexpected error = %v", err)
		}

		if storedUser == nil {
			t.Fatal("пользователь не создан")
		}
		if storedUser.Email != testEmail {
			t.Errorf("email = %q, want %q", storedUser.Email, testEmail)
		}
		// Роль у профиля не хранится: владельцем личного тенанта становится
		// сам пользователь, и роль у него client (сотрудником он не рождается).
		if storedTenant == nil || storedTenant.Name != storedUser.FullName() {
			t.Errorf("личный тенант = %+v, want тенант пользователя %q", storedTenant, storedUser.FullName())
		}
		if storedMember == nil || storedMember.Role != model.RoleClient || !storedMember.IsOwner {
			t.Errorf("членство = %+v, want владельца с ролью %q", storedMember, model.RoleClient)
		}
		if storedCredentials == nil || storedCredentials.UserID != user.ID {
			t.Errorf("учётные данные = %+v, want для пользователя %q", storedCredentials, user.ID)
		}
		if storedCredentials.Hash == testPassword {
			t.Error("в базу попал открытый пароль")
		}
		if err := storedCredentials.Verify(testPassword); err != nil {
			t.Errorf("сохранённый пароль не проходит проверку: %v", err)
		}
		if storedToken == nil || storedToken.UserID != user.ID {
			t.Errorf("токен = %+v, want для пользователя %q", storedToken, user.ID)
		}
		if storedMember != nil && storedToken.TenantID != storedMember.TenantID {
			t.Errorf("TenantID сессии = %q, want %q", storedToken.TenantID, storedMember.TenantID)
		}
		if session.AccessToken == "" || session.Token != storedToken {
			t.Errorf("сессия = %+v, want подписанный токен с записью %+v", session, storedToken)
		}
		if len(m.events.events) != 1 || m.events.events[0] != EventUserRegistered {
			t.Errorf("события = %v, want [%s]", m.events.events, EventUserRegistered)
		}
	})

	t.Run("невалидный вход не доходит до репозитория", func(t *testing.T) {
		called := false

		m := newAuthMocks()
		m.users.createFunc = func(context.Context, *model.User) error {
			called = true

			return nil
		}

		svc := newTestAuthService(t, m)

		in := model.RegisterInput{CreateUserInput: testCreateInput(), Password: "short"}

		if _, _, err := svc.Register(context.Background(), in); !errors.Is(err, model.ErrInvalidPassword) {
			t.Errorf("Register() error = %v, want %v", err, model.ErrInvalidPassword)
		}
		if called {
			t.Error("Register() создал пользователя с невалидным паролем")
		}
	})

	t.Run("повторный email не создаёт пароль", func(t *testing.T) {
		credentialsCreated := false

		m := newAuthMocks()
		m.users.createFunc = func(context.Context, *model.User) error { return database.ErrDuplicate }
		m.credentials.createFunc = func(context.Context, *model.Credentials) error {
			credentialsCreated = true

			return nil
		}

		svc := newTestAuthService(t, m)

		in := model.RegisterInput{CreateUserInput: testCreateInput(), Password: testPassword}

		if _, _, err := svc.Register(context.Background(), in); !errors.Is(err, database.ErrDuplicate) {
			t.Errorf("Register() error = %v, want %v", err, database.ErrDuplicate)
		}
		if credentialsCreated {
			t.Error("Register() сохранил пароль при конфликте email")
		}
	})

	t.Run("сбой сохранения пароля откатывает пользователя", func(t *testing.T) {
		var deletedID string

		m := newAuthMocks()
		m.credentials.createFunc = func(context.Context, *model.Credentials) error {
			return errors.New("insert credentials failed")
		}
		m.users.deleteFunc = func(_ context.Context, id string) error {
			deletedID = id

			return nil
		}

		svc := newTestAuthService(t, m)

		in := model.RegisterInput{CreateUserInput: testCreateInput(), Password: testPassword}

		user, session, err := svc.Register(context.Background(), in)
		if err == nil {
			t.Fatal("Register() error = nil, want ошибку")
		}
		if user != nil || session != nil {
			t.Error("Register() вернул результат при ошибке")
		}
		if deletedID == "" {
			t.Error("Register() не удалил пользователя без пароля")
		}
	})
}

func TestAuthService_Login(t *testing.T) {
	t.Run("выдаёт токен и обновляет время входа", func(t *testing.T) {
		user := testAuthUser(t)

		var (
			updated       *model.User
			created       *model.Token
			cleanupCalled bool
		)

		m := newAuthMocks()
		m.users.getByEmailFunc = func(_ context.Context, email string) (*model.User, error) {
			// Сервис сам приводит адрес к нижнему регистру: вход не зависит
			// от того, как набран email.
			if email != "ivan@example.com" {
				t.Errorf("GetByEmail() email = %q, want %q", email, "ivan@example.com")
			}

			return user, nil
		}
		m.credentials.getByUserIDFunc = func(_ context.Context, userID string) (*model.Credentials, error) {
			return testCredentials(userID), nil
		}
		m.users.updateFunc = func(_ context.Context, got *model.User) error {
			updated = got

			return nil
		}
		m.tokens.createFunc = func(_ context.Context, token *model.Token) error {
			created = token

			return nil
		}
		m.tokens.deleteExpiredFunc = func(context.Context, string, time.Time) (int64, error) {
			cleanupCalled = true

			return 1, nil
		}

		svc := newTestAuthService(t, m)

		session, err := svc.Login(context.Background(), "IVAN@example.COM", testPassword, "")
		if err != nil {
			t.Fatalf("Login() unexpected error = %v", err)
		}

		if session.AccessToken == "" {
			t.Error("Login() вернул пустой токен")
		}
		if created == nil || created.UserID != user.ID {
			t.Errorf("запись сессии = %+v, want для пользователя %q", created, user.ID)
		}
		if !created.ExpiresAt.Equal(testNow.Add(testTokenTTL)) {
			t.Errorf("ExpiresAt = %v, want %v", created.ExpiresAt, testNow.Add(testTokenTTL))
		}
		if updated == nil || updated.LastLoginAt == nil || !updated.LastLoginAt.Equal(testNow) {
			t.Errorf("LastLoginAt = %+v, want %v", updated, testNow)
		}
		if !cleanupCalled {
			t.Error("Login() не убрал просроченные сессии")
		}
		if len(m.events.events) != 1 || m.events.events[0] != EventUserLoggedIn {
			t.Errorf("события = %v, want [%s]", m.events.events, EventUserLoggedIn)
		}
	})

	t.Run("неизвестный email", func(t *testing.T) {
		tokenCreated := false

		m := newAuthMocks()
		m.tokens.createFunc = func(context.Context, *model.Token) error {
			tokenCreated = true

			return nil
		}

		svc := newTestAuthService(t, m)

		if _, err := svc.Login(context.Background(), "nobody@example.com", testPassword, ""); !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("Login() error = %v, want %v", err, ErrInvalidCredentials)
		}
		if tokenCreated {
			t.Error("Login() выдал токен неизвестному пользователю")
		}
		if len(m.events.events) != 0 {
			t.Errorf("события = %v, want пусто", m.events.events)
		}
	})

	t.Run("у пользователя нет пароля", func(t *testing.T) {
		m := newAuthMocks()
		m.users.getByEmailFunc = func(context.Context, string) (*model.User, error) {
			return testAuthUser(t), nil
		}

		svc := newTestAuthService(t, m)

		if _, err := svc.Login(context.Background(), "ivan@example.com", testPassword, ""); !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("Login() error = %v, want %v", err, ErrInvalidCredentials)
		}
	})

	t.Run("неверный пароль", func(t *testing.T) {
		m := newAuthMocks()
		m.users.getByEmailFunc = func(context.Context, string) (*model.User, error) {
			return testAuthUser(t), nil
		}
		m.credentials.getByUserIDFunc = func(_ context.Context, userID string) (*model.Credentials, error) {
			return testCredentials(userID), nil
		}

		svc := newTestAuthService(t, m)

		_, err := svc.Login(context.Background(), "ivan@example.com", "wrong password entirely", "")
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("Login() error = %v, want %v", err, ErrInvalidCredentials)
		}
	})

	t.Run("ошибка репозитория не подменяется отказом в доступе", func(t *testing.T) {
		m := newAuthMocks()
		m.users.getByEmailFunc = func(context.Context, string) (*model.User, error) {
			return nil, errors.New("select user failed")
		}

		svc := newTestAuthService(t, m)

		_, err := svc.Login(context.Background(), "ivan@example.com", testPassword, "")
		if err == nil || errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("Login() error = %v, want ошибку репозитория", err)
		}
	})
}

func TestAuthService_Logout(t *testing.T) {
	user := testAuthUser(t)

	t.Run("отзывает сессию", func(t *testing.T) {
		var (
			revokedID string
			revokedAt time.Time
		)

		session := testSession(user, testNow.Add(time.Hour))

		m := newAuthMocks()
		m.codec.parseFunc = func(raw string, _ time.Time) (*model.Token, error) {
			if raw != "access-token" {
				t.Errorf("Parse() raw = %q, want %q", raw, "access-token")
			}

			return session, nil
		}
		m.tokens.revokeFunc = func(_ context.Context, id string, at time.Time) error {
			revokedID, revokedAt = id, at

			return nil
		}

		svc := newTestAuthService(t, m)

		if err := svc.Logout(context.Background(), "access-token"); err != nil {
			t.Fatalf("Logout() unexpected error = %v", err)
		}

		if revokedID != session.ID {
			t.Errorf("отозван токен %q, want %q", revokedID, session.ID)
		}
		if !revokedAt.Equal(testNow) {
			t.Errorf("время отзыва = %v, want %v", revokedAt, testNow)
		}
		if len(m.events.events) != 1 || m.events.events[0] != EventUserLoggedOut {
			t.Errorf("события = %v, want [%s]", m.events.events, EventUserLoggedOut)
		}
	})

	t.Run("неразбираемый токен", func(t *testing.T) {
		m := newAuthMocks()
		m.codec.parseFunc = func(string, time.Time) (*model.Token, error) {
			return nil, errors.New("token is expired")
		}

		svc := newTestAuthService(t, m)

		if err := svc.Logout(context.Background(), "garbage"); !errors.Is(err, ErrUnauthorized) {
			t.Errorf("Logout() error = %v, want %v", err, ErrUnauthorized)
		}
		if len(m.events.events) != 0 {
			t.Errorf("события = %v, want пусто", m.events.events)
		}
	})

	t.Run("ошибка отзыва пробрасывается", func(t *testing.T) {
		m := newAuthMocks()
		m.codec.parseFunc = func(string, time.Time) (*model.Token, error) {
			return testSession(user, testNow.Add(time.Hour)), nil
		}
		m.tokens.revokeFunc = func(context.Context, string, time.Time) error {
			return errors.New("update token failed")
		}

		svc := newTestAuthService(t, m)

		err := svc.Logout(context.Background(), "access-token")
		if err == nil || errors.Is(err, ErrUnauthorized) {
			t.Errorf("Logout() error = %v, want ошибку репозитория", err)
		}
	})
}

// testSession собирает запись сессии пользователя: срок действия задаётся явно,
// чтобы тесты проверяли и действующую, и просроченную сессию. Роль и тенант
// сессии хранятся в записи: роль — снимок на момент выдачи, тенант — тот, в
// котором сессия действует.
func testSession(user *model.User, expiresAt time.Time) *model.Token {
	return &model.Token{
		ID:        "1c9edc8e-1f1c-4d5c-8a5e-6f2f1d3c4b5a",
		UserID:    user.ID,
		TenantID:  testTenantID,
		Role:      model.RoleClient,
		IssuedAt:  testNow.Add(-time.Minute),
		ExpiresAt: expiresAt,
	}
}

func TestAuthService_Authenticate(t *testing.T) {
	user := testAuthUser(t)

	// newMocksWithSession — моки, в которых подпись сходится, а сессия найдена.
	// Роль берётся из членства в тенанте сессии, поэтому мок членства отдаёт её.
	newMocksWithSession := func(session *model.Token) *authMocks {
		m := newAuthMocks()
		m.codec.parseFunc = func(string, time.Time) (*model.Token, error) { return session, nil }
		m.tokens.getByIDFunc = func(context.Context, string) (*model.Token, error) { return session, nil }
		m.users.getByIDFunc = func(context.Context, string) (*model.User, error) { return user, nil }
		m.tenants.getMemberFunc = func(context.Context, string, string) (*model.TenantMember, error) {
			return testMember(user.ID, model.RoleOperator), nil
		}

		return m
	}

	t.Run("возвращает действующего пользователя и роль из членства", func(t *testing.T) {
		session := testSession(user, testNow.Add(time.Hour))

		var touchedAt time.Time

		m := newMocksWithSession(session)
		m.tokens.touchFunc = func(_ context.Context, id string, at time.Time) error {
			if id != session.ID {
				t.Errorf("TouchLastUsed() id = %q, want %q", id, session.ID)
			}

			touchedAt = at

			return nil
		}

		svc := newTestAuthService(t, m)

		principal, err := svc.Authenticate(context.Background(), "access-token")
		if err != nil {
			t.Fatalf("Authenticate() unexpected error = %v", err)
		}

		if principal.User != user || principal.Token != session {
			t.Errorf("Principal = %+v, want пользователя %q и сессию %q", principal, user.ID, session.ID)
		}
		if principal.ID() != user.ID || principal.Role != model.RoleOperator {
			t.Errorf("Principal = %q/%q, want %q/%q", principal.ID(), principal.Role, user.ID, model.RoleOperator)
		}
		if principal.TenantID() != testTenantID {
			t.Errorf("TenantID() = %q, want %q", principal.TenantID(), testTenantID)
		}
		if !principal.IsStaff() {
			t.Error("IsStaff() = false для оператора тенанта, want true")
		}
		if !touchedAt.Equal(testNow) {
			t.Errorf("время активности = %v, want %v", touchedAt, testNow)
		}
	})

	t.Run("без членства сессия недействительна", func(t *testing.T) {
		session := testSession(user, testNow.Add(time.Hour))
		session.Role = model.RoleClient

		m := newAuthMocks()
		m.codec.parseFunc = func(string, time.Time) (*model.Token, error) { return session, nil }
		m.tokens.getByIDFunc = func(context.Context, string) (*model.Token, error) { return session, nil }
		m.users.getByIDFunc = func(context.Context, string) (*model.User, error) { return user, nil }
		// По умолчанию мок членства отвечает database.ErrNotFound: членство
		// удалено, поэтому снимок роли из токена больше не действует.

		svc := newTestAuthService(t, m)

		if _, err := svc.Authenticate(context.Background(), "access-token"); !errors.Is(err, ErrUnauthorized) {
			t.Errorf("Authenticate() error = %v, want %v", err, ErrUnauthorized)
		}
	})

	t.Run("изменение роли в тенанте действует без перевыпуска токена", func(t *testing.T) {
		session := testSession(user, testNow.Add(time.Hour))
		session.Role = model.RoleClient

		m := newMocksWithSession(session)
		m.tenants.getMemberFunc = func(context.Context, string, string) (*model.TenantMember, error) {
			return testMember(user.ID, model.RoleAdmin), nil
		}

		svc := newTestAuthService(t, m)

		principal, err := svc.Authenticate(context.Background(), "access-token")
		if err != nil {
			t.Fatalf("Authenticate() unexpected error = %v", err)
		}

		if principal.Role != model.RoleAdmin {
			t.Errorf("Role = %q, want %q: роль берётся из членства, а не из токена",
				principal.Role, model.RoleAdmin)
		}
	})

	t.Run("активность не пишется чаще раза в час", func(t *testing.T) {
		session := testSession(user, testNow.Add(time.Hour))
		recent := testNow.Add(-time.Minute)
		session.LastUsedAt = &recent

		called := false

		m := newMocksWithSession(session)
		m.tokens.touchFunc = func(context.Context, string, time.Time) error {
			called = true

			return nil
		}

		svc := newTestAuthService(t, m)

		if _, err := svc.Authenticate(context.Background(), "access-token"); err != nil {
			t.Fatalf("Authenticate() unexpected error = %v", err)
		}
		if called {
			t.Error("Authenticate() записал активность повторно в пределах часа")
		}
	})

	revoked := testSession(user, testNow.Add(time.Hour))
	revoked.Revoke(testNow.Add(-time.Minute))

	otherUser := testSession(user, testNow.Add(time.Hour))
	otherUser.UserID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"

	tests := []struct {
		name    string
		token   string
		prepare func(m *authMocks, session *model.Token)
		want    error
	}{
		{
			name:  "токен не передан",
			token: "",
			want:  ErrUnauthorized,
		},
		{
			name:  "подпись не сходится",
			token: "access-token",
			prepare: func(m *authMocks, _ *model.Token) {
				m.codec.parseFunc = func(string, time.Time) (*model.Token, error) {
					return nil, errors.New("token signature is invalid")
				}
			},
			want: ErrUnauthorized,
		},
		{
			name:  "сессии нет в базе",
			token: "access-token",
			prepare: func(m *authMocks, _ *model.Token) {
				m.tokens.getByIDFunc = func(context.Context, string) (*model.Token, error) {
					return nil, database.ErrNotFound
				}
			},
			want: ErrUnauthorized,
		},
		{
			name:  "токен ссылается на чужую сессию",
			token: "access-token",
			prepare: func(m *authMocks, _ *model.Token) {
				m.codec.parseFunc = func(string, time.Time) (*model.Token, error) { return otherUser, nil }
			},
			want: ErrUnauthorized,
		},
		{
			name:  "сессия отозвана",
			token: "access-token",
			prepare: func(m *authMocks, _ *model.Token) {
				m.codec.parseFunc = func(string, time.Time) (*model.Token, error) { return revoked, nil }
				m.tokens.getByIDFunc = func(context.Context, string) (*model.Token, error) { return revoked, nil }
			},
			want: ErrUnauthorized,
		},
		{
			name:  "срок действия истёк",
			token: "access-token",
			prepare: func(m *authMocks, _ *model.Token) {
				expired := testSession(user, testNow)
				m.codec.parseFunc = func(string, time.Time) (*model.Token, error) { return expired, nil }
				m.tokens.getByIDFunc = func(context.Context, string) (*model.Token, error) { return expired, nil }
			},
			want: ErrUnauthorized,
		},
		{
			name:  "пользователь удалён",
			token: "access-token",
			prepare: func(m *authMocks, _ *model.Token) {
				m.users.getByIDFunc = func(context.Context, string) (*model.User, error) {
					return nil, database.ErrNotFound
				}
			},
			want: ErrUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := testSession(user, testNow.Add(time.Hour))
			m := newMocksWithSession(session)

			if tt.prepare != nil {
				tt.prepare(m, session)
			}

			svc := newTestAuthService(t, m)

			if _, err := svc.Authenticate(context.Background(), tt.token); !errors.Is(err, tt.want) {
				t.Errorf("Authenticate() error = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("ошибка базы не маскируется под 401", func(t *testing.T) {
		m := newAuthMocks()
		m.codec.parseFunc = func(string, time.Time) (*model.Token, error) {
			return testSession(user, testNow.Add(time.Hour)), nil
		}
		m.tokens.getByIDFunc = func(context.Context, string) (*model.Token, error) {
			return nil, errors.New("select token failed")
		}

		svc := newTestAuthService(t, m)

		_, err := svc.Authenticate(context.Background(), "access-token")
		if err == nil || errors.Is(err, ErrUnauthorized) {
			t.Errorf("Authenticate() error = %v, want ошибку базы", err)
		}
	})
}

// expiredCredentials возвращает учётные данные с просроченным паролем: срок
// действия (expires_at) истёк раньше testNow.
func expiredCredentials(userID string) *model.Credentials {
	credentials := testCredentials(userID)
	credentials.ExpiresAt = testNow.Add(-time.Hour)

	return credentials
}

func TestAuthService_Login_PasswordExpired(t *testing.T) {
	user := testAuthUser(t)

	tokenCreated := false

	m := newAuthMocks()
	m.users.getByEmailFunc = func(context.Context, string) (*model.User, error) {
		return user, nil
	}
	m.credentials.getByUserIDFunc = func(_ context.Context, userID string) (*model.Credentials, error) {
		return expiredCredentials(userID), nil
	}
	m.tokens.createFunc = func(context.Context, *model.Token) error {
		tokenCreated = true

		return nil
	}

	svc := newTestAuthService(t, m)

	_, err := svc.Login(context.Background(), "ivan@example.com", testPassword, "")
	if !errors.Is(err, ErrPasswordExpired) {
		t.Fatalf("Login() error = %v, want %v", err, ErrPasswordExpired)
	}
	if tokenCreated {
		t.Error("Login() выдал токен при просроченном пароле")
	}
	if len(m.events.events) != 0 {
		t.Errorf("события = %v, want пусто", m.events.events)
	}
}

func TestAuthService_Login_UpgradesLegacyHash(t *testing.T) {
	t.Run("прежний хеш пересчитывается в Argon2id", func(t *testing.T) {
		user := testAuthUser(t)

		stored := testCredentials(user.ID)
		var updated *model.Credentials

		m := newAuthMocks()
		m.users.getByEmailFunc = func(context.Context, string) (*model.User, error) {
			return user, nil
		}
		m.credentials.getByUserIDFunc = func(context.Context, string) (*model.Credentials, error) {
			return stored, nil
		}
		m.credentials.updateFunc = func(_ context.Context, credentials *model.Credentials) error {
			updated = credentials

			return nil
		}

		svc := newTestAuthService(t, m)

		if _, err := svc.Login(context.Background(), "ivan@example.com", testPassword, ""); err != nil {
			t.Fatalf("Login() unexpected error = %v", err)
		}

		if updated == nil {
			t.Fatal("Login() не обновил устаревший хеш")
		}
		if !strings.HasPrefix(updated.Hash, "argon2id$") {
			t.Errorf("Hash = %q, want Argon2id", updated.Hash)
		}
		if !updated.ExpiresAt.Equal(stored.ExpiresAt) {
			t.Errorf("ExpiresAt = %v, want %v: пересчёт не должен сбрасывать срок пароля",
				updated.ExpiresAt, stored.ExpiresAt)
		}
	})

	t.Run("актуальный хеш не пересчитывается", func(t *testing.T) {
		user := testAuthUser(t)

		hash, err := model.HashPassword(testPassword)
		if err != nil {
			t.Fatalf("model.HashPassword() unexpected error = %v", err)
		}

		updateCalled := false

		m := newAuthMocks()
		m.users.getByEmailFunc = func(context.Context, string) (*model.User, error) {
			return user, nil
		}
		m.credentials.getByUserIDFunc = func(_ context.Context, userID string) (*model.Credentials, error) {
			credentials := testCredentials(userID)
			credentials.Hash = hash

			return credentials, nil
		}
		m.credentials.updateFunc = func(context.Context, *model.Credentials) error {
			updateCalled = true

			return nil
		}

		svc := newTestAuthService(t, m)

		if _, err := svc.Login(context.Background(), "ivan@example.com", testPassword, ""); err != nil {
			t.Fatalf("Login() unexpected error = %v", err)
		}

		if updateCalled {
			t.Error("Login() пересчитал актуальный Argon2id-хеш")
		}
	})
}

func TestAuthService_ChangePassword(t *testing.T) {
	const newPassword = "brand new password phrase"

	t.Run("меняет пароль и выдаёт сессию", func(t *testing.T) {
		user := testAuthUser(t)

		var (
			updated   *model.Credentials
			created   *model.Token
			loginTime *model.User
		)

		m := newAuthMocks()
		m.users.getByEmailFunc = func(_ context.Context, email string) (*model.User, error) {
			if email != "ivan@example.com" {
				t.Errorf("GetByEmail() email = %q, want %q", email, "ivan@example.com")
			}

			return user, nil
		}
		// Пароль просрочен: смена — единственный способ войти.
		m.credentials.getByUserIDFunc = func(_ context.Context, userID string) (*model.Credentials, error) {
			return expiredCredentials(userID), nil
		}
		m.credentials.updateFunc = func(_ context.Context, credentials *model.Credentials) error {
			updated = credentials

			return nil
		}
		m.users.updateFunc = func(_ context.Context, got *model.User) error {
			loginTime = got

			return nil
		}
		m.tokens.createFunc = func(_ context.Context, token *model.Token) error {
			created = token

			return nil
		}

		svc := newTestAuthService(t, m)

		session, err := svc.ChangePassword(context.Background(), "IVAN@example.COM", testPassword, newPassword, "")
		if err != nil {
			t.Fatalf("ChangePassword() unexpected error = %v", err)
		}

		if session == nil || session.AccessToken == "" {
			t.Fatal("ChangePassword() не выдал токен")
		}
		if updated == nil {
			t.Fatal("ChangePassword() не сохранил новый пароль")
		}
		if err := updated.Verify(newPassword); err != nil {
			t.Errorf("новый пароль не проходит проверку: %v", err)
		}
		if !updated.ExpiresAt.Equal(testNow.Add(testPasswordTTL)) {
			t.Errorf("ExpiresAt = %v, want %v: срок пароля начинается заново",
				updated.ExpiresAt, testNow.Add(testPasswordTTL))
		}
		if created == nil || created.UserID != user.ID {
			t.Errorf("запись сессии = %+v, want для пользователя %q", created, user.ID)
		}
		if loginTime == nil || loginTime.LastLoginAt == nil || !loginTime.LastLoginAt.Equal(testNow) {
			t.Errorf("LastLoginAt = %+v, want %v", loginTime, testNow)
		}
		if len(m.events.events) != 1 || m.events.events[0] != EventUserPasswordChanged {
			t.Errorf("события = %v, want [%s]", m.events.events, EventUserPasswordChanged)
		}
	})

	t.Run("неверный текущий пароль", func(t *testing.T) {
		user := testAuthUser(t)

		updatedCalled := false

		m := newAuthMocks()
		m.users.getByEmailFunc = func(context.Context, string) (*model.User, error) {
			return user, nil
		}
		m.credentials.getByUserIDFunc = func(_ context.Context, userID string) (*model.Credentials, error) {
			return testCredentials(userID), nil
		}
		m.credentials.updateFunc = func(context.Context, *model.Credentials) error {
			updatedCalled = true

			return nil
		}

		svc := newTestAuthService(t, m)

		_, err := svc.ChangePassword(context.Background(), "ivan@example.com", "wrong password entirely", newPassword, "")
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("ChangePassword() error = %v, want %v", err, ErrInvalidCredentials)
		}
		if updatedCalled {
			t.Error("ChangePassword() сохранил пароль при неверном текущем")
		}
	})

	t.Run("неизвестный email", func(t *testing.T) {
		m := newAuthMocks()

		svc := newTestAuthService(t, m)

		_, err := svc.ChangePassword(context.Background(), "nobody@example.com", testPassword, newPassword, "")
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("ChangePassword() error = %v, want %v", err, ErrInvalidCredentials)
		}
	})

	t.Run("слабый новый пароль не сохраняется", func(t *testing.T) {
		user := testAuthUser(t)

		updateCalled := false

		m := newAuthMocks()
		m.users.getByEmailFunc = func(context.Context, string) (*model.User, error) {
			return user, nil
		}
		m.credentials.getByUserIDFunc = func(_ context.Context, userID string) (*model.Credentials, error) {
			return testCredentials(userID), nil
		}
		m.credentials.updateFunc = func(context.Context, *model.Credentials) error {
			updateCalled = true

			return nil
		}

		svc := newTestAuthService(t, m)

		_, err := svc.ChangePassword(context.Background(), "ivan@example.com", testPassword, "short", "")
		if !errors.Is(err, model.ErrInvalidPassword) {
			t.Errorf("ChangePassword() error = %v, want %v", err, model.ErrInvalidPassword)
		}
		if updateCalled {
			t.Error("ChangePassword() сохранил слабый пароль")
		}
	})

	t.Run("ошибка репозитория не подменяется отказом в доступе", func(t *testing.T) {
		m := newAuthMocks()
		m.users.getByEmailFunc = func(context.Context, string) (*model.User, error) {
			return nil, errors.New("select user failed")
		}

		svc := newTestAuthService(t, m)

		_, err := svc.ChangePassword(context.Background(), "ivan@example.com", testPassword, newPassword, "")
		if err == nil || errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("ChangePassword() error = %v, want ошибку репозитория", err)
		}
	})
}

func TestAuthService_SwitchTenant(t *testing.T) {
	t.Run("выдаёт сессию в выбранном тенанте", func(t *testing.T) {
		tenant := activeTenant(otherTenantID)
		tenant.Slug = "acme"

		var created *model.Token

		m := newAuthMocks()
		m.tenants.getMemberBySlugFunc = func(_ context.Context, slug, userID string) (*model.TenantMembership, error) {
			if slug != "acme" || userID != testPrincipal(t, model.RoleClient).User.ID {
				t.Errorf("GetMemberBySlug() = %q/%q", slug, userID)
			}

			return &model.TenantMembership{
				Tenant: *tenant,
				Member: model.TenantMember{
					TenantID: tenant.ID,
					UserID:   userID,
					Role:     model.RoleManager,
				},
			}, nil
		}
		m.tokens.createFunc = func(_ context.Context, token *model.Token) error {
			created = token

			return nil
		}

		svc := newTestAuthService(t, m)

		session, err := svc.SwitchTenant(context.Background(), testPrincipal(t, model.RoleClient), "acme")
		if err != nil {
			t.Fatalf("SwitchTenant() unexpected error = %v", err)
		}

		if session.Tenant == nil || session.Tenant.ID != otherTenantID || session.Role != model.RoleManager {
			t.Errorf("session = %+v", session)
		}

		if created == nil || created.TenantID != otherTenantID || created.Role != model.RoleManager {
			t.Errorf("token = %+v", created)
		}
	})

	t.Run("чужой тенант — запрещено", func(t *testing.T) {
		m := newAuthMocks()
		m.tenants.getMemberBySlugFunc = func(context.Context, string, string) (*model.TenantMembership, error) {
			return nil, database.ErrNotFound
		}

		svc := newTestAuthService(t, m)

		_, err := svc.SwitchTenant(context.Background(), testPrincipal(t, model.RoleClient), "other")
		if !errors.Is(err, ErrForbidden) {
			t.Errorf("SwitchTenant() error = %v, want %v", err, ErrForbidden)
		}
	})
}

func TestAuthService_ListMyTenants(t *testing.T) {
	memberships := []*model.TenantMembership{
		{Tenant: *activeTenant(testTenantID), Member: model.TenantMember{TenantID: testTenantID, Role: model.RoleClient}},
	}

	m := newAuthMocks()
	m.tenants.listMembershipsFunc = func(_ context.Context, userID string) ([]*model.TenantMembership, error) {
		if userID == "" {
			t.Error("ListMemberships() userID пуст")
		}

		return memberships, nil
	}

	svc := newTestAuthService(t, m)

	got, err := svc.ListMyTenants(context.Background(), testPrincipal(t, model.RoleClient))
	if err != nil || len(got) != 1 || got[0].Tenant.ID != testTenantID {
		t.Fatalf("ListMyTenants() = %v, %v", got, err)
	}
}

func TestAuthService_LogoutAll(t *testing.T) {
	var revokedUser string

	m := newAuthMocks()
	m.tokens.revokeByUserFunc = func(_ context.Context, userID string, _ time.Time) (int64, error) {
		revokedUser = userID

		return 3, nil
	}

	svc := newTestAuthService(t, m)
	actor := testPrincipal(t, model.RoleClient)

	if err := svc.LogoutAll(context.Background(), actor); err != nil {
		t.Fatalf("LogoutAll() unexpected error = %v", err)
	}

	if revokedUser != actor.User.ID {
		t.Errorf("revoked user = %q, want %q", revokedUser, actor.User.ID)
	}

	if len(m.events.events) != 1 || m.events.events[0] != EventUserLoggedOut {
		t.Errorf("events = %v", m.events.events)
	}
}

func TestAuthService_RegisterByTenantLink(t *testing.T) {
	t.Run("создаёт клиентское членство без личного тенанта", func(t *testing.T) {
		tenant := activeTenant(otherTenantID)
		tenant.Slug = "acme"

		var (
			addedMember *model.TenantMember
			personal    bool
		)

		m := newAuthMocks()
		m.tenants.getBySlugFunc = func(_ context.Context, slug string) (*model.Tenant, error) {
			if slug != "acme" {
				t.Errorf("GetBySlug() slug = %q", slug)
			}

			return tenant, nil
		}
		m.tenants.addMemberFunc = func(_ context.Context, member *model.TenantMember) error {
			addedMember = member

			return nil
		}
		m.tenants.createWithOwnerFunc = func(context.Context, *model.Tenant, *model.TenantMember) error {
			personal = true

			return nil
		}

		svc := newTestAuthService(t, m)

		user, session, err := svc.Register(context.Background(), model.RegisterInput{
			CreateUserInput: testCreateInput(),
			Password:        testPassword,
			TenantSlug:      "acme",
		})
		if err != nil {
			t.Fatalf("Register() unexpected error = %v", err)
		}

		if personal {
			t.Error("Register() создал личный тенант, want членство по ссылке")
		}

		if addedMember == nil || addedMember.TenantID != tenant.ID ||
			addedMember.UserID != user.ID || addedMember.Role != model.RoleClient || addedMember.IsOwner {
			t.Errorf("member = %+v", addedMember)
		}

		if session.Tenant == nil || session.Tenant.ID != tenant.ID {
			t.Errorf("session tenant = %+v", session.Tenant)
		}
	})

	t.Run("приостановленный тенант — запрещено", func(t *testing.T) {
		m := newAuthMocks()
		m.tenants.getBySlugFunc = func(context.Context, string) (*model.Tenant, error) {
			return &model.Tenant{ID: otherTenantID, Slug: "acme", Status: model.TenantStatusSuspended}, nil
		}

		svc := newTestAuthService(t, m)

		_, _, err := svc.Register(context.Background(), model.RegisterInput{
			CreateUserInput: testCreateInput(),
			Password:        testPassword,
			TenantSlug:      "acme",
		})
		if !errors.Is(err, ErrForbidden) {
			t.Errorf("Register() error = %v, want %v", err, ErrForbidden)
		}
	})
}
