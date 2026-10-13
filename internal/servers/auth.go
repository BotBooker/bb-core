package servers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/model"
	"github.com/botbooker/bb-core/internal/service"
	"github.com/botbooker/bb-core/pkg/tools"
)

// AuthService — то, что транспорту нужно от сервиса авторизации.
//
// Интерфейс объявлен узко: транспорту нужны только эти четыре операции, хотя
// сервис умеет больше (например, чистку просроченных сессий).
type AuthService interface {
	Register(ctx context.Context, in model.RegisterInput) (*model.User, *service.Session, error)
	Login(ctx context.Context, email, password string) (*service.Session, error)
	Logout(ctx context.Context, accessToken string) error
	Authenticate(ctx context.Context, accessToken string) (*service.Principal, error)
}

// AuthorizationHeader — заголовок HTTP и ключ метаданных gRPC, в котором
// приходит токен. Схема — Bearer, как в proto (BearerAuth) и спецификации.
const AuthorizationHeader = "Authorization"

// bearerPrefix — схема авторизации в заголовке.
const bearerPrefix = "Bearer "

// Тексты ошибок для клиента. Внутренние детали (что именно не подошло с
// токеном, какая таблица не ответила) наружу не отдаются: они только логируются.
const (
	unauthorizedMessage  = "unauthorized"
	forbiddenMessage     = "forbidden"
	notFoundMessage      = "not found"
	conflictMessage      = "already exists"
	badRequestMessage    = "invalid request"
	internalErrorMessage = "internal error"
)

// errNoToken — токен не передан в запросе.
var errNoToken = errors.New("token is not provided")

// loginRequest — тело POST /auth/login.
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// registerRequest — тело POST /auth/register: профиль и пароль.
type registerRequest struct {
	Email       string `json:"email"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	MiddleName  string `json:"middle_name"`
	PhoneNumber string `json:"phone_number"`
	Gender      string `json:"gender"`
	Password    string `json:"password"`
}

// sessionResponse — ответ на вход и регистрацию: чем клиент будет
// авторизовываться дальше и до какого момента.
type sessionResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

// newSessionResponse собирает ответ по выданной сессии: время в RFC 3339,
// как отдаёт google.protobuf.Timestamp grpc-gateway.
func newSessionResponse(session *service.Session) sessionResponse {
	return sessionResponse{
		Token:     session.AccessToken,
		ExpiresAt: session.Token.ExpiresAt.UTC().Format(time.RFC3339),
	}
}

// registerHandler — POST /auth/register: самостоятельная регистрация.
//
// Созданный пользователь получает роль client (её проставляет модель) и сразу
// токен: после регистрации не нужно отдельно входить.
func (h *handler) registerHandler(w http.ResponseWriter, r *http.Request) {
	var request registerRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, badRequestMessage)
		return
	}

	gender := model.GenderUnspecified

	if request.Gender != "" {
		parsed, err := model.ParseGender(request.Gender)
		if err != nil {
			writeServiceError(w, err, "register user")

			return
		}

		gender = parsed
	}

	in := model.RegisterInput{
		CreateUserInput: model.CreateUserInput{
			Email:       request.Email,
			FirstName:   request.FirstName,
			LastName:    request.LastName,
			MiddleName:  request.MiddleName,
			PhoneNumber: request.PhoneNumber,
			Gender:      gender,
		},
		Password: request.Password,
	}

	_, session, err := h.authService.Register(r.Context(), in)
	if err != nil {
		writeServiceError(w, err, "register user")

		return
	}

	writeJSON(w, http.StatusCreated, newSessionResponse(session))
}

// loginHandler — POST /auth/login: вход по email и паролю.
func (h *handler) loginHandler(w http.ResponseWriter, r *http.Request) {
	var request loginRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, badRequestMessage)
		return
	}

	session, err := h.authService.Login(r.Context(), request.Email, request.Password)
	if err != nil {
		writeServiceError(w, err, "login user")

		return
	}

	writeJSON(w, http.StatusOK, newSessionResponse(session))
}

// logoutHandler — POST /auth/logout: выход из системы.
//
// Middleware уже проверила токен, поэтому здесь остаётся отозвать сессию:
// после этого тот же токен не примут ни HTTP, ни gRPC.
func (h *handler) logoutHandler(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, unauthorizedMessage)
		return
	}

	if err := h.authService.Logout(r.Context(), token); err != nil {
		writeServiceError(w, err, "logout user")

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// getCurrentUser — GET /users/me: профиль действующего пользователя.
//
// Профиль берётся из Principal: его положила middleware, а она получила его от
// сервиса авторизации, который уже прочитал пользователя из базы. Отдельный
// запрос здесь был бы вторым чтением той же строки.
func (h *handler) getCurrentUser(w http.ResponseWriter, r *http.Request) {
	principal, ok := service.PrincipalFromContext(r.Context())
	if !ok || principal.User == nil {
		writeError(w, http.StatusUnauthorized, unauthorizedMessage)
		return
	}

	// Ответ в том же формате, что у GET /api/v1/users/{uuid}: обе ручки отдают
	// одно proto-сообщение, поэтому клиенту не нужно знать два формата.
	writeProtoJSON(w, http.StatusOK, UserToProto(principal.User))
}

// bearerToken достаёт токен из заголовка Authorization HTTP-запроса.
func bearerToken(r *http.Request) (string, error) {
	token, ok := parseBearer(r.Header.Get(AuthorizationHeader))
	if !ok {
		return "", errNoToken
	}

	return token, nil
}

// bearerTokenFromMetadata достаёт токен из метаданных gRPC-вызова: в gRPC нет
// заголовков HTTP, их роль играют метаданные с тем же ключом.
func bearerTokenFromMetadata(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", errNoToken
	}

	for _, value := range md.Get(strings.ToLower(AuthorizationHeader)) {
		if token, valid := parseBearer(value); valid {
			return token, nil
		}
	}

	return "", errNoToken
}

// parseBearer выделяет токен из значения заголовка.
//
// Схема сравнивается без учёта регистра (RFC 6750 разрешает её любой вариант),
// а пустой токен после схемы невалиден: "Bearer" без значения — не токен.
func parseBearer(value string) (string, bool) {
	value = tools.SanitizeString(value)
	if len(value) <= len(bearerPrefix) || !strings.EqualFold(value[:len(bearerPrefix)], bearerPrefix) {
		return "", false
	}

	token := strings.TrimSpace(value[len(bearerPrefix):])

	return token, token != ""
}

// publicPaths — ручки, доступные без токена. Список явный: новая ручка по
// умолчанию защищена, открыть её можно только добавив сюда.
var publicPaths = map[string]struct{}{
	// health нужен балансировщикам и readiness-пробам.
	healthPath: {},
	// Вход и регистрация, разумеется, без токена: его ещё нет.
	"/auth/login":    {},
	"/auth/register": {},
}

// isPublicPath сообщает, что запрос можно обрабатывать без токена.
//
// Кроме ручек из publicPaths открыт Swagger UI: он отдаёт статику и спецификацию,
// данных пользователей в нём нет, а без него документацию не открыть.
func isPublicPath(path string) bool {
	if _, ok := publicPaths[path]; ok {
		return true
	}

	return strings.HasPrefix(path, SwaggerPrefix)
}

// authenticate — middleware HTTP: проверяет токен и кладёт действующего
// пользователя в контекст запроса.
//
// Проверку делает сервис авторизации — тот же самый, что и в gRPC-интерцепторе.
// Так правила доступа не расходятся между транспортами, а middleware остаётся
// тонким: разбор заголовка, вызов сервиса, ответ 401.
func (h *handler) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)

			return
		}

		token, err := bearerToken(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, unauthorizedMessage)

			return
		}

		principal, err := h.authService.Authenticate(r.Context(), token)
		if err != nil {
			writeServiceError(w, err, "authenticate request")

			return
		}

		next.ServeHTTP(w, r.WithContext(service.WithPrincipal(r.Context(), principal)))
	})
}

// NewAuthInterceptor создаёт унарный интерцептор gRPC: он проверяет токен и
// кладёт действующего пользователя в контекст вызова.
//
// Интерцептор нужен отдельно от HTTP-middleware: gateway вызывает адаптер
// proto-сервиса в процессе приложения, поэтому HTTP-запрос не проходит через
// интерцептор, а gRPC-вызов — через middleware. Обе проверки вызывают один
// метод сервиса, так что правила доступа совпадают.
func NewAuthInterceptor(auth AuthService) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if isPublicGRPCMethod(info.FullMethod) {
			return handler(ctx, req)
		}

		token, err := bearerTokenFromMetadata(ctx)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, unauthorizedMessage)
		}

		principal, err := auth.Authenticate(ctx, token)
		if err != nil {
			return nil, toStatusError(err)
		}

		return handler(service.WithPrincipal(ctx, principal), req)
	}
}

// Служебные gRPC-методы, которые не защищаются: health нужен балансировщикам и
// пробам, reflection — отладке (в том числе в smoke-тестах). Проверка идёт по
// префиксу: у health и reflection бывает несколько версий и методов.
const (
	healthMethodPrefix     = "/grpc.health.v1.Health/"
	reflectionMethodPrefix = "/grpc.reflection."
)

// isPublicGRPCMethod сообщает, что вызов можно обслуживать без токена.
// Без токена обслуживаются ровно служебные методы (см. isServiceGRPCMethod):
// health нужен балансировщикам и пробам, reflection — отладке.
func isPublicGRPCMethod(fullMethod string) bool {
	return isServiceGRPCMethod(fullMethod)
}

// errorResponse — тело ответа с ошибкой.
type errorResponse struct {
	Error string `json:"error"`
}

// writeError отправляет ошибку в JSON.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

// writeJSON отправляет ответ в JSON.
//
// Content-Type ставится всегда: клиент не должен угадывать формат по телу, а
// ошибки кодирования логируются — ответ уже начат, и заменить его нечем.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("failed to write json response", "err", err)
	}
}

// writeProtoJSON отправляет proto-сообщение в JSON так же, как это делает
// grpc-gateway: /users/me отвечает в том же формате, что GET /api/v1/users/{uuid}.
func writeProtoJSON(w http.ResponseWriter, status int, message proto.Message) {
	body, err := protojson.Marshal(message)
	if err != nil {
		slog.Error("failed to marshal proto response", "err", err)
		writeError(w, http.StatusInternalServerError, internalErrorMessage)

		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if _, err := w.Write(body); err != nil {
		slog.Error("failed to write proto response", "err", err)
	}
}

// writeServiceError переводит ошибку сервиса в HTTP-статус.
//
// Соответствие статусов то же, что у grpc-gateway в toStatusError: 401, 403,
// 404, 409, 400 и 500. Тексты для 401 и 403 фиксированные — иначе по ответу
// можно было бы отличить «пользователя нет» от «пароль не тот».
func writeServiceError(w http.ResponseWriter, err error, action string) {
	switch {
	case errors.Is(err, service.ErrUnauthorized), errors.Is(err, service.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, unauthorizedMessage)
	case errors.Is(err, service.ErrForbidden):
		writeError(w, http.StatusForbidden, forbiddenMessage)
	case errors.Is(err, database.ErrNotFound):
		writeError(w, http.StatusNotFound, notFoundMessage)
	case errors.Is(err, database.ErrDuplicate):
		writeError(w, http.StatusConflict, conflictMessage)
	case isInvalidArgument(err):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		slog.Error(action+" failed", "err", tools.SanitizeString(err.Error(), "error"))
		writeError(w, http.StatusInternalServerError, internalErrorMessage)
	}
}
