package servers

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/model"
	"github.com/botbooker/bb-core/internal/service"
	auth_v1 "github.com/botbooker/bb-core/pkg/proto/auth/v1"
	user_v1 "github.com/botbooker/bb-core/pkg/proto/user/v1"
)

// authTestSession — сессия, как её вернул бы доменный сервис авторизации.
func authTestSession(t *testing.T) *service.Session {
	t.Helper()

	user := handlerTestUser(t)

	// Сессия действует в тенанте, а роль — атрибут членства: в записи хранится
	// снимок роли на момент выдачи.
	token, err := model.NewToken(user, testTenantID, model.RoleClient, time.Hour, time.Now().UTC())
	if err != nil {
		t.Fatalf("model.NewToken() unexpected error = %v", err)
	}

	return &service.Session{AccessToken: "signed.jwt.token", Token: token}
}

// validRegisterRequest — валидный по proto-правилам запрос регистрации.
func validRegisterRequest() *auth_v1.AuthServiceRegisterRequest {
	return &auth_v1.AuthServiceRegisterRequest{
		Email:     "ivan@example.com",
		FirstName: "Иван",
		LastName:  "Иванов",
		Gender:    user_v1.Gender_GENDER_MALE,
		Password:  "secret-password",
	}
}

func TestNewAuthServiceServer(t *testing.T) {
	if NewAuthServiceServer(&mockAuthService{}) == nil {
		t.Fatal("NewAuthServiceServer() returned nil")
	}
}

func TestAuthServiceServer_Register(t *testing.T) {
	session := authTestSession(t)
	user := handlerTestUser(t)

	var got model.RegisterInput

	auth := &mockAuthService{registerFunc: func(_ context.Context, in model.RegisterInput) (*model.User, *service.Session, error) {
		got = in

		return user, session, nil
	}}

	server := NewAuthServiceServer(auth)

	resp, err := server.Register(context.Background(), validRegisterRequest())
	if err != nil {
		t.Fatalf("Register() unexpected error = %v", err)
	}

	if resp.GetUser().GetUuid() != user.ID {
		t.Errorf("Register() user uuid = %q, want %q", resp.GetUser().GetUuid(), user.ID)
	}

	if resp.GetSession().GetToken() != session.AccessToken {
		t.Errorf("Register() session token = %q, want %q", resp.GetSession().GetToken(), session.AccessToken)
	}

	if !resp.GetSession().GetExpiresAt().AsTime().Equal(session.Token.ExpiresAt) {
		t.Errorf("Register() expires_at = %v, want %v", resp.GetSession().GetExpiresAt().AsTime(), session.Token.ExpiresAt)
	}

	if got.Password != "secret-password" || got.Email != "ivan@example.com" {
		t.Errorf("Register() input = %+v", got)
	}

	if got.Gender != model.GenderMale {
		t.Errorf("Register() gender = %q, want %q", got.Gender, model.GenderMale)
	}
}

func TestAuthServiceServer_Register_InvalidRequest(t *testing.T) {
	auth := &mockAuthService{registerFunc: func(context.Context, model.RegisterInput) (*model.User, *service.Session, error) {
		t.Error("Register() дошёл до сервиса с невалидным запросом")

		return nil, nil, nil
	}}

	server := NewAuthServiceServer(auth)

	req := validRegisterRequest()
	req.Email = "не-email"

	_, err := server.Register(context.Background(), req)
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("Register() code = %v, want %v (err = %v)", status.Code(err), codes.InvalidArgument, err)
	}
}

func TestAuthServiceServer_Login(t *testing.T) {
	session := authTestSession(t)

	auth := &mockAuthService{loginFunc: func(_ context.Context, email, password string) (*service.Session, error) {
		if email != "ivan@example.com" || password != "secret-password" {
			t.Errorf("Login() args = %q, %q", email, password)
		}

		return session, nil
	}}

	server := NewAuthServiceServer(auth)

	resp, err := server.Login(context.Background(), &auth_v1.AuthServiceLoginRequest{
		Email:    "ivan@example.com",
		Password: "secret-password",
	})
	if err != nil {
		t.Fatalf("Login() unexpected error = %v", err)
	}

	if resp.GetSession().GetToken() != session.AccessToken {
		t.Errorf("Login() session token = %q, want %q", resp.GetSession().GetToken(), session.AccessToken)
	}
}

func TestAuthServiceServer_Login_InvalidCredentials(t *testing.T) {
	auth := &mockAuthService{loginFunc: func(context.Context, string, string) (*service.Session, error) {
		return nil, service.ErrInvalidCredentials
	}}

	server := NewAuthServiceServer(auth)

	_, err := server.Login(context.Background(), &auth_v1.AuthServiceLoginRequest{
		Email:    "ivan@example.com",
		Password: "wrong-password",
	})
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("Login() code = %v, want %v (err = %v)", status.Code(err), codes.Unauthenticated, err)
	}
}

func TestAuthServiceServer_ChangePassword(t *testing.T) {
	session := authTestSession(t)

	auth := &mockAuthService{changePasswordFunc: func(
		_ context.Context,
		email, currentPassword, newPassword string,
	) (*service.Session, error) {
		if email != "ivan@example.com" || currentPassword != "old-password" || newPassword != "new-password" {
			t.Errorf("ChangePassword() args = %q, %q, %q", email, currentPassword, newPassword)
		}

		return session, nil
	}}

	server := NewAuthServiceServer(auth)

	_, err := server.ChangePassword(context.Background(), &auth_v1.AuthServiceChangePasswordRequest{
		Email:           "ivan@example.com",
		CurrentPassword: "old-password",
		NewPassword:     "new-password",
	})
	if err != nil {
		t.Fatalf("ChangePassword() unexpected error = %v", err)
	}
}

// metadataContext кладёт токен в метаданные вызова так, как это делает
// grpc-gateway или gRPC-клиент.
func metadataContext(token string) context.Context {
	return metadata.NewIncomingContext(
		context.Background(),
		metadata.Pairs("authorization", bearerPrefix+token),
	)
}

func TestAuthServiceServer_Logout(t *testing.T) {
	var got string

	auth := &mockAuthService{logoutFunc: func(_ context.Context, token string) error {
		got = token

		return nil
	}}

	server := NewAuthServiceServer(auth)

	if _, err := server.Logout(metadataContext("signed.jwt.token"), &auth_v1.AuthServiceLogoutRequest{}); err != nil {
		t.Fatalf("Logout() unexpected error = %v", err)
	}

	if got != "signed.jwt.token" {
		t.Errorf("Logout() token = %q, want %q", got, "signed.jwt.token")
	}
}

func TestAuthServiceServer_Logout_NoToken(t *testing.T) {
	auth := &mockAuthService{logoutFunc: func(context.Context, string) error {
		t.Error("Logout() дошёл до сервиса без токена")

		return nil
	}}

	server := NewAuthServiceServer(auth)

	_, err := server.Logout(context.Background(), &auth_v1.AuthServiceLogoutRequest{})
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("Logout() code = %v, want %v (err = %v)", status.Code(err), codes.Unauthenticated, err)
	}
}

func TestAuthServiceServer_GetCurrentUser(t *testing.T) {
	user := handlerTestUser(t)
	server := NewAuthServiceServer(&mockAuthService{})

	ctx := service.WithPrincipal(context.Background(), testPrincipal(t, user, model.RoleClient))

	resp, err := server.GetCurrentUser(ctx, &auth_v1.AuthServiceGetCurrentUserRequest{})
	if err != nil {
		t.Fatalf("GetCurrentUser() unexpected error = %v", err)
	}

	if resp.GetUser().GetUuid() != user.ID || resp.GetUser().GetEmail() != user.Email {
		t.Errorf("GetCurrentUser() user = %+v, want uuid %q", resp.GetUser(), user.ID)
	}
}

func TestAuthServiceServer_GetCurrentUser_NoPrincipal(t *testing.T) {
	server := NewAuthServiceServer(&mockAuthService{})

	_, err := server.GetCurrentUser(context.Background(), &auth_v1.AuthServiceGetCurrentUserRequest{})
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("GetCurrentUser() code = %v, want %v (err = %v)", status.Code(err), codes.Unauthenticated, err)
	}
}

func TestSessionToProto(t *testing.T) {
	if got := SessionToProto(nil); got != nil {
		t.Errorf("SessionToProto(nil) = %v, want nil", got)
	}

	if got := SessionToProto(&service.Session{}); got != nil {
		t.Errorf("SessionToProto(без токена) = %v, want nil", got)
	}

	session := authTestSession(t)

	got := SessionToProto(session)
	if got.GetToken() != session.AccessToken {
		t.Errorf("SessionToProto() token = %q, want %q", got.GetToken(), session.AccessToken)
	}

	if !got.GetExpiresAt().AsTime().Equal(session.Token.ExpiresAt) {
		t.Errorf("SessionToProto() expires_at = %v, want %v", got.GetExpiresAt().AsTime(), session.Token.ExpiresAt)
	}
}

func TestRegisterInputFromProto(t *testing.T) {
	if _, err := RegisterInputFromProto(nil); !errors.Is(err, errNilRequest) {
		t.Errorf("RegisterInputFromProto(nil) error = %v, want %v", err, errNilRequest)
	}

	in, err := RegisterInputFromProto(validRegisterRequest())
	if err != nil {
		t.Fatalf("RegisterInputFromProto() unexpected error = %v", err)
	}

	if in.Email != "ivan@example.com" || in.FirstName != "Иван" || in.LastName != "Иванов" {
		t.Errorf("RegisterInputFromProto() = %+v", in)
	}

	if in.Password != "secret-password" || in.Gender != model.GenderMale {
		t.Errorf("RegisterInputFromProto() = %+v", in)
	}
}

// TestAuthServiceServer_Register_ServiceErrors проверяет, что ошибки доменного
// сервиса доезжают до клиента правильными кодами gRPC.
func TestAuthServiceServer_Register_ServiceErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode codes.Code
	}{
		{name: "конфликт", err: database.ErrDuplicate, wantCode: codes.AlreadyExists},
		{name: "валидация", err: model.ErrInvalidPassword, wantCode: codes.InvalidArgument},
		{name: "внутренняя", err: errors.New("boom"), wantCode: codes.Internal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := &mockAuthService{registerFunc: func(context.Context, model.RegisterInput) (*model.User, *service.Session, error) {
				return nil, nil, tt.err
			}}

			server := NewAuthServiceServer(auth)

			_, err := server.Register(context.Background(), validRegisterRequest())
			if status.Code(err) != tt.wantCode {
				t.Errorf("Register() code = %v, want %v (err = %v)", status.Code(err), tt.wantCode, err)
			}
		})
	}
}

// TestIsPublicGRPCMethod проверяет, какие вызовы обслуживаются без токена:
// служебные (health, reflection) и открытые методы аутентификации. Выход и
// чтение профиля защищены, как и методы сервиса пользователей.
func TestIsPublicGRPCMethod(t *testing.T) {
	tests := []struct {
		name   string
		method string
		want   bool
	}{
		{name: "health", method: "/grpc.health.v1.Health/Check", want: true},
		{name: "reflection", method: "/grpc.reflection.v1.ServerReflection/ServerReflectionInfo", want: true},
		{name: "register", method: "/" + auth_v1.AuthService_ServiceDesc.ServiceName + "/Register", want: true},
		{name: "login", method: "/" + auth_v1.AuthService_ServiceDesc.ServiceName + "/Login", want: true},
		{
			name:   "change password",
			method: "/" + auth_v1.AuthService_ServiceDesc.ServiceName + "/ChangePassword",
			want:   true,
		},
		{name: "logout", method: "/" + auth_v1.AuthService_ServiceDesc.ServiceName + "/Logout", want: false},
		{name: "get current user", method: "/" + auth_v1.AuthService_ServiceDesc.ServiceName + "/GetCurrentUser", want: false},
		{name: "user create", method: "/user.v1.UserService/Create", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPublicGRPCMethod(tt.method); got != tt.want {
				t.Errorf("isPublicGRPCMethod(%q) = %v, want %v", tt.method, got, tt.want)
			}
		})
	}
}

// TestNewAuthInterceptor_AuthMethods проверяет, что интерцептор пропускает
// открытые методы аутентификации без токена и требует токен для защищённых.
func TestNewAuthInterceptor_AuthMethods(t *testing.T) {
	auth := &mockAuthService{authenticateFunc: func(_ context.Context, token string) (*service.Principal, error) {
		if token == "good-token" {
			return testPrincipal(t, handlerTestUser(t), model.RoleClient), nil
		}

		return nil, service.ErrUnauthorized
	}}

	interceptor := NewAuthInterceptor(auth)

	var called bool

	handler := func(context.Context, any) (any, error) {
		called = true

		return "ok", nil
	}

	t.Run("Register без токена проходит", func(t *testing.T) {
		called = false

		_, err := interceptor(
			context.Background(),
			nil,
			&grpc.UnaryServerInfo{FullMethod: "/" + auth_v1.AuthService_ServiceDesc.ServiceName + "/Register"},
			handler,
		)
		if err != nil || !called {
			t.Errorf("Register без токена: err = %v, called = %v", err, called)
		}
	})

	t.Run("GetCurrentUser без токена отклоняется", func(t *testing.T) {
		called = false

		_, err := interceptor(
			context.Background(),
			nil,
			&grpc.UnaryServerInfo{FullMethod: "/" + auth_v1.AuthService_ServiceDesc.ServiceName + "/GetCurrentUser"},
			handler,
		)
		if status.Code(err) != codes.Unauthenticated || called {
			t.Errorf("GetCurrentUser без токена: code = %v, called = %v", status.Code(err), called)
		}
	})

	t.Run("GetCurrentUser с токеном проходит", func(t *testing.T) {
		called = false

		_, err := interceptor(
			metadataContext("good-token"),
			nil,
			&grpc.UnaryServerInfo{FullMethod: "/" + auth_v1.AuthService_ServiceDesc.ServiceName + "/GetCurrentUser"},
			handler,
		)
		if err != nil || !called {
			t.Errorf("GetCurrentUser с токеном: err = %v, called = %v", err, called)
		}
	})
}
