package servers

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	date "google.golang.org/genproto/googleapis/type/date"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	health_v1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/botbooker/bb-core/internal/config"
	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/database/sqlite"
	"github.com/botbooker/bb-core/internal/events"
	"github.com/botbooker/bb-core/internal/model"
	"github.com/botbooker/bb-core/internal/repository"
	"github.com/botbooker/bb-core/internal/service"
	user_v1 "github.com/botbooker/bb-core/pkg/proto/user/v1"
)

// migrationsDir — каталог с SQL-миграциями относительно пакета: go test
// запускает тесты с рабочим каталогом пакета.
const migrationsDir = "../../migrations"

// smokeApp — приложение, собранное целиком, как в cmd/api: SQLite во временном
// каталоге, репозиторий, сервис, адаптер, gRPC-сервер и HTTP-gateway.
//
// Собирается теми же конструкторами, что и рабочее приложение (newGRPCServer,
// NewHandler, NewUserServiceServer), поэтому проверки идут по реальным портам
// и по настоящему протоколу: gRPC — по HTTP/2, HTTP — через gateway.
type smokeApp struct {
	grpcConn *grpc.ClientConn
	httpURL  string
}

// smokeToken — токен, под которым смок-тест ходит в защищённые ручки.
//
// Сама авторизация здесь мокается: сервис аутентификации проверяется своими
// тестами (internal/service, internal/jwt), а смок-тесту важно, что middleware
// HTTP и интерцептор gRPC требуют токен и передают Principal дальше по цепочке.
const smokeToken = "smoke-access-token"

// newSmokeAuthService собирает мок сервиса авторизации: любой непустой токен
// считается действующим, а его владелец — администратором.
//
// Роль важна: ручки пользователей теперь защищены правами. Создавать
// пользователей может только сотрудник, а удалять — только администратор,
// поэтому мок отдаёт Principal с ролью admin: он проходит все проверки прав,
// и смок-тест проверяет транспорт, а не разграничение доступа.
func newSmokeAuthService(t *testing.T) AuthService {
	t.Helper()

	admin, err := model.NewUser(model.CreateUserInput{
		Email:       "admin@example.com",
		FirstName:   "Анна",
		LastName:    "Админова",
		PhoneNumber: "+79990000000",
	}, time.Now())
	if err != nil {
		t.Fatalf("model.NewUser() unexpected error = %v", err)
	}

	// Смок-администратор — суперпользователь: он управляет всеми тенантами и
	// может назначать глобальных операторов. Флаг — системный уровень доступа,
	// он хранится в профиле, а не в членстве.
	admin.IsSuperuser = true

	// Роль в профиле не хранится: администратором пользователь становится
	// через членство в тенанте (tenant_members.role_id), поэтому роль
	// задаётся в Principal — так же, как это делает service.Authenticate.
	return &mockAuthService{authenticateFunc: func(_ context.Context, token string) (*service.Principal, error) {
		if token == "" {
			return nil, service.ErrUnauthorized
		}

		return testPrincipal(t, admin, model.RoleAdmin), nil
	}}
}

// withSmokeToken кладёт токен в метаданные каждого gRPC-вызова: интерцептор
// сервера требует его так же, как middleware в HTTP. Без этого защищённые
// методы отвечали бы Unauthenticated.
func withSmokeToken(
	ctx context.Context,
	method string,
	req, reply any,
	cc *grpc.ClientConn,
	invoker grpc.UnaryInvoker,
	opts ...grpc.CallOption,
) error {
	ctx = metadata.AppendToOutgoingContext(ctx, strings.ToLower(AuthorizationHeader), bearerPrefix+smokeToken)

	return invoker(ctx, method, req, reply, cc, opts...)
}

// newSmokeApp поднимает приложение на свободных портах и закрывает всё за тестом.
func newSmokeApp(t *testing.T) *smokeApp {
	t.Helper()

	ctx := context.Background()

	conn, err := sqlite.New(ctx, config.DatabaseConfig{
		DSN:          filepath.Join(t.TempDir(), "smoke.db"),
		MaxDBConnect: 1,
	})
	if err != nil {
		t.Fatalf("sqlite.New() unexpected error = %v", err)
	}

	t.Cleanup(func() {
		if closeErr := conn.Close(); closeErr != nil {
			t.Errorf("Close() unexpected error = %v", closeErr)
		}
	})

	if err := database.Migrate(ctx, conn, database.DialectSQLite, migrationsDir); err != nil {
		t.Fatalf("database.Migrate() unexpected error = %v", err)
	}

	userRepo := repository.NewUserRepo(ctx, conn, database.DialectSQLite)
	tenantRepo := repository.NewTenantRepo(ctx, conn, database.DialectSQLite)
	tokenRepo := repository.NewTokenRepo(ctx, conn, database.DialectSQLite)
	catalogRepo := repository.NewCatalogRepo(ctx, conn)
	eventBus := events.NewEventBus(ctx)

	// Тенант сессии: UserService.Create добавляет нового пользователя в тенант
	// действующего сотрудника, поэтому строка тенанта должна существовать.
	smokeTenant, err := model.NewTenant("Smoke Tenant", "smoke", time.Now())
	if err != nil {
		t.Fatalf("model.NewTenant() unexpected error = %v", err)
	}

	smokeTenant.ID = testTenantID
	if err := tenantRepo.Create(ctx, smokeTenant); err != nil {
		t.Fatalf("tenantRepo.Create() unexpected error = %v", err)
	}

	// Авторизация мокается (см. newSmokeAuthService), но транспорт, сервисы
	// пользователей и тенантов и репозитории — настоящие.
	authService := newSmokeAuthService(t)
	userService := service.NewUserService(ctx, userRepo, tenantRepo, eventBus)
	tenantService := service.NewTenantService(ctx, tenantRepo, userRepo, tokenRepo, eventBus)
	catalogService := service.NewPublicCatalogService(ctx, tenantRepo, catalogRepo)
	userServiceServer := NewUserServiceServer(userService)
	tenantServiceServer := NewTenantServiceServer(tenantService)

	// gRPC: тот же сервер, что создаёт initGRPCServer, — с health и reflection.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() unexpected error = %v", err)
	}

	grpcServer := newGRPCServer(userServiceServer, tenantServiceServer, NewPublicCatalogServer(catalogService), authService)

	go func() {
		if serveErr := grpcServer.Serve(listener); serveErr != nil && serveErr != grpc.ErrServerStopped {
			t.Errorf("grpc Serve() unexpected error = %v", serveErr)
		}
	}()

	t.Cleanup(grpcServer.Stop)

	// HTTP: те же ручки, что отдаёт initHTTPServer.
	handler, err := NewHandler(authService, userServiceServer, tenantServiceServer, catalogService)
	if err != nil {
		t.Fatalf("NewHandler() unexpected error = %v", err)
	}

	httpServer := httptest.NewServer(handler.Routes())
	t.Cleanup(httpServer.Close)

	clientConn, err := grpc.NewClient(
		listener.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		// Каждый вызов несёт токен: сервер защищает методы интерцептором.
		grpc.WithUnaryInterceptor(withSmokeToken),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient() unexpected error = %v", err)
	}

	t.Cleanup(func() {
		if closeErr := clientConn.Close(); closeErr != nil {
			t.Errorf("grpc Close() unexpected error = %v", closeErr)
		}
	})

	return &smokeApp{grpcConn: clientConn, httpURL: httpServer.URL}
}

// smokeContext возвращает контекст с таймаутом: смок-тест не должен висеть,
// если сервер не отвечает.
func smokeContext(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	return ctx
}

// do выполняет HTTP-запрос к приложению и возвращает статус и тело ответа.
func (a *smokeApp) do(t *testing.T, method, path, body string) (int, []byte) {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}

	req, err := http.NewRequestWithContext(context.Background(), method, a.httpURL+path, reader)
	if err != nil {
		t.Fatalf("http.NewRequest() unexpected error = %v", err)
	}

	// Все ручки, кроме публичных (/health, /auth/*), требуют токен.
	req.Header.Set(AuthorizationHeader, bearerPrefix+smokeToken)

	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("HTTP %s %s unexpected error = %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() unexpected error = %v", err)
	}

	return resp.StatusCode, payload
}

// smokeUser — то, что возвращает HTTP-ручка: имена полей — как их отдаёт
// protojson (camelCase).
type smokeUser struct {
	UUID             string     `json:"uuid"`
	Email            string     `json:"email"`
	FirstName        string     `json:"firstName"`
	LastName         string     `json:"lastName"`
	BirthDate        *smokeDate `json:"birthday"`
	IsGlobalOperator bool       `json:"isGlobalOperator"`
}

// smokeDate — google.type.Date в JSON: год, месяц и день без времени.
type smokeDate struct {
	Year  int32 `json:"year"`
	Month int32 `json:"month"`
	Day   int32 `json:"day"`
}

// assertSmokeDate проверяет дату рождения в JSON-ответе HTTP-ручки.
func assertSmokeDate(t *testing.T, got *smokeDate, want smokeDate) {
	t.Helper()

	if got == nil || *got != want {
		t.Errorf("birthDate = %+v, want %+v", got, want)
	}
}

// smokeUserResponse — тело ответа ручек: proto-сообщение с вложенным user.
type smokeUserResponse struct {
	User smokeUser `json:"user"`
}

// decodeUser разбирает ответ ручки, возвращающей пользователя.
func decodeUser(t *testing.T, payload []byte) smokeUser {
	t.Helper()

	var response smokeUserResponse
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatalf("json.Unmarshal() unexpected error = %v (body %s)", err, payload)
	}

	return response.User
}

// assertBirthDate проверяет дату рождения (google.type.Date) в ответе ручки.
func assertBirthDate(t *testing.T, got *date.Date, wantYear, wantMonth, wantDay int32) {
	t.Helper()

	if got.GetYear() != wantYear || got.GetMonth() != wantMonth || got.GetDay() != wantDay {
		t.Errorf("BirthDate = %v, want %04d-%02d-%02d", got, wantYear, wantMonth, wantDay)
	}
}

// createUser создаёт пользователя по HTTP и возвращает разобранный ответ.
func (a *smokeApp) createUser(t *testing.T, body string) smokeUser {
	t.Helper()

	status, payload := a.do(t, http.MethodPost, "/api/v1/users", body)
	if status != http.StatusOK {
		t.Fatalf("POST /api/v1/users = %d (%s), want %d", status, payload, http.StatusOK)
	}

	return decodeUser(t, payload)
}

// TestSmokeGRPC проверяет gRPC-транспорт по сети: health-сервис, полный цикл
// работы с пользователем и коды ошибок.
func TestSmokeGRPC(t *testing.T) {
	app := newSmokeApp(t)
	ctx := smokeContext(t)

	client := user_v1.NewUserServiceClient(app.grpcConn)

	var uuid string

	t.Run("health отвечает SERVING", func(t *testing.T) {
		healthClient := health_v1.NewHealthClient(app.grpcConn)

		for _, service := range []string{"", user_v1.UserService_ServiceDesc.ServiceName} {
			resp, err := healthClient.Check(ctx, &health_v1.HealthCheckRequest{Service: service})
			if err != nil {
				t.Fatalf("Health.Check(%q) unexpected error = %v", service, err)
			}

			if resp.GetStatus() != health_v1.HealthCheckResponse_SERVING {
				t.Errorf("Health.Check(%q) = %v, want SERVING", service, resp.GetStatus())
			}
		}
	})

	t.Run("Create", func(t *testing.T) {
		resp, err := client.Create(ctx, &user_v1.UserServiceCreateRequest{
			Email:       "grpc@example.com",
			FirstName:   "Иван",
			LastName:    "Иванов",
			PhoneNumber: "+79990000010",
			Gender:      user_v1.Gender_GENDER_MALE,
			Birthday:    &date.Date{Year: 1990, Month: 5, Day: 17},
		})
		if err != nil {
			t.Fatalf("Create() unexpected error = %v", err)
		}

		user := resp.GetUser()

		if err := model.ValidateUUID(user.GetUuid()); err != nil {
			t.Errorf("Create() вернул uuid = %q: %v", user.GetUuid(), err)
		}
		if user.GetEmail() != "grpc@example.com" {
			t.Errorf("Email = %q, want %q", user.GetEmail(), "grpc@example.com")
		}
		if user.GetCreatedAt() == nil || user.GetUpdatedAt() == nil {
			t.Error("Create() вернул пользователя без created_at/updated_at")
		}

		assertBirthDate(t, user.GetBirthday(), 1990, 5, 17)

		uuid = user.GetUuid()
	})

	t.Run("Create того же email — AlreadyExists", func(t *testing.T) {
		_, err := client.Create(ctx, &user_v1.UserServiceCreateRequest{
			Email:     "grpc@example.com",
			FirstName: "Пётр",
			LastName:  "Петров",
		})

		if status.Code(err) != codes.AlreadyExists {
			t.Errorf("Create() code = %v, want %v (err = %v)", status.Code(err), codes.AlreadyExists, err)
		}
	})

	t.Run("Create с невалидным email — InvalidArgument", func(t *testing.T) {
		_, err := client.Create(ctx, &user_v1.UserServiceCreateRequest{
			Email:     "не-email",
			FirstName: "Иван",
			LastName:  "Иванов",
		})

		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("Create() code = %v, want %v (err = %v)", status.Code(err), codes.InvalidArgument, err)
		}
	})

	t.Run("Get", func(t *testing.T) {
		resp, err := client.Get(ctx, &user_v1.UserServiceGetRequest{Uuid: uuid})
		if err != nil {
			t.Fatalf("Get() unexpected error = %v", err)
		}

		if resp.GetUser().GetUuid() != uuid {
			t.Errorf("uuid = %q, want %q", resp.GetUser().GetUuid(), uuid)
		}
	})

	t.Run("Get несуществующего — NotFound", func(t *testing.T) {
		_, err := client.Get(ctx, &user_v1.UserServiceGetRequest{
			Uuid: "11111111-1111-4111-8111-111111111111",
		})

		if status.Code(err) != codes.NotFound {
			t.Errorf("Get() code = %v, want %v (err = %v)", status.Code(err), codes.NotFound, err)
		}
	})

	t.Run("Get с не-UUID — InvalidArgument", func(t *testing.T) {
		_, err := client.Get(ctx, &user_v1.UserServiceGetRequest{Uuid: "not-a-uuid"})

		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("Get() code = %v, want %v (err = %v)", status.Code(err), codes.InvalidArgument, err)
		}
	})

	t.Run("Update", func(t *testing.T) {
		resp, err := client.Update(ctx, &user_v1.UserServiceUpdateRequest{
			Uuid:      uuid,
			FirstName: wrapperspb.String("Пётр"),
			Gender:    wrapperspb.Int32(int32(user_v1.Gender_GENDER_FEMALE)),
		})
		if err != nil {
			t.Fatalf("Update() unexpected error = %v", err)
		}

		user := resp.GetUser()
		if user.GetFirstName() != "Пётр" {
			t.Errorf("FirstName = %q, want %q", user.GetFirstName(), "Пётр")
		}
		if user.GetLastName() != "Иванов" {
			t.Errorf("LastName = %q, want %q (непереданное поле не должно меняться)", user.GetLastName(), "Иванов")
		}
		if user.GetGender() != user_v1.Gender_GENDER_FEMALE {
			t.Errorf("Gender = %v, want FEMALE", user.GetGender())
		}

		assertBirthDate(t, user.GetBirthday(), 1990, 5, 17)
	})

	t.Run("AssignGlobalOperator", func(t *testing.T) {
		resp, err := client.AssignGlobalOperator(ctx, &user_v1.UserServiceAssignGlobalOperatorRequest{Uuid: uuid})
		if err != nil {
			t.Fatalf("AssignGlobalOperator() unexpected error = %v", err)
		}
		if !resp.GetUser().GetIsGlobalOperator() {
			t.Error("is_global_operator = false, want true")
		}
	})

	t.Run("RevokeGlobalOperator", func(t *testing.T) {
		if _, err := client.RevokeGlobalOperator(ctx, &user_v1.UserServiceRevokeGlobalOperatorRequest{Uuid: uuid}); err != nil {
			t.Fatalf("RevokeGlobalOperator() unexpected error = %v", err)
		}

		resp, err := client.Get(ctx, &user_v1.UserServiceGetRequest{Uuid: uuid})
		if err != nil {
			t.Fatalf("Get() unexpected error = %v", err)
		}
		if resp.GetUser().GetIsGlobalOperator() {
			t.Error("is_global_operator = true после RevokeGlobalOperator, want false")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		if _, err := client.Delete(ctx, &user_v1.UserServiceDeleteRequest{Uuid: uuid}); err != nil {
			t.Fatalf("Delete() unexpected error = %v", err)
		}

		_, err := client.Get(ctx, &user_v1.UserServiceGetRequest{Uuid: uuid})
		if status.Code(err) != codes.NotFound {
			t.Errorf("Get() после Delete code = %v, want %v (err = %v)", status.Code(err), codes.NotFound, err)
		}

		_, err = client.Delete(ctx, &user_v1.UserServiceDeleteRequest{Uuid: uuid})
		if status.Code(err) != codes.NotFound {
			t.Errorf("повторный Delete() code = %v, want %v (err = %v)", status.Code(err), codes.NotFound, err)
		}
	})
}

// TestSmokeHTTP проверяет HTTP-транспорт по сети: свои ручки (/health),
// ручки proto-сервиса через grpc-gateway и перевод кодов gRPC в статусы HTTP.
func TestSmokeHTTP(t *testing.T) {
	app := newSmokeApp(t)

	t.Run("GET /health", func(t *testing.T) {
		statusCode, payload := app.do(t, http.MethodGet, "/health", "")
		if statusCode != http.StatusOK {
			t.Fatalf("GET /health = %d, want %d", statusCode, http.StatusOK)
		}

		if string(payload) != "ok\n" {
			t.Errorf("тело ответа = %q, want %q", payload, "ok\n")
		}
	})

	var uuid string

	t.Run("POST /api/v1/users", func(t *testing.T) {
		user := app.createUser(t, `{
			"email": "http@example.com",
			"first_name": "Мария",
			"last_name": "Петрова",
			"phone_number": "+79990000011",
			"gender": "GENDER_FEMALE",
			"birthday": {"year": 1990, "month": 5, "day": 17}
		}`)

		if err := model.ValidateUUID(user.UUID); err != nil {
			t.Errorf("uuid = %q: %v", user.UUID, err)
		}
		if user.Email != "http@example.com" || user.FirstName != "Мария" {
			t.Errorf("ответ = %+v", user)
		}

		assertSmokeDate(t, user.BirthDate, smokeDate{Year: 1990, Month: 5, Day: 17})

		uuid = user.UUID
	})

	t.Run("POST /api/v1/users повторно — 409", func(t *testing.T) {
		statusCode, payload := app.do(t, http.MethodPost, "/api/v1/users", `{
			"email": "http@example.com",
			"first_name": "Мария",
			"last_name": "Петрова"
		}`)

		if statusCode != http.StatusConflict {
			t.Errorf("статус = %d (%s), want %d", statusCode, payload, http.StatusConflict)
		}
	})

	t.Run("POST /api/v1/users с невалидным email — 400", func(t *testing.T) {
		statusCode, payload := app.do(t, http.MethodPost, "/api/v1/users", `{
			"email": "не-email",
			"first_name": "Мария",
			"last_name": "Петрова"
		}`)

		if statusCode != http.StatusBadRequest {
			t.Errorf("статус = %d (%s), want %d", statusCode, payload, http.StatusBadRequest)
		}
	})

	t.Run("GET /api/v1/users/{uuid}", func(t *testing.T) {
		statusCode, payload := app.do(t, http.MethodGet, "/api/v1/users/"+uuid, "")
		if statusCode != http.StatusOK {
			t.Fatalf("статус = %d (%s), want %d", statusCode, payload, http.StatusOK)
		}

		user := decodeUser(t, payload)

		if user.UUID != uuid {
			t.Errorf("uuid = %q, want %q", user.UUID, uuid)
		}
	})

	t.Run("GET /api/v1/users/{uuid} отсутствующего — 404", func(t *testing.T) {
		statusCode, payload := app.do(
			t,
			http.MethodGet,
			"/api/v1/users/11111111-1111-4111-8111-111111111111",
			"",
		)

		if statusCode != http.StatusNotFound {
			t.Errorf("статус = %d (%s), want %d", statusCode, payload, http.StatusNotFound)
		}
	})

	t.Run("GET /api/v1/users/{не-uuid} — 400", func(t *testing.T) {
		statusCode, payload := app.do(t, http.MethodGet, "/api/v1/users/not-a-uuid", "")

		if statusCode != http.StatusBadRequest {
			t.Errorf("статус = %d (%s), want %d", statusCode, payload, http.StatusBadRequest)
		}
	})

	t.Run("PATCH /api/v1/users/{uuid}", func(t *testing.T) {
		statusCode, payload := app.do(t, http.MethodPatch, "/api/v1/users/"+uuid, `{
			"first_name": "Мария-Петра",
			"gender": 1
		}`)
		if statusCode != http.StatusOK {
			t.Fatalf("статус = %d (%s), want %d", statusCode, payload, http.StatusOK)
		}

		user := decodeUser(t, payload)

		if user.FirstName != "Мария-Петра" {
			t.Errorf("firstName = %q, want %q", user.FirstName, "Мария-Петра")
		}
		if user.LastName != "Петрова" {
			t.Errorf("lastName = %q, want %q (непереданное поле не меняется)", user.LastName, "Петрова")
		}

		assertSmokeDate(t, user.BirthDate, smokeDate{Year: 1990, Month: 5, Day: 17})
	})

	t.Run("POST /api/v1/users/{uuid}/global-operator", func(t *testing.T) {
		statusCode, payload := app.do(t, http.MethodPost, "/api/v1/users/"+uuid+"/global-operator", "")
		if statusCode != http.StatusOK {
			t.Fatalf("статус = %d (%s), want %d", statusCode, payload, http.StatusOK)
		}

		if user := decodeUser(t, payload); !user.IsGlobalOperator {
			t.Error("isGlobalOperator = false, want true")
		}
	})

	t.Run("DELETE /api/v1/users/{uuid}/global-operator", func(t *testing.T) {
		statusCode, payload := app.do(t, http.MethodDelete, "/api/v1/users/"+uuid+"/global-operator", "")
		if statusCode != http.StatusOK {
			t.Fatalf("статус = %d (%s), want %d", statusCode, payload, http.StatusOK)
		}

		statusCode, payload = app.do(t, http.MethodGet, "/api/v1/users/"+uuid, "")
		if statusCode != http.StatusOK {
			t.Fatalf("GET после DELETE = %d (%s), want %d", statusCode, payload, http.StatusOK)
		}
		if user := decodeUser(t, payload); user.IsGlobalOperator {
			t.Error("isGlobalOperator = true после DELETE, want false")
		}
	})

	t.Run("DELETE /api/v1/users/{uuid}", func(t *testing.T) {
		statusCode, payload := app.do(t, http.MethodDelete, "/api/v1/users/"+uuid, "")
		if statusCode != http.StatusOK {
			t.Fatalf("статус = %d (%s), want %d", statusCode, payload, http.StatusOK)
		}

		statusCode, payload = app.do(t, http.MethodGet, "/api/v1/users/"+uuid, "")
		if statusCode != http.StatusNotFound {
			t.Errorf("GET после DELETE = %d (%s), want %d", statusCode, payload, http.StatusNotFound)
		}

		statusCode, payload = app.do(t, http.MethodDelete, "/api/v1/users/"+uuid, "")
		if statusCode != http.StatusNotFound {
			t.Errorf("повторный DELETE = %d (%s), want %d", statusCode, payload, http.StatusNotFound)
		}
	})
}
