package servers

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/model"
	"github.com/botbooker/bb-core/internal/service"
	user_v1 "github.com/botbooker/bb-core/pkg/proto/user/v1"
)

// mockUserServiceAPI — мок доменного сервиса: адаптер проверяется без базы.
// Умолчания повторяют поведение «пользователя нет»: тесты, которым важен
// только код ответа, не описывают лишних деталей.
type mockUserServiceAPI struct {
	createFunc func(ctx context.Context, actor *service.Principal, in model.CreateUserInput) (*model.User, error)
	getFunc    func(ctx context.Context, actor *service.Principal, id string) (*model.User, error)
	updateFunc func(
		ctx context.Context,
		actor *service.Principal,
		id string,
		in model.UpdateUserInput,
	) (*model.User, error)
	deleteFunc               func(ctx context.Context, actor *service.Principal, id string) error
	assignGlobalOperatorFunc func(ctx context.Context, actor *service.Principal, id string) (*model.User, error)
	revokeGlobalOperatorFunc func(ctx context.Context, actor *service.Principal, id string) error
}

func (m *mockUserServiceAPI) Create(
	ctx context.Context,
	actor *service.Principal,
	in model.CreateUserInput,
) (*model.User, error) {
	if m.createFunc != nil {
		return m.createFunc(ctx, actor, in)
	}

	return nil, nil
}

func (m *mockUserServiceAPI) Get(ctx context.Context, actor *service.Principal, id string) (*model.User, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, actor, id)
	}

	return nil, database.ErrNotFound
}

func (m *mockUserServiceAPI) Update(
	ctx context.Context,
	actor *service.Principal,
	id string,
	in model.UpdateUserInput,
) (*model.User, error) {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, actor, id, in)
	}

	return nil, database.ErrNotFound
}

func (m *mockUserServiceAPI) Delete(ctx context.Context, actor *service.Principal, id string) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, actor, id)
	}

	return database.ErrNotFound
}

func (m *mockUserServiceAPI) AssignGlobalOperator(
	ctx context.Context,
	actor *service.Principal,
	id string,
) (*model.User, error) {
	if m.assignGlobalOperatorFunc != nil {
		return m.assignGlobalOperatorFunc(ctx, actor, id)
	}

	return nil, database.ErrNotFound
}

func (m *mockUserServiceAPI) RevokeGlobalOperator(ctx context.Context, actor *service.Principal, id string) error {
	if m.revokeGlobalOperatorFunc != nil {
		return m.revokeGlobalOperatorFunc(ctx, actor, id)
	}

	return database.ErrNotFound
}

// adapterContext возвращает контекст с действующим пользователем: адаптер
// берёт его из контекста, а кладёт туда транспорт — middleware или интерцептор.
func adapterContext(t *testing.T) context.Context {
	t.Helper()

	user, err := model.NewUser(model.CreateUserInput{
		Email:     "operator@example.com",
		FirstName: "Ольга",
		LastName:  "Операторова",
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("model.NewUser() unexpected error = %v", err)
	}

	// Роль у профиля не хранится: она принадлежит членству в тенанте, поэтому
	// в Principal она задаётся явно — так же, как это делает Authenticate.
	return service.WithPrincipal(context.Background(), testPrincipal(t, user, model.RoleAdmin))
}

// adapterTestUser — пользователь, как его вернул бы доменный сервис.
func adapterTestUser(t *testing.T) *model.User {
	t.Helper()

	user, err := model.NewUser(model.CreateUserInput{
		Email:       "ivan@example.com",
		FirstName:   "Иван",
		LastName:    "Иванов",
		PhoneNumber: "+79990000001",
		Gender:      model.GenderMale,
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("model.NewUser() unexpected error = %v", err)
	}

	return user
}

// validUUID — идентификатор для запросов: его формат проверяет proto-валидация.
const validUUID = "11111111-1111-4111-8111-111111111111"

func TestNewUserServiceServer(t *testing.T) {
	if NewUserServiceServer(&mockUserServiceAPI{}) == nil {
		t.Fatal("NewUserServiceServer() returned nil")
	}
}

func TestUserServiceServer_Create(t *testing.T) {
	t.Run("возвращает созданного пользователя", func(t *testing.T) {
		want := adapterTestUser(t)

		var gotInput model.CreateUserInput

		api := &mockUserServiceAPI{createFunc: func(_ context.Context, _ *service.Principal, in model.CreateUserInput) (*model.User, error) {
			gotInput = in

			return want, nil
		}}

		resp, err := NewUserServiceServer(api).Create(adapterContext(t), &user_v1.UserServiceCreateRequest{
			Email:       "ivan@example.com",
			FirstName:   "Иван",
			LastName:    "Иванов",
			PhoneNumber: "+79990000001",
			Gender:      user_v1.Gender_GENDER_MALE,
		})
		if err != nil {
			t.Fatalf("Create() unexpected error = %v", err)
		}

		if gotInput.Email != "ivan@example.com" || gotInput.FirstName != "Иван" {
			t.Errorf("Create() передал в сервис %+v", gotInput)
		}
		if gotInput.Gender != model.GenderMale {
			t.Errorf("Gender = %q, want %q", gotInput.Gender, model.GenderMale)
		}
		if resp.GetUser().GetUuid() != want.ID {
			t.Errorf("uuid = %q, want %q", resp.GetUser().GetUuid(), want.ID)
		}
		if resp.GetUser().GetEmail() != want.Email {
			t.Errorf("email = %q, want %q", resp.GetUser().GetEmail(), want.Email)
		}
	})

	t.Run("коды ошибок", func(t *testing.T) {
		tests := []struct {
			name     string
			req      *user_v1.UserServiceCreateRequest
			err      error
			wantCode codes.Code
		}{
			{
				name:     "nil-запрос — ошибка транспорта",
				req:      nil,
				wantCode: codes.InvalidArgument,
			},
			{
				name:     "пустой email — ошибка proto-валидации",
				req:      &user_v1.UserServiceCreateRequest{FirstName: "Иван", LastName: "Иванов"},
				wantCode: codes.InvalidArgument,
			},
			{
				name: "неизвестный пол — ошибка разбора",
				req: &user_v1.UserServiceCreateRequest{
					Email:     "ivan@example.com",
					FirstName: "Иван",
					LastName:  "Иванов",
					Gender:    user_v1.Gender(42),
				},
				wantCode: codes.InvalidArgument,
			},
			{
				name: "конфликт уникальности",
				req: &user_v1.UserServiceCreateRequest{
					Email:     "ivan@example.com",
					FirstName: "Иван",
					LastName:  "Иванов",
				},
				err:      database.ErrDuplicate,
				wantCode: codes.AlreadyExists,
			},
			{
				name: "ошибка валидации модели",
				req: &user_v1.UserServiceCreateRequest{
					Email:     "ivan@example.com",
					FirstName: "Иван",
					LastName:  "Иванов",
				},
				err:      model.ErrInvalidPhone,
				wantCode: codes.InvalidArgument,
			},
			{
				name: "внутренняя ошибка",
				req: &user_v1.UserServiceCreateRequest{
					Email:     "ivan@example.com",
					FirstName: "Иван",
					LastName:  "Иванов",
				},
				err:      errors.New("database is down"),
				wantCode: codes.Internal,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				api := &mockUserServiceAPI{createFunc: func(
					context.Context,
					*service.Principal,
					model.CreateUserInput,
				) (*model.User, error) {
					return nil, tt.err
				}}

				_, err := NewUserServiceServer(api).Create(adapterContext(t), tt.req)
				if status.Code(err) != tt.wantCode {
					t.Errorf("Create() code = %v, want %v (err = %v)", status.Code(err), tt.wantCode, err)
				}
			})
		}
	})
}

func TestUserServiceServer_Get(t *testing.T) {
	t.Run("возвращает пользователя", func(t *testing.T) {
		want := adapterTestUser(t)

		api := &mockUserServiceAPI{getFunc: func(_ context.Context, _ *service.Principal, id string) (*model.User, error) {
			if id != validUUID {
				t.Errorf("сервис получил id = %q, want %q", id, validUUID)
			}

			return want, nil
		}}

		resp, err := NewUserServiceServer(api).Get(
			adapterContext(t),
			&user_v1.UserServiceGetRequest{Uuid: validUUID},
		)
		if err != nil {
			t.Fatalf("Get() unexpected error = %v", err)
		}
		if resp.GetUser().GetUuid() != want.ID {
			t.Errorf("uuid = %q, want %q", resp.GetUser().GetUuid(), want.ID)
		}
	})

	t.Run("коды ошибок", func(t *testing.T) {
		tests := []struct {
			name     string
			req      *user_v1.UserServiceGetRequest
			err      error
			wantCode codes.Code
		}{
			{
				name:     "невалидный UUID",
				req:      &user_v1.UserServiceGetRequest{Uuid: "not-a-uuid"},
				wantCode: codes.InvalidArgument,
			},
			{
				name:     "пользователь не найден",
				req:      &user_v1.UserServiceGetRequest{Uuid: validUUID},
				err:      database.ErrNotFound,
				wantCode: codes.NotFound,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				api := &mockUserServiceAPI{getFunc: func(context.Context, *service.Principal, string) (*model.User, error) {
					return nil, tt.err
				}}

				_, err := NewUserServiceServer(api).Get(adapterContext(t), tt.req)
				if status.Code(err) != tt.wantCode {
					t.Errorf("Get() code = %v, want %v (err = %v)", status.Code(err), tt.wantCode, err)
				}
			})
		}
	})
}

func TestUserServiceServer_Update(t *testing.T) {
	t.Run("передаёт только переданные поля", func(t *testing.T) {
		want := adapterTestUser(t)

		var gotInput model.UpdateUserInput

		api := &mockUserServiceAPI{updateFunc: func(
			_ context.Context,
			_ *service.Principal,
			id string,
			in model.UpdateUserInput,
		) (*model.User, error) {
			if id != validUUID {
				t.Errorf("сервис получил id = %q, want %q", id, validUUID)
			}

			gotInput = in

			return want, nil
		}}

		resp, err := NewUserServiceServer(api).Update(adapterContext(t), &user_v1.UserServiceUpdateRequest{
			Uuid:      validUUID,
			FirstName: wrapperspb.String("Пётр"),
		})
		if err != nil {
			t.Fatalf("Update() unexpected error = %v", err)
		}

		if gotInput.FirstName == nil || *gotInput.FirstName != "Пётр" {
			t.Errorf("FirstName = %v, want \"Пётр\"", gotInput.FirstName)
		}
		// Остальные поля не передавались: nil означает «не менять».
		if gotInput.LastName != nil || gotInput.PhoneNumber != nil || gotInput.Gender != nil {
			t.Errorf("Update() заполнил непереданные поля: %+v", gotInput)
		}
		if resp.GetUser().GetUuid() != want.ID {
			t.Errorf("uuid = %q, want %q", resp.GetUser().GetUuid(), want.ID)
		}
	})

	t.Run("коды ошибок", func(t *testing.T) {
		tests := []struct {
			name     string
			req      *user_v1.UserServiceUpdateRequest
			err      error
			wantCode codes.Code
		}{
			{
				name:     "невалидный UUID",
				req:      &user_v1.UserServiceUpdateRequest{Uuid: "not-a-uuid"},
				wantCode: codes.InvalidArgument,
			},
			{
				name:     "пользователь не найден",
				req:      &user_v1.UserServiceUpdateRequest{Uuid: validUUID},
				err:      database.ErrNotFound,
				wantCode: codes.NotFound,
			},
			{
				name:     "внутренняя ошибка",
				req:      &user_v1.UserServiceUpdateRequest{Uuid: validUUID},
				err:      errors.New("database is down"),
				wantCode: codes.Internal,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				api := &mockUserServiceAPI{updateFunc: func(
					context.Context,
					*service.Principal,
					string,
					model.UpdateUserInput,
				) (*model.User, error) {
					return nil, tt.err
				}}

				_, err := NewUserServiceServer(api).Update(adapterContext(t), tt.req)
				if status.Code(err) != tt.wantCode {
					t.Errorf("Update() code = %v, want %v (err = %v)", status.Code(err), tt.wantCode, err)
				}
			})
		}
	})
}

func TestUserServiceServer_Delete(t *testing.T) {
	t.Run("удаляет пользователя", func(t *testing.T) {
		called := false

		api := &mockUserServiceAPI{deleteFunc: func(_ context.Context, _ *service.Principal, id string) error {
			called = true

			if id != validUUID {
				t.Errorf("сервис получил id = %q, want %q", id, validUUID)
			}

			return nil
		}}

		resp, err := NewUserServiceServer(api).Delete(
			adapterContext(t),
			&user_v1.UserServiceDeleteRequest{Uuid: validUUID},
		)
		if err != nil {
			t.Fatalf("Delete() unexpected error = %v", err)
		}
		if resp == nil {
			t.Error("Delete() вернул nil-ответ, want google.protobuf.Empty")
		}
		if !called {
			t.Error("Delete() не вызвал сервис")
		}
	})

	t.Run("коды ошибок", func(t *testing.T) {
		tests := []struct {
			name     string
			err      error
			wantCode codes.Code
		}{
			{name: "пользователь не найден", err: database.ErrNotFound, wantCode: codes.NotFound},
			{name: "внутренняя ошибка", err: errors.New("database is down"), wantCode: codes.Internal},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				api := &mockUserServiceAPI{deleteFunc: func(context.Context, *service.Principal, string) error { return tt.err }}

				_, err := NewUserServiceServer(api).Delete(
					adapterContext(t),
					&user_v1.UserServiceDeleteRequest{Uuid: validUUID},
				)
				if status.Code(err) != tt.wantCode {
					t.Errorf("Delete() code = %v, want %v (err = %v)", status.Code(err), tt.wantCode, err)
				}
			})
		}
	})
}

func TestUserServiceServer_AssignGlobalOperator(t *testing.T) {
	t.Run("назначает глобального оператора", func(t *testing.T) {
		want := adapterTestUser(t)
		want.IsGlobalOperator = true

		var gotID string

		api := &mockUserServiceAPI{
			assignGlobalOperatorFunc: func(_ context.Context, _ *service.Principal, id string) (*model.User, error) {
				gotID = id

				return want, nil
			},
		}

		resp, err := NewUserServiceServer(api).AssignGlobalOperator(
			adapterContext(t),
			&user_v1.UserServiceAssignGlobalOperatorRequest{Uuid: validUUID},
		)
		if err != nil {
			t.Fatalf("AssignGlobalOperator() unexpected error = %v", err)
		}

		if gotID != validUUID {
			t.Errorf("сервис получил id = %q, want %q", gotID, validUUID)
		}
		if !resp.GetUser().GetIsGlobalOperator() {
			t.Error("is_global_operator = false, want true")
		}
	})

	t.Run("коды ошибок", func(t *testing.T) {
		tests := []struct {
			name     string
			req      *user_v1.UserServiceAssignGlobalOperatorRequest
			err      error
			wantCode codes.Code
		}{
			{
				name:     "nil-запрос",
				req:      nil,
				wantCode: codes.InvalidArgument,
			},
			{
				name:     "невалидный UUID",
				req:      &user_v1.UserServiceAssignGlobalOperatorRequest{Uuid: "not-a-uuid"},
				wantCode: codes.InvalidArgument,
			},
			{
				name:     "нет прав",
				req:      &user_v1.UserServiceAssignGlobalOperatorRequest{Uuid: validUUID},
				err:      service.ErrForbidden,
				wantCode: codes.PermissionDenied,
			},
			{
				name:     "пользователь не найден",
				req:      &user_v1.UserServiceAssignGlobalOperatorRequest{Uuid: validUUID},
				err:      database.ErrNotFound,
				wantCode: codes.NotFound,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				api := &mockUserServiceAPI{
					assignGlobalOperatorFunc: func(context.Context, *service.Principal, string) (*model.User, error) {
						return nil, tt.err
					},
				}

				_, err := NewUserServiceServer(api).AssignGlobalOperator(adapterContext(t), tt.req)
				if status.Code(err) != tt.wantCode {
					t.Errorf("AssignGlobalOperator() code = %v, want %v (err = %v)",
						status.Code(err), tt.wantCode, err)
				}
			})
		}
	})
}

func TestUserServiceServer_RevokeGlobalOperator(t *testing.T) {
	t.Run("снимает права глобального оператора", func(t *testing.T) {
		called := false

		api := &mockUserServiceAPI{
			revokeGlobalOperatorFunc: func(_ context.Context, _ *service.Principal, id string) error {
				called = true

				if id != validUUID {
					t.Errorf("сервис получил id = %q, want %q", id, validUUID)
				}

				return nil
			},
		}

		resp, err := NewUserServiceServer(api).RevokeGlobalOperator(
			adapterContext(t),
			&user_v1.UserServiceRevokeGlobalOperatorRequest{Uuid: validUUID},
		)
		if err != nil {
			t.Fatalf("RevokeGlobalOperator() unexpected error = %v", err)
		}
		if resp == nil {
			t.Error("RevokeGlobalOperator() вернул nil-ответ, want google.protobuf.Empty")
		}
		if !called {
			t.Error("RevokeGlobalOperator() не вызвал сервис")
		}
	})

	t.Run("коды ошибок", func(t *testing.T) {
		tests := []struct {
			name     string
			req      *user_v1.UserServiceRevokeGlobalOperatorRequest
			err      error
			wantCode codes.Code
		}{
			{
				name:     "nil-запрос",
				req:      nil,
				wantCode: codes.InvalidArgument,
			},
			{
				name:     "невалидный UUID",
				req:      &user_v1.UserServiceRevokeGlobalOperatorRequest{Uuid: "not-a-uuid"},
				wantCode: codes.InvalidArgument,
			},
			{
				name:     "нет прав",
				req:      &user_v1.UserServiceRevokeGlobalOperatorRequest{Uuid: validUUID},
				err:      service.ErrForbidden,
				wantCode: codes.PermissionDenied,
			},
			{
				name:     "пользователь не найден",
				req:      &user_v1.UserServiceRevokeGlobalOperatorRequest{Uuid: validUUID},
				err:      database.ErrNotFound,
				wantCode: codes.NotFound,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				api := &mockUserServiceAPI{
					revokeGlobalOperatorFunc: func(context.Context, *service.Principal, string) error {
						return tt.err
					},
				}

				_, err := NewUserServiceServer(api).RevokeGlobalOperator(adapterContext(t), tt.req)
				if status.Code(err) != tt.wantCode {
					t.Errorf("RevokeGlobalOperator() code = %v, want %v (err = %v)",
						status.Code(err), tt.wantCode, err)
				}
			})
		}
	})
}

func TestToStatusError(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantCode    codes.Code
		wantMessage string
	}{
		{name: "nil остаётся nil", err: nil, wantCode: codes.OK},
		{
			name:        "не найдено",
			err:         database.ErrNotFound,
			wantCode:    codes.NotFound,
			wantMessage: notFoundMessage,
		},
		{
			name:        "конфликт",
			err:         database.ErrDuplicate,
			wantCode:    codes.AlreadyExists,
			wantMessage: conflictMessage,
		},
		{name: "валидация модели", err: model.ErrInvalidEmail, wantCode: codes.InvalidArgument},
		{name: "валидация пароля", err: model.ErrInvalidPassword, wantCode: codes.InvalidArgument},
		{name: "nil-запрос", err: errNilRequest, wantCode: codes.InvalidArgument},
		{name: "нет действующего пользователя", err: service.ErrUnauthorized, wantCode: codes.Unauthenticated},
		{name: "неверные учётные данные", err: service.ErrInvalidCredentials, wantCode: codes.Unauthenticated},
		{name: "нет прав", err: service.ErrForbidden, wantCode: codes.PermissionDenied},
		{name: "пароль просрочен", err: service.ErrPasswordExpired, wantCode: codes.PermissionDenied},
		{
			name:     "обёрнутая ошибка",
			err:      errors.Join(database.ErrNotFound, errors.New("x")),
			wantCode: codes.NotFound,
		},
		{
			// Ошибка драйвера обёрнута в ErrNotFound: её текст клиенту не отдаётся.
			name:        "детали драйвера не утекают",
			err:         errors.Join(database.ErrNotFound, errors.New("sql: no rows in result set")),
			wantCode:    codes.NotFound,
			wantMessage: notFoundMessage,
		},
		{name: "внутренняя", err: errors.New("boom"), wantCode: codes.Internal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := toStatusError(context.Background(), tt.err)
			if status.Code(err) != tt.wantCode {
				t.Errorf("toStatusError() code = %v, want %v", status.Code(err), tt.wantCode)
			}

			if tt.err == nil && err != nil {
				t.Errorf("toStatusError(nil) = %v, want nil", err)
			}

			if tt.wantMessage != "" && status.Convert(err).Message() != tt.wantMessage {
				t.Errorf("toStatusError() message = %q, want %q", status.Convert(err).Message(), tt.wantMessage)
			}
		})
	}
}

// TestUserServiceServer_NoPrincipal проверяет вызов без действующего пользователя:
// адаптер отвечает Unauthenticated, не обращаясь к сервису. Так вызов, который
// каким-то образом обошёл middleware или интерцептор, не станет анонимным.
func TestUserServiceServer_NoPrincipal(t *testing.T) {
	api := &mockUserServiceAPI{
		createFunc: func(context.Context, *service.Principal, model.CreateUserInput) (*model.User, error) {
			t.Error("Create() дошёл до сервиса без действующего пользователя")

			return nil, nil
		},
		getFunc: func(context.Context, *service.Principal, string) (*model.User, error) {
			t.Error("Get() дошёл до сервиса без действующего пользователя")

			return nil, nil
		},
		updateFunc: func(
			context.Context,
			*service.Principal,
			string,
			model.UpdateUserInput,
		) (*model.User, error) {
			t.Error("Update() дошёл до сервиса без действующего пользователя")

			return nil, nil
		},
		deleteFunc: func(context.Context, *service.Principal, string) error {
			t.Error("Delete() дошёл до сервиса без действующего пользователя")

			return nil
		},
		assignGlobalOperatorFunc: func(context.Context, *service.Principal, string) (*model.User, error) {
			t.Error("AssignGlobalOperator() дошёл до сервиса без действующего пользователя")

			return nil, nil
		},
		revokeGlobalOperatorFunc: func(context.Context, *service.Principal, string) error {
			t.Error("RevokeGlobalOperator() дошёл до сервиса без действующего пользователя")

			return nil
		},
	}

	server := NewUserServiceServer(api)
	ctx := context.Background()

	calls := map[string]error{}

	_, err := server.Create(ctx, &user_v1.UserServiceCreateRequest{
		Email:     "ivan@example.com",
		FirstName: "Иван",
		LastName:  "Иванов",
	})
	calls["Create"] = err

	_, err = server.Get(ctx, &user_v1.UserServiceGetRequest{Uuid: validUUID})
	calls["Get"] = err

	_, err = server.Update(ctx, &user_v1.UserServiceUpdateRequest{Uuid: validUUID})
	calls["Update"] = err

	_, err = server.Delete(ctx, &user_v1.UserServiceDeleteRequest{Uuid: validUUID})
	calls["Delete"] = err

	_, err = server.AssignGlobalOperator(ctx, &user_v1.UserServiceAssignGlobalOperatorRequest{Uuid: validUUID})
	calls["AssignGlobalOperator"] = err

	_, err = server.RevokeGlobalOperator(ctx, &user_v1.UserServiceRevokeGlobalOperatorRequest{Uuid: validUUID})
	calls["RevokeGlobalOperator"] = err

	for method, err := range calls {
		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("%s() code = %v, want %v (err = %v)", method, status.Code(err), codes.Unauthenticated, err)
		}
	}
}
