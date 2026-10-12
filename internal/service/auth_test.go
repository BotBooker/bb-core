package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/model"
)

const (
	// testTokenTTL — срок жизни токена в тестах: значение не влияет на проверки,
	// они смотрят на выданную запись, а не на конкретное число.
	testTokenTTL = 24 * time.Hour
	// testPassword — пароль тестовых учётных данных.
	testPassword = "correct horse battery staple"
	// testPasswordHash — хеш testPassword, посчитанный заранее: PBKDF2 с 210 000
	// итераций стоит ~0,2 с, и платить их в каждом тесте незачем (само
	// хеширование проверяется в тестах модели).
	testPasswordHash = "pbkdf2_sha256$210000$dQIjm0wTireOlDo8WomRug$+qvx5cWbyYARbq061iEy9aedaWfstobFn25SQIDvZbc"
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

// mockAuthTokenRepository — мок репозитория сессий.
type mockAuthTokenRepository struct {
	createFunc        func(ctx context.Context, token *model.Token) error
	getByIDFunc       func(ctx context.Context, id string) (*model.Token, error)
	touchFunc         func(ctx context.Context, id string, at time.Time) error
	revokeFunc        func(ctx context.Context, id string, at time.Time) error
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

func (m *mockAuthEventPublisher) Publish(event string) {
	m.events = append(m.events, event)
}

// authMocks — набор моков сервиса авторизации: тест заполняет только те методы,
// которые его интересуют.
type authMocks struct {
	users       *mockAuthUserRepository
	credentials *mockAuthCredentialRepository
	tokens      *mockAuthTokenRepository
	codec       *mockTokenCodec
	events      *mockAuthEventPublisher
}

// newAuthMocks создаёт моки с поведением по умолчанию.
func newAuthMocks() *authMocks {
	return &authMocks{
		users:       &mockAuthUserRepository{},
		credentials: &mockAuthCredentialRepository{},
		tokens:      &mockAuthTokenRepository{},
		codec:       &mockTokenCodec{},
		events:      &mockAuthEventPublisher{},
	}
}

// newTestAuthService создаёт сервис с моками и фиксированными часами.
func newTestAuthService(t *testing.T, m *authMocks) *authService {
	t.Helper()

	created := NewAuthService(m.users, m.credentials, m.tokens, m.codec, m.events, testTokenTTL)

	svc, ok := created.(*authService)
	if !ok {
		t.Fatalf("NewAuthService() returned %T, want *authService", created)
	}

	svc.now = func() time.Time { return testNow }

	return svc
}

// testAuthUser — пользователь, которого «нашёл» репозиторий.
func testAuthUser(t *testing.T, role model.Role) *model.User {
	t.Helper()

	user, err := model.NewUser(model.CreateUserInput{
		Email:       "ivan@example.com",
		FirstName:   "Иван",
		LastName:    "Иванов",
		PhoneNumber: "+79990000001",
	}, testNow)
	if err != nil {
		t.Fatalf("model.NewUser() unexpected error = %v", err)
	}

	user.Role = role

	return user
}

// testCredentials — учётные данные с известным паролем.
func testCredentials(userID string) *model.Credentials {
	return &model.Credentials{
		UserID:    userID,
		Hash:      testPasswordHash,
		CreatedAt: testNow,
		UpdatedAt: testNow,
	}
}

func TestAuthService_Register(t *testing.T) {
	t.Run("создаёт пользователя, пароль и сессию", func(t *testing.T) {
		var storedUser *model.User
		var storedCredentials *model.Credentials
		var storedToken *model.Token

		m := newAuthMocks()
		m.users.createFunc = func(_ context.Context, user *model.User) error {
			storedUser = user

			return nil
		}
		m.credentials.createFunc = func(_ context.Context, credentials *model.Credentials) error {
			storedCredentials = credentials

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

		if storedUser == nil || storedUser.Role != model.RoleClient {
			t.Errorf("созданный пользователь = %+v, want роль %q", storedUser, model.RoleClient)
		}
		if storedUser.Email != "ivan@example.com" {
			t.Errorf("email = %q, want %q", storedUser.Email, "ivan@example.com")
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
		user := testAuthUser(t, model.RoleClient)

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

		session, err := svc.Login(context.Background(), "IVAN@example.COM", testPassword)
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

		if _, err := svc.Login(context.Background(), "nobody@example.com", testPassword); !errors.Is(err, ErrInvalidCredentials) {
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
			return testAuthUser(t, model.RoleClient), nil
		}

		svc := newTestAuthService(t, m)

		if _, err := svc.Login(context.Background(), "ivan@example.com", testPassword); !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("Login() error = %v, want %v", err, ErrInvalidCredentials)
		}
	})

	t.Run("неверный пароль", func(t *testing.T) {
		m := newAuthMocks()
		m.users.getByEmailFunc = func(context.Context, string) (*model.User, error) {
			return testAuthUser(t, model.RoleClient), nil
		}
		m.credentials.getByUserIDFunc = func(_ context.Context, userID string) (*model.Credentials, error) {
			return testCredentials(userID), nil
		}

		svc := newTestAuthService(t, m)

		_, err := svc.Login(context.Background(), "ivan@example.com", "wrong password entirely")
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

		_, err := svc.Login(context.Background(), "ivan@example.com", testPassword)
		if err == nil || errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("Login() error = %v, want ошибку репозитория", err)
		}
	})
}

func TestAuthService_Logout(t *testing.T) {
	user := testAuthUser(t, model.RoleClient)

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
// чтобы тесты проверяли и действующую, и просроченную сессию.
func testSession(user *model.User, expiresAt time.Time) *model.Token {
	return &model.Token{
		ID:        "1c9edc8e-1f1c-4d5c-8a5e-6f2f1d3c4b5a",
		UserID:    user.ID,
		Role:      user.Role,
		IssuedAt:  testNow.Add(-time.Minute),
		ExpiresAt: expiresAt,
	}
}

func TestAuthService_Authenticate(t *testing.T) {
	user := testAuthUser(t, model.RoleOperator)

	// newMocksWithSession — моки, в которых подпись сходится, а сессия найдена.
	newMocksWithSession := func(session *model.Token) *authMocks {
		m := newAuthMocks()
		m.codec.parseFunc = func(string, time.Time) (*model.Token, error) { return session, nil }
		m.tokens.getByIDFunc = func(context.Context, string) (*model.Token, error) { return session, nil }
		m.users.getByIDFunc = func(context.Context, string) (*model.User, error) { return user, nil }

		return m
	}

	t.Run("возвращает действующего пользователя", func(t *testing.T) {
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
		if principal.ID() != user.ID || principal.Role() != model.RoleOperator {
			t.Errorf("Principal = %q/%q, want %q/%q", principal.ID(), principal.Role(), user.ID, model.RoleOperator)
		}
		if !touchedAt.Equal(testNow) {
			t.Errorf("время активности = %v, want %v", touchedAt, testNow)
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
