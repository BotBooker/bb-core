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
	"github.com/botbooker/bb-core/pkg/observability"
	auth_v1 "github.com/botbooker/bb-core/pkg/proto/auth/v1"
	public_v1 "github.com/botbooker/bb-core/pkg/proto/public/v1"
	"github.com/botbooker/bb-core/pkg/tools"
)

// AuthService — то, что транспорту нужно от сервиса авторизации.
//
// Интерфейс объявлен узко: транспорту нужны только эти четыре операции, хотя
// сервис умеет больше (например, чистку просроченных сессий).
type AuthService interface {
	Register(ctx context.Context, in model.RegisterInput) (*model.User, *service.Session, error)
	Login(ctx context.Context, email, password, tenantSlug string) (*service.Session, error)
	ChangePassword(ctx context.Context, email, currentPassword, newPassword, tenantSlug string) (*service.Session, error)
	SwitchTenant(ctx context.Context, actor *service.Principal, tenantSlug string) (*service.Session, error)
	ListMyTenants(ctx context.Context, actor *service.Principal) ([]*model.TenantMembership, error)
	LogoutAll(ctx context.Context, actor *service.Principal) error
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
	// passwordExpiredMessage и passwordExpiredCode — ответ на попытку входа с
	// просроченным паролем: пользователь обязан сменить пароль, прежде чем
	// продолжать работу. Code — машиночитаемый признак, по которому клиент
	// ведёт пользователя на экран смены пароля.
	passwordExpiredMessage = "password expired"
	passwordExpiredCode    = "password_expired"
)

// errNoToken — токен не передан в запросе.
var errNoToken = errors.New("token is not provided")

// loginRequest — тело POST /auth/login.
type loginRequest struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	TenantSlug string `json:"tenant_slug"`
}

// changePasswordRequest — тело POST /auth/password: смена пароля.
//
// Email и текущий пароль подтверждают личность — так же, как при входе. Так
// пароль может сменить и тот, у кого срок его действия истёк: действующего
// токена у него нет, предъявить больше нечего.
type changePasswordRequest struct {
	Email           string `json:"email"`
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
	TenantSlug      string `json:"tenant_slug"`
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
	TenantSlug  string `json:"tenant_slug"`
}

// switchTenantRequest — тело POST /auth/tenant/switch.
type switchTenantRequest struct {
	TenantSlug string `json:"tenant_slug"`
}

// tenantMembershipResponse — членство пользователя в тенанте для GET /auth/tenants.
type tenantMembershipResponse struct {
	TenantUUID string `json:"tenant_uuid"`
	TenantSlug string `json:"tenant_slug"`
	TenantName string `json:"tenant_name"`
	Status     string `json:"status"`
	Role       string `json:"role"`
	IsOwner    bool   `json:"is_owner"`
	JoinedAt   string `json:"joined_at"`
}

// listMyTenantsResponse — ответ GET /auth/tenants.
type listMyTenantsResponse struct {
	Tenants []tenantMembershipResponse `json:"tenants"`
}

// авторизовываться дальше и до какого момента.
type sessionResponse struct {
	Token      string `json:"token"`
	ExpiresAt  string `json:"expires_at"`
	TenantUUID string `json:"tenant_uuid,omitempty"`
	TenantSlug string `json:"tenant_slug,omitempty"`
	TenantName string `json:"tenant_name,omitempty"`
	Role       string `json:"role,omitempty"`
}

// newSessionResponse собирает ответ по выданной сессии: время в RFC 3339,
// как отдаёт google.protobuf.Timestamp grpc-gateway.
func newSessionResponse(session *service.Session) sessionResponse {
	response := sessionResponse{
		Token:     session.AccessToken,
		ExpiresAt: session.Token.ExpiresAt.UTC().Format(time.RFC3339),
		Role:      session.Role.String(),
	}

	if session.Tenant != nil {
		response.TenantUUID = session.Tenant.ID
		response.TenantSlug = session.Tenant.Slug
		response.TenantName = session.Tenant.Name
	}

	return response
}

// registerHandler — POST /auth/register: самостоятельная регистрация.
//
// Созданный пользователь получает роль client (её проставляет модель) и сразу
// токен: после регистрации не нужно отдельно входить.
func (h *handler) registerHandler(w http.ResponseWriter, r *http.Request) {
	var request registerRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(r.Context(), w, http.StatusBadRequest, badRequestMessage)
		return
	}

	gender := model.GenderUnspecified

	if request.Gender != "" {
		parsed, err := model.ParseGender(request.Gender)
		if err != nil {
			writeServiceError(r.Context(), w, err, "register user")

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
		Password:   request.Password,
		TenantSlug: request.TenantSlug,
	}

	_, session, err := h.authService.Register(r.Context(), in)
	if err != nil {
		writeServiceError(r.Context(), w, err, "register user")

		return
	}

	writeJSON(r.Context(), w, http.StatusCreated, newSessionResponse(session))
}

// loginHandler — POST /auth/login: вход по email и паролю.
func (h *handler) loginHandler(w http.ResponseWriter, r *http.Request) {
	var request loginRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(r.Context(), w, http.StatusBadRequest, badRequestMessage)
		return
	}

	session, err := h.authService.Login(r.Context(), request.Email, request.Password, request.TenantSlug)
	if err != nil {
		writeServiceError(r.Context(), w, err, "login user")

		return
	}

	writeJSON(r.Context(), w, http.StatusOK, newSessionResponse(session))
}

// changePasswordHandler — POST /auth/password: смена пароля по текущему паролю.
//
// Ручка открыта (не требует токена): она и есть способ выйти из состояния
// «пароль просрочен», когда токена нет. Личность подтверждает текущий пароль.
func (h *handler) changePasswordHandler(w http.ResponseWriter, r *http.Request) {
	var request changePasswordRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(r.Context(), w, http.StatusBadRequest, badRequestMessage)
		return
	}

	session, err := h.authService.ChangePassword(
		r.Context(), request.Email, request.CurrentPassword, request.NewPassword, request.TenantSlug,
	)
	if err != nil {
		writeServiceError(r.Context(), w, err, "change password")

		return
	}

	writeJSON(r.Context(), w, http.StatusOK, newSessionResponse(session))
}

// logoutHandler — POST /auth/logout: выход из системы.
//
// Middleware уже проверила токен, поэтому здесь остаётся отозвать сессию:
// после этого тот же токен не примут ни HTTP, ни gRPC.
func (h *handler) logoutHandler(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		writeError(r.Context(), w, http.StatusUnauthorized, unauthorizedMessage)
		return
	}

	if err := h.authService.Logout(r.Context(), token); err != nil {
		writeServiceError(r.Context(), w, err, "logout user")

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// listMyTenantsHandler — GET /auth/tenants: тенанты действующего пользователя.
func (h *handler) listMyTenantsHandler(w http.ResponseWriter, r *http.Request) {
	principal, ok := service.PrincipalFromContext(r.Context())
	if !ok || principal.User == nil {
		writeError(r.Context(), w, http.StatusUnauthorized, unauthorizedMessage)

		return
	}

	memberships, err := h.authService.ListMyTenants(r.Context(), principal)
	if err != nil {
		writeServiceError(r.Context(), w, err, "list my tenants")

		return
	}

	items := make([]tenantMembershipResponse, 0, len(memberships))
	for _, membership := range memberships {
		items = append(items, tenantMembershipResponse{
			TenantUUID: membership.Tenant.ID,
			TenantSlug: membership.Tenant.Slug,
			TenantName: membership.Tenant.Name,
			Status:     membership.Tenant.Status.String(),
			Role:       membership.Member.Role.String(),
			IsOwner:    membership.Member.IsOwner,
			JoinedAt:   membership.Member.JoinedAt.UTC().Format(time.RFC3339),
		})
	}

	writeJSON(r.Context(), w, http.StatusOK, listMyTenantsResponse{Tenants: items})
}

// switchTenantHandler — POST /auth/tenant/switch: новая сессия в другом тенанте.
func (h *handler) switchTenantHandler(w http.ResponseWriter, r *http.Request) {
	principal, ok := service.PrincipalFromContext(r.Context())
	if !ok || principal.User == nil {
		writeError(r.Context(), w, http.StatusUnauthorized, unauthorizedMessage)

		return
	}

	var request switchTenantRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(r.Context(), w, http.StatusBadRequest, badRequestMessage)

		return
	}

	session, err := h.authService.SwitchTenant(r.Context(), principal, request.TenantSlug)
	if err != nil {
		writeServiceError(r.Context(), w, err, "switch tenant")

		return
	}

	writeJSON(r.Context(), w, http.StatusOK, newSessionResponse(session))
}

// logoutAllHandler — POST /auth/logout-all: выход из всех сессий пользователя.
func (h *handler) logoutAllHandler(w http.ResponseWriter, r *http.Request) {
	principal, ok := service.PrincipalFromContext(r.Context())
	if !ok || principal.User == nil {
		writeError(r.Context(), w, http.StatusUnauthorized, unauthorizedMessage)

		return
	}

	if err := h.authService.LogoutAll(r.Context(), principal); err != nil {
		writeServiceError(r.Context(), w, err, "logout all")

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
		writeError(r.Context(), w, http.StatusUnauthorized, unauthorizedMessage)
		return
	}

	// Ответ в том же формате, что у GET /api/v1/users/{uuid}: обе ручки отдают
	// одно proto-сообщение, поэтому клиенту не нужно знать два формата.
	writeProtoJSON(r.Context(), w, http.StatusOK, UserToProto(principal.User))
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
	traceCtx, spanEnd := observability.TraceSpan(ctx, "bearerTokenFromMetadata")
	defer spanEnd()

	md, ok := metadata.FromIncomingContext(traceCtx)
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

// maxAuthorizationLength — предел длины значения заголовка Authorization.
//
// Ограничение защищает от разбора и хеширования огромного значения (DoS), но
// не искажает его: слишком длинный заголовок отвергается, а не обрезается, и
// клиент получает 401, а не загадочный отказ проверки подписи. 8 КиБ — запас
// больше чем в 20 раз к реальному JWT при дефолтных claim'ах, при этом
// совпадает с дефолтом nginx (large_client_header_buffers).
const maxAuthorizationLength = 8 << 10

// parseBearer выделяет токен из значения заголовка как есть.
//
// Санитайзер здесь намеренно не применяется: значение — учётные данные, а
// санитизация (замена переводов строк и обрезка до 500 байт) — операция уровня
// логирования; на пути проверки креденшла она испортила бы валидный токен.
// Защита от log-injection тоже не нужна: net/http и HTTP/2 отклоняют значения
// заголовков с управляющими символами, а значение нигде не логируется.
//
// Схема сравнивается без учёта регистра (RFC 6750 разрешает её любой вариант),
// а пустой токен после схемы невалиден: "Bearer" без значения — не токен.
func parseBearer(value string) (string, bool) {
	if len(value) > maxAuthorizationLength {
		return "", false
	}

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
	// Смена пароля тоже без токена: она нужна и тем, у кого пароль просрочен,
	// — личность подтверждает текущий пароль в теле запроса.
	"/auth/password": {},
	// Те же ручки, что отдаёт auth.v1.AuthService через grpc-gateway, под
	// префиксом /api/v1. Список открытых путей общий для обоих транспортов:
	// без этих строк вход и регистрация в HTTP-gateway требовали бы токен,
	// которого у клиента ещё нет.
	"/api/v1/auth/login":    {},
	"/api/v1/auth/register": {},
	"/api/v1/auth/password": {},
}

// isPublicPath сообщает, что запрос можно обрабатывать без токена.
//
// Кроме ручек из publicPaths открыт Swagger UI: он отдаёт статику и спецификацию,
// данных пользователей в нём нет, а без него документацию не открыть.
func isPublicPath(path string) bool {
	if _, ok := publicPaths[path]; ok {
		return true
	}

	// Публичный каталог открыт без токена: это витрина, а не данные участника.
	if strings.HasPrefix(path, "/api/v1/public/") || strings.HasPrefix(path, "/t/") {
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
			writeError(r.Context(), w, http.StatusUnauthorized, unauthorizedMessage)

			return
		}

		principal, err := h.authService.Authenticate(r.Context(), token)
		if err != nil {
			writeServiceError(r.Context(), w, err, "authenticate request")

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
			return nil, toStatusError(ctx, err)
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

// publicGRPCMethods — вызовы proto-сервиса аутентификации, доступные без токена.
//
// Список — зеркало publicPaths для gRPC: вход и регистрация открыты, потому что
// токена у клиента ещё нет, а смена пароля — потому что она нужна и тем, у кого
// срок действия пароля истёк (токен им не выдаётся). Личность подтверждает
// пароль в теле запроса.
//
// Имена собираются из AuthService_ServiceDesc.ServiceName, а не пишутся
// литералами: переименование сервиса в proto не оставит здесь мёртвых строк.
var publicGRPCMethods = map[string]struct{}{
	"/" + auth_v1.AuthService_ServiceDesc.ServiceName + "/Register":       {},
	"/" + auth_v1.AuthService_ServiceDesc.ServiceName + "/Login":          {},
	"/" + auth_v1.AuthService_ServiceDesc.ServiceName + "/ChangePassword": {},
}

// isPublicGRPCMethod сообщает, что вызов можно обслуживать без токена.
//
// Без токена обслуживаются служебные методы (см. isServiceGRPCMethod): health
// нужен балансировщикам и пробам, reflection — отладке, — и открытые методы
// аутентификации (см. publicGRPCMethods). Остальные вызовы защищены.
func isPublicGRPCMethod(fullMethod string) bool {
	if isServiceGRPCMethod(fullMethod) {
		return true
	}

	if strings.HasPrefix(fullMethod, "/"+public_v1.PublicCatalogService_ServiceDesc.ServiceName+"/") {
		return true
	}

	_, ok := publicGRPCMethods[fullMethod]

	return ok
}

// errorResponse — тело ответа с ошибкой.
type errorResponse struct {
	Error string `json:"error"`
	// Code — машиночитаемый признак для случаев, когда клиенту нужно
	// отреагировать иначе: password_expired значит «смените пароль». У
	// большинства ошибок признак пуст.
	Code string `json:"code,omitempty"`
}

// writeError отправляет ошибку в JSON.
//
// ctx нужен для логирования: trace_id активной трассы добавляет обработчик
// логов (см. pkg/logger.TraceHandler), поэтому записи делаются контекстными
// вызовами slog.
func writeError(ctx context.Context, w http.ResponseWriter, status int, message string) {
	writeJSON(ctx, w, status, errorResponse{Error: message})
}

// writeErrorCode отправляет ошибку в JSON с машиночитаемым признаком.
func writeErrorCode(ctx context.Context, w http.ResponseWriter, status int, message, code string) {
	writeJSON(ctx, w, status, errorResponse{Error: message, Code: code})
}

// writeJSON отправляет ответ в JSON.
//
// Content-Type ставится всегда: клиент не должен угадывать формат по телу, а
// ошибки кодирования логируются — ответ уже начат, и заменить его нечем.
func writeJSON(ctx context.Context, w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.ErrorContext(ctx, "failed to write json response", "err", err)
	}
}

// writeProtoJSON отправляет proto-сообщение в JSON так же, как это делает
// grpc-gateway: /users/me отвечает в том же формате, что GET /api/v1/users/{uuid}.
func writeProtoJSON(ctx context.Context, w http.ResponseWriter, status int, message proto.Message) {
	body, err := protojson.Marshal(message)
	if err != nil {
		slog.ErrorContext(ctx, "failed to marshal proto response", "err", err)
		writeError(ctx, w, http.StatusInternalServerError, internalErrorMessage)

		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if _, err := w.Write(body); err != nil {
		slog.ErrorContext(ctx, "failed to write proto response", "err", err)
	}
}

// writeServiceError переводит ошибку сервиса в HTTP-статус.
//
// Соответствие статусов то же, что у grpc-gateway в toStatusError: 401, 403,
// 404, 409, 400 и 500. Тексты для 401 и 403 фиксированные — иначе по ответу
// можно было бы отличить «пользователя нет» от «пароль не тот».
func writeServiceError(ctx context.Context, w http.ResponseWriter, err error, action string) {
	switch {
	case errors.Is(err, service.ErrPasswordExpired):
		// 403 с признаком password_expired: полного доступа нет, но клиент
		// знает, что делать — отправить пользователя на смену пароля.
		writeErrorCode(ctx, w, http.StatusForbidden, passwordExpiredMessage, passwordExpiredCode)
	case errors.Is(err, service.ErrUnauthorized), errors.Is(err, service.ErrInvalidCredentials):
		writeError(ctx, w, http.StatusUnauthorized, unauthorizedMessage)
	case errors.Is(err, service.ErrForbidden):
		writeError(ctx, w, http.StatusForbidden, forbiddenMessage)
	case errors.Is(err, database.ErrNotFound):
		writeError(ctx, w, http.StatusNotFound, notFoundMessage)
	case errors.Is(err, database.ErrDuplicate):
		writeError(ctx, w, http.StatusConflict, conflictMessage)
	case isInvalidArgument(err):
		writeError(ctx, w, http.StatusBadRequest, err.Error())
	default:
		slog.ErrorContext(ctx, action+" failed", "err", tools.SanitizeInputString(err.Error(), "error"))
		writeError(ctx, w, http.StatusInternalServerError, internalErrorMessage)
	}
}
