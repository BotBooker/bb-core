package servers

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/model"
	"github.com/botbooker/bb-core/internal/service"
	user_v1 "github.com/botbooker/bb-core/pkg/proto/user/v1"
)

// UserServiceAPI — то, что адаптеру нужно от сервиса пользователей.
// Объявлен узко: адаптеру не нужны ни события сервиса, ни регистрация.
type UserServiceAPI interface {
	Create(ctx context.Context, actor *service.Principal, in model.CreateUserInput) (*model.User, error)
	Get(ctx context.Context, actor *service.Principal, id string) (*model.User, error)
	Update(
		ctx context.Context,
		actor *service.Principal,
		id string,
		in model.UpdateUserInput,
	) (*model.User, error)
	Delete(ctx context.Context, actor *service.Principal, id string) error
	AssignGlobalOperator(ctx context.Context, actor *service.Principal, id string) (*model.User, error)
	RevokeGlobalOperator(ctx context.Context, actor *service.Principal, id string) error
}

// actor возвращает действующего пользователя из контекста вызова.
//
// Контекст заполняет транспорт: middleware HTTP или интерцептор gRPC. Если
// пользователя нет, вызов пришёл в обход проверки токена: сервис вернул бы 401
// и сам, но отвечать до обращения к нему и правильнее, и дешевле.
func actor(ctx context.Context) (*service.Principal, error) {
	principal, ok := service.PrincipalFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, unauthorizedMessage)
	}

	return principal, nil
}

// userServiceServer — адаптер proto-сервиса к доменному сервису:
// протокол (валидация сообщений, коды ответов) остаётся здесь, бизнес-логика —
// в сервисе, а разбор полей — в конвертерах user_mapping.go.
type userServiceServer struct {
	user_v1.UnimplementedUserServiceServer

	users UserServiceAPI
}

// Проверяем на этапе компиляции, что адаптер реализует proto-сервис целиком.
var _ user_v1.UserServiceServer = (*userServiceServer)(nil)

// NewUserServiceServer создаёт реализацию UserServiceServer поверх сервиса.
//
// Один и тот же адаптер регистрируется и в gRPC-сервере, и в grpc-gateway:
// HTTP и gRPC обслуживает один код, поэтому ответы и коды ошибок совпадают.
func NewUserServiceServer(users UserServiceAPI) user_v1.UserServiceServer {
	return &userServiceServer{users: users}
}

// Create создаёт пользователя.
func (s *userServiceServer) Create(
	ctx context.Context,
	req *user_v1.UserServiceCreateRequest,
) (*user_v1.UserServiceCreateResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	in, err := CreateInputFromProto(req)
	if err != nil {
		return nil, toStatusError(ctx, err)
	}

	current, err := actor(ctx)
	if err != nil {
		return nil, err
	}

	user, err := s.users.Create(ctx, current, in)
	if err != nil {
		return nil, toStatusError(ctx, err)
	}

	return &user_v1.UserServiceCreateResponse{User: UserToProto(user)}, nil
}

// Get возвращает пользователя по UUID.
func (s *userServiceServer) Get(
	ctx context.Context,
	req *user_v1.UserServiceGetRequest,
) (*user_v1.UserServiceGetResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	current, err := actor(ctx)
	if err != nil {
		return nil, err
	}

	user, err := s.users.Get(ctx, current, req.GetUuid())
	if err != nil {
		return nil, toStatusError(ctx, err)
	}

	return &user_v1.UserServiceGetResponse{User: UserToProto(user)}, nil
}

// Update обновляет данные пользователя.
func (s *userServiceServer) Update(
	ctx context.Context,
	req *user_v1.UserServiceUpdateRequest,
) (*user_v1.UserServiceUpdateResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	in, err := UpdateInputFromProto(req)
	if err != nil {
		return nil, toStatusError(ctx, err)
	}

	current, err := actor(ctx)
	if err != nil {
		return nil, err
	}

	user, err := s.users.Update(ctx, current, req.GetUuid(), in)
	if err != nil {
		return nil, toStatusError(ctx, err)
	}

	return &user_v1.UserServiceUpdateResponse{User: UserToProto(user)}, nil
}

// Delete удаляет пользователя.
func (s *userServiceServer) Delete(
	ctx context.Context,
	req *user_v1.UserServiceDeleteRequest,
) (*emptypb.Empty, error) {
	if err := req.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	current, err := actor(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.users.Delete(ctx, current, req.GetUuid()); err != nil {
		return nil, toStatusError(ctx, err)
	}

	return &emptypb.Empty{}, nil
}

// AssignGlobalOperator назначает пользователя глобальным оператором.
//
// Право назначать проверяет сервис: суперпользователь — системная роль, и
// транспорт не должен дублировать это правило. Глобальный оператор получает
// права администратора во всех тенантах.
func (s *userServiceServer) AssignGlobalOperator(
	ctx context.Context,
	req *user_v1.UserServiceAssignGlobalOperatorRequest,
) (*user_v1.UserServiceAssignGlobalOperatorResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, errNilRequest.Error())
	}

	if err := req.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	current, err := actor(ctx)
	if err != nil {
		return nil, err
	}

	user, err := s.users.AssignGlobalOperator(ctx, current, req.GetUuid())
	if err != nil {
		return nil, toStatusError(ctx, err)
	}

	return &user_v1.UserServiceAssignGlobalOperatorResponse{User: UserToProto(user)}, nil
}

// RevokeGlobalOperator снимает права глобального оператора.
func (s *userServiceServer) RevokeGlobalOperator(
	ctx context.Context,
	req *user_v1.UserServiceRevokeGlobalOperatorRequest,
) (*emptypb.Empty, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, errNilRequest.Error())
	}

	if err := req.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	current, err := actor(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.users.RevokeGlobalOperator(ctx, current, req.GetUuid()); err != nil {
		return nil, toStatusError(ctx, err)
	}

	return &emptypb.Empty{}, nil
}

// invalidArgumentErrors — ошибки слоёв ниже, которые означают «клиент прислал
// неверные данные»: gateway отдаёт на них 400, а не 500.
var invalidArgumentErrors = []error{
	errNilRequest,
	model.ErrInvalidUUID,
	model.ErrInvalidEmail,
	model.ErrNameRequired,
	model.ErrInvalidMiddleName,
	model.ErrInvalidPhone,
	model.ErrInvalidGender,
	model.ErrInvalidRole,
	model.ErrInvalidBirthDate,
	model.ErrInvalidPassword,
	model.ErrInvalidPageToken,
	model.ErrInvalidTenant,
	model.ErrInvalidTenantMember,
}

// toStatusError переводит доменные ошибки в коды gRPC.
//
// grpc-gateway превращает коды в HTTP-статусы, поэтому наружу уходят понятные
// 400/401/403/404/409. Текст внутренних ошибок не отдаётся клиенту: он только
// логируется, чтобы наружу не утекли детали SQL. Для 401 и 403 текст вообще
// фиксированный: по нему нельзя отличить «пользователя нет» от «пароль не тот».
//
// ctx нужен для логирования внутренней ошибки: trace_id активной трассы
// добавляет обработчик логов (см. pkg/logger.TraceHandler).
func toStatusError(ctx context.Context, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, database.ErrNotFound):
		// Текст фиксированный: в err.Error() обёрнута ошибка драйвера
		// (sql.ErrNoRows и т.п.), а детали SQL наружу отдавать нельзя.
		return status.Error(codes.NotFound, notFoundMessage)
	case errors.Is(err, database.ErrDuplicate):
		// Здесь драйвер сообщает имя индекса и SQLSTATE, поэтому текст тоже
		// фиксированный — как у ErrNotFound.
		return status.Error(codes.AlreadyExists, conflictMessage)
	case errors.Is(err, service.ErrPasswordExpired):
		return status.Error(codes.PermissionDenied, passwordExpiredMessage)
	case errors.Is(err, service.ErrUnauthorized), errors.Is(err, service.ErrInvalidCredentials):
		return status.Error(codes.Unauthenticated, unauthorizedMessage)
	case errors.Is(err, service.ErrForbidden):
		return status.Error(codes.PermissionDenied, forbiddenMessage)
	case isInvalidArgument(err):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		slog.ErrorContext(ctx, "service call failed", "err", err)

		return status.Error(codes.Internal, internalErrorMessage)
	}
}

// isInvalidArgument сообщает, что ошибка — ошибка валидации данных клиента.
func isInvalidArgument(err error) bool {
	for _, target := range invalidArgumentErrors {
		if errors.Is(err, target) {
			return true
		}
	}

	return false
}
