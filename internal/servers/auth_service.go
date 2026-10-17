package servers

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/botbooker/bb-core/internal/service"
	"github.com/botbooker/bb-core/pkg/observability"
	auth_v1 "github.com/botbooker/bb-core/pkg/proto/auth/v1"
)

// authServiceServer — адаптер proto-сервиса аутентификации к доменному сервису:
// протокол (валидация сообщений, коды ответов) остаётся здесь, бизнес-логика — в
// сервисе, а разбор полей — в конвертерах auth_mapping.go.
//
// Что именно нужно транспорту от сервиса, описывает интерфейс AuthService
// (см. auth.go): адаптер не шире его, потому что заводить отдельный интерфейс
// для тех же пяти операций незачем.
type authServiceServer struct {
	auth_v1.UnimplementedAuthServiceServer

	auth AuthService
}

// Проверяем на этапе компиляции, что адаптер реализует proto-сервис целиком.
var _ auth_v1.AuthServiceServer = (*authServiceServer)(nil)

// NewAuthServiceServer создаёт реализацию AuthServiceServer поверх сервиса.
//
// Адаптер регистрируется и в gRPC-сервере, и в grpc-gateway: HTTP и gRPC
// обслуживает один код, поэтому ответы и коды ошибок совпадают. Адаптер
// без состояния, поэтому отдельные его экземпляры на каждом транспорте
// ведут себя одинаково — общего состояния у них нет.
func NewAuthServiceServer(auth AuthService) auth_v1.AuthServiceServer {
	return &authServiceServer{auth: auth}
}

// Register создаёт пользователя с паролем и выдаёт ему токен.
func (s *authServiceServer) Register(
	ctx context.Context,
	req *auth_v1.AuthServiceRegisterRequest,
) (*auth_v1.AuthServiceRegisterResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	in, err := RegisterInputFromProto(req)
	if err != nil {
		return nil, toStatusError(ctx, err)
	}

	user, session, err := s.auth.Register(ctx, in)
	if err != nil {
		return nil, toStatusError(ctx, err)
	}

	return &auth_v1.AuthServiceRegisterResponse{
		User:    UserToProto(user),
		Session: SessionToProto(session),
	}, nil
}

// Login проверяет email и пароль и выдаёт токен.
func (s *authServiceServer) Login(
	ctx context.Context,
	req *auth_v1.AuthServiceLoginRequest,
) (*auth_v1.AuthServiceLoginResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	session, err := s.auth.Login(ctx, req.GetEmail(), req.GetPassword())
	if err != nil {
		return nil, toStatusError(ctx, err)
	}

	return &auth_v1.AuthServiceLoginResponse{Session: SessionToProto(session)}, nil
}

// ChangePassword меняет пароль и выдаёт новую сессию.
//
// Личность подтверждает текущий пароль из тела запроса, а не токен: сменить
// пароль нужно и тем, у кого срок его действия истёк, — токена у них нет.
func (s *authServiceServer) ChangePassword(
	ctx context.Context,
	req *auth_v1.AuthServiceChangePasswordRequest,
) (*auth_v1.AuthServiceChangePasswordResponse, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "ChangePassword")
	defer spanEnd()

	if err := req.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	session, err := s.auth.ChangePassword(traceCtx, req.GetEmail(), req.GetCurrentPassword(), req.GetNewPassword())
	if err != nil {
		return nil, toStatusError(traceCtx, err)
	}

	return &auth_v1.AuthServiceChangePasswordResponse{Session: SessionToProto(session)}, nil
}

// Logout отзывает токен из заголовка Authorization.
//
// Токен читается из метаданных вызова: grpc-gateway прокидывает заголовок
// Authorization в метаданные "authorization", поэтому одна и та же проверка
// работает и для gRPC, и для HTTP (см. bearerTokenFromMetadata).
func (s *authServiceServer) Logout(
	ctx context.Context,
	_ *auth_v1.AuthServiceLogoutRequest,
) (*emptypb.Empty, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "Logout")
	defer spanEnd()

	token, err := bearerTokenFromMetadata(traceCtx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, unauthorizedMessage)
	}

	if err := s.auth.Logout(traceCtx, token); err != nil {
		return nil, toStatusError(ctx, err)
	}

	return &emptypb.Empty{}, nil
}

// GetCurrentUser возвращает профиль действующего пользователя.
//
// Профиль берётся из Principal: его положила middleware HTTP или интерцептор
// gRPC, а они получили его от сервиса авторизации, который уже прочитал
// пользователя из базы. Отдельный запрос здесь был бы вторым чтением той же
// строки.
func (s *authServiceServer) GetCurrentUser(
	ctx context.Context,
	_ *auth_v1.AuthServiceGetCurrentUserRequest,
) (*auth_v1.AuthServiceGetCurrentUserResponse, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "GetCurrentUser")
	defer spanEnd()

	principal, ok := service.PrincipalFromContext(traceCtx)
	if !ok || principal.User == nil {
		return nil, status.Error(codes.Unauthenticated, unauthorizedMessage)
	}

	return &auth_v1.AuthServiceGetCurrentUserResponse{User: UserToProto(principal.User)}, nil
}
