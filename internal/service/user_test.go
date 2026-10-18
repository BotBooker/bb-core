package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/model"
)

// mockUserRepository — мок репозитория: тесты подменяют отдельные операции,
// чтобы проверить и проброс результата, и то, что запись не выполнялась.
type mockUserRepository struct {
	createFunc      func(ctx context.Context, user *model.User) error
	getByIDFunc     func(ctx context.Context, id string) (*model.User, error)
	updateFunc      func(ctx context.Context, user *model.User) error
	deleteFunc      func(ctx context.Context, id string) error
	listAllFunc     func(ctx context.Context, limit, offset int) ([]*model.User, error)
	listVisibleFunc func(ctx context.Context, tenantID string, limit, offset int) ([]*model.User, error)
	getVisibleFunc  func(ctx context.Context, tenantID, id string) (*model.User, error)
}

func (m *mockUserRepository) Create(ctx context.Context, user *model.User) error {
	if m.createFunc != nil {
		return m.createFunc(ctx, user)
	}

	return nil
}

func (m *mockUserRepository) GetByID(ctx context.Context, id string) (*model.User, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}

	return nil, database.ErrNotFound
}

func (m *mockUserRepository) Update(ctx context.Context, user *model.User) error {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, user)
	}

	return nil
}

func (m *mockUserRepository) Delete(ctx context.Context, id string) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, id)
	}

	return nil
}

func (m *mockUserRepository) ListAll(ctx context.Context, limit, offset int) ([]*model.User, error) {
	if m.listAllFunc != nil {
		return m.listAllFunc(ctx, limit, offset)
	}

	return nil, nil
}

func (m *mockUserRepository) ListVisible(
	ctx context.Context,
	tenantID string,
	limit, offset int,
) ([]*model.User, error) {
	if m.listVisibleFunc != nil {
		return m.listVisibleFunc(ctx, tenantID, limit, offset)
	}

	return nil, nil
}

func (m *mockUserRepository) GetVisible(ctx context.Context, tenantID, id string) (*model.User, error) {
	if m.getVisibleFunc != nil {
		return m.getVisibleFunc(ctx, tenantID, id)
	}

	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}

	return nil, database.ErrNotFound
}

// mockUserMembershipRepository — мок членства для UserService.
type mockUserMembershipRepository struct {
	addMemberFunc func(ctx context.Context, member *model.TenantMember) error
	getMemberFunc func(ctx context.Context, tenantID, userID string) (*model.TenantMember, error)
}

func (m *mockUserMembershipRepository) AddMember(ctx context.Context, member *model.TenantMember) error {
	if m.addMemberFunc != nil {
		return m.addMemberFunc(ctx, member)
	}

	return nil
}

func (m *mockUserMembershipRepository) GetMember(
	ctx context.Context,
	tenantID, userID string,
) (*model.TenantMember, error) {
	if m.getMemberFunc != nil {
		return m.getMemberFunc(ctx, tenantID, userID)
	}

	// По умолчанию цель считается участником тенанта: тесты, которым
	// членство не важно, проверяют другие правила.
	return &model.TenantMember{TenantID: tenantID, UserID: userID, Role: model.RoleClient}, nil
}

// testPrincipal возвращает действующего пользователя с заданной ролью в тенанте.
//
// Идентификатор отличается от идентификаторов записей в тестах: иначе проверка
// прав проходила бы «сама собой» — из-за совпадения действующего пользователя
// с целью операции. Роль хранится в Principal, а не в профиле: она приходит из
// членства в тенанте сессии.
func testPrincipal(t *testing.T, role model.Role) *Principal {
	t.Helper()

	return &Principal{
		User:  storedUser(t, "7c9e6679-7425-40de-944b-e07fc1f90ae7"),
		Token: &model.Token{TenantID: testTenantID, UserID: "7c9e6679-7425-40de-944b-e07fc1f90ae7"},
		Role:  role,
	}
}

// testSuperuserPrincipal — действующий суперпользователь: системный флаг
// важнее роли в тенанте, поэтому роль может быть любой.
func testSuperuserPrincipal(t *testing.T) *Principal {
	t.Helper()

	principal := testPrincipal(t, model.RoleClient)
	principal.User.IsSuperuser = true

	return principal
}

// testGlobalOperatorPrincipal — действующий глобальный оператор: он управляет
// всеми тенантами, но назначать других операторов не может.
func testGlobalOperatorPrincipal(t *testing.T) *Principal {
	t.Helper()

	principal := testPrincipal(t, model.RoleClient)
	principal.User.IsGlobalOperator = true

	return principal
}

// mockUserEventPublisher запоминает опубликованные события: так проверяется,
// что событие уходит только после успешной операции.
type mockUserEventPublisher struct {
	events []string
}

func (m *mockUserEventPublisher) Publish(_ context.Context, event string) {
	m.events = append(m.events, event)
}

// testCreateInput — валидный вход создания пользователя.
func testCreateInput() model.CreateUserInput {
	return model.CreateUserInput{
		Email:       "ivan@example.com",
		FirstName:   "Иван",
		LastName:    "Иванов",
		PhoneNumber: "+79990000001",
		Gender:      model.GenderMale,
	}
}

// newTestUserService создаёт сервис с моками и возвращает их для проверок.
func newTestUserService(repo UserRepository) (UserService, *mockUserEventPublisher) {
	events := &mockUserEventPublisher{}

	return NewUserService(context.Background(), repo, &mockUserMembershipRepository{}, events), events
}

func TestNewUserService(t *testing.T) {
	tests := []struct {
		name     string
		userRepo UserRepository
		events   UserEventPublisher
	}{
		{
			name:     "all valid dependencies",
			userRepo: &mockUserRepository{},
			events:   &mockUserEventPublisher{},
		},
		{
			name:     "nil user repo",
			userRepo: nil,
			events:   &mockUserEventPublisher{},
		},
		{
			name:     "nil events",
			userRepo: &mockUserRepository{},
			events:   nil,
		},
		{
			name:     "all nil",
			userRepo: nil,
			events:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewUserService(context.Background(), tt.userRepo, &mockUserMembershipRepository{}, tt.events)
			if got == nil {
				t.Error("NewUserService() returned nil")
			}
		})
	}
}

func TestUserService_Create(t *testing.T) {
	t.Run("создаёт пользователя и публикует событие", func(t *testing.T) {
		var stored *model.User

		repo := &mockUserRepository{createFunc: func(_ context.Context, user *model.User) error {
			stored = user

			return nil
		}}
		svc, events := newTestUserService(repo)

		user, err := svc.Create(context.Background(), testPrincipal(t, model.RoleOperator), testCreateInput())
		if err != nil {
			t.Fatalf("Create() unexpected error = %v", err)
		}

		// Идентификатор проставляет модель, а не репозиторий; роли у профиля
		// нет — она принадлежит членству в тенанте.
		if err := model.ValidateUUID(user.ID); err != nil {
			t.Errorf("Create() вернул пользователя с невалидным UUID: %v", err)
		}
		if stored != user {
			t.Error("Create() не передал созданного пользователя в репозиторий")
		}
		if len(events.events) != 1 || events.events[0] != EventUserCreated {
			t.Errorf("события = %v, want [%s]", events.events, EventUserCreated)
		}
	})

	t.Run("невалидный вход не доходит до репозитория", func(t *testing.T) {
		called := false

		repo := &mockUserRepository{createFunc: func(context.Context, *model.User) error {
			called = true

			return nil
		}}
		svc, events := newTestUserService(repo)

		in := testCreateInput()
		in.Email = "не-email"

		_, err := svc.Create(context.Background(), testPrincipal(t, model.RoleAdmin), in)
		if !errors.Is(err, model.ErrInvalidEmail) {
			t.Errorf("Create() error = %v, want %v", err, model.ErrInvalidEmail)
		}
		if called {
			t.Error("Create() вызвал репозиторий при невалидном входе")
		}
		if len(events.events) != 0 {
			t.Errorf("события = %v, want пусто", events.events)
		}
	})

	t.Run("пробрасывает конфликт уникальности", func(t *testing.T) {
		repo := &mockUserRepository{createFunc: func(context.Context, *model.User) error {
			return database.ErrDuplicate
		}}
		svc, events := newTestUserService(repo)

		_, err := svc.Create(context.Background(), testPrincipal(t, model.RoleAdmin), testCreateInput())
		if !errors.Is(err, database.ErrDuplicate) {
			t.Errorf("Create() error = %v, want %v", err, database.ErrDuplicate)
		}
		if len(events.events) != 0 {
			t.Errorf("события = %v, want пусто: операция не удалась", events.events)
		}
	})
}

// storedUser — пользователь, как его вернул бы репозиторий: валидный UUID,
// роль client и заполненные поля.
func storedUser(t *testing.T, id string) *model.User {
	t.Helper()

	user, err := model.NewUser(testCreateInput(), time.Now())
	if err != nil {
		t.Fatalf("model.NewUser() unexpected error = %v", err)
	}

	user.ID = id

	return user
}

func TestUserService_Get(t *testing.T) {
	const id = "3f4a1e2c-5b6d-4e8f-9a0b-1c2d3e4f5a6b"

	t.Run("возвращает пользователя", func(t *testing.T) {
		want := storedUser(t, id)

		repo := &mockUserRepository{getByIDFunc: func(_ context.Context, gotID string) (*model.User, error) {
			if gotID != id {
				t.Errorf("GetByID() id = %q, want %q", gotID, id)
			}

			return want, nil
		}}
		svc, _ := newTestUserService(repo)

		got, err := svc.Get(context.Background(), testPrincipal(t, model.RoleOperator), id)
		if err != nil {
			t.Fatalf("Get() unexpected error = %v", err)
		}
		if got != want {
			t.Error("Get() вернул не ту запись, что отдал репозиторий")
		}
	})

	t.Run("пробрасывает «не найдено»", func(t *testing.T) {
		repo := &mockUserRepository{getByIDFunc: func(context.Context, string) (*model.User, error) {
			return nil, database.ErrNotFound
		}}
		svc, _ := newTestUserService(repo)

		if _, err := svc.Get(context.Background(), testPrincipal(t, model.RoleOperator), id); !errors.Is(err, database.ErrNotFound) {
			t.Errorf("Get() error = %v, want %v", err, database.ErrNotFound)
		}
	})

	t.Run("невалидный UUID не доходит до репозитория", func(t *testing.T) {
		called := false

		repo := &mockUserRepository{getByIDFunc: func(context.Context, string) (*model.User, error) {
			called = true

			return nil, nil
		}}
		svc, _ := newTestUserService(repo)

		if _, err := svc.Get(context.Background(), testPrincipal(t, model.RoleOperator), "не-uuid"); !errors.Is(err, model.ErrInvalidUUID) {
			t.Errorf("Get() error = %v, want %v", err, model.ErrInvalidUUID)
		}
		if called {
			t.Error("Get() вызвал репозиторий с невалидным UUID")
		}
	})
}

func TestUserService_Update(t *testing.T) {
	const id = "3f4a1e2c-5b6d-4e8f-9a0b-1c2d3e4f5a6b"

	t.Run("применяет частичное обновление", func(t *testing.T) {
		existing := storedUser(t, id)
		// Системные флаги не входят в UpdateUserInput: обновление профиля их
		// не трогает, даже если они выставлены.
		existing.IsSuperuser = true
		existing.IsGlobalOperator = true

		firstName := "Пётр"

		var stored *model.User

		repo := &mockUserRepository{
			getByIDFunc: func(context.Context, string) (*model.User, error) { return existing, nil },
			updateFunc: func(_ context.Context, user *model.User) error {
				stored = user

				return nil
			},
		}
		svc, events := newTestUserService(repo)

		got, err := svc.Update(context.Background(), testPrincipal(t, model.RoleAdmin), id,
			model.UpdateUserInput{FirstName: &firstName})
		if err != nil {
			t.Fatalf("Update() unexpected error = %v", err)
		}
		if got.FirstName != firstName {
			t.Errorf("FirstName = %q, want %q", got.FirstName, firstName)
		}
		if got.Email != existing.Email || got.LastName != existing.LastName {
			t.Error("Update() изменил поля, которых не было во входе")
		}
		if !got.IsSuperuser || !got.IsGlobalOperator {
			t.Error("Update() сбросил системные флаги суперпользователя/глобального оператора")
		}
		if !got.UpdatedAt.After(got.CreatedAt) {
			t.Errorf("UpdatedAt = %v, want позже CreatedAt %v", got.UpdatedAt, got.CreatedAt)
		}
		if stored != got {
			t.Error("Update() передал в репозиторий не обновлённую запись")
		}
		if len(events.events) != 1 || events.events[0] != EventUserUpdated {
			t.Errorf("события = %v, want [%s]", events.events, EventUserUpdated)
		}
	})

	t.Run("невалидный вход не меняет запись", func(t *testing.T) {
		updateCalled := false
		empty := ""

		repo := &mockUserRepository{
			getByIDFunc: func(context.Context, string) (*model.User, error) { return storedUser(t, id), nil },
			updateFunc: func(context.Context, *model.User) error {
				updateCalled = true

				return nil
			},
		}
		svc, _ := newTestUserService(repo)

		_, err := svc.Update(context.Background(), testPrincipal(t, model.RoleAdmin), id,
			model.UpdateUserInput{FirstName: &empty})
		if !errors.Is(err, model.ErrNameRequired) {
			t.Errorf("Update() error = %v, want %v", err, model.ErrNameRequired)
		}
		if updateCalled {
			t.Error("Update() записал в базу невалидное обновление")
		}
	})

	t.Run("не найдено", func(t *testing.T) {
		repo := &mockUserRepository{getByIDFunc: func(context.Context, string) (*model.User, error) {
			return nil, database.ErrNotFound
		}}
		svc, _ := newTestUserService(repo)

		_, err := svc.Update(context.Background(), testPrincipal(t, model.RoleAdmin), id, model.UpdateUserInput{})
		if !errors.Is(err, database.ErrNotFound) {
			t.Errorf("Update() error = %v, want %v", err, database.ErrNotFound)
		}
	})
}

func TestUserService_Delete(t *testing.T) {
	const id = "3f4a1e2c-5b6d-4e8f-9a0b-1c2d3e4f5a6b"

	t.Run("удаляет и публикует событие", func(t *testing.T) {
		var deletedID string

		repo := &mockUserRepository{deleteFunc: func(_ context.Context, gotID string) error {
			deletedID = gotID

			return nil
		}}
		svc, events := newTestUserService(repo)

		if err := svc.Delete(context.Background(), testPrincipal(t, model.RoleAdmin), id); err != nil {
			t.Fatalf("Delete() unexpected error = %v", err)
		}
		if deletedID != id {
			t.Errorf("Delete() id = %q, want %q", deletedID, id)
		}
		if len(events.events) != 1 || events.events[0] != EventUserDeleted {
			t.Errorf("события = %v, want [%s]", events.events, EventUserDeleted)
		}
	})

	t.Run("пробрасывает «не найдено»", func(t *testing.T) {
		repo := &mockUserRepository{deleteFunc: func(context.Context, string) error {
			return database.ErrNotFound
		}}
		svc, _ := newTestUserService(repo)

		if err := svc.Delete(context.Background(), testPrincipal(t, model.RoleAdmin), id); !errors.Is(err, database.ErrNotFound) {
			t.Errorf("Delete() error = %v, want %v", err, database.ErrNotFound)
		}
	})

	t.Run("невалидный UUID не доходит до репозитория", func(t *testing.T) {
		called := false

		repo := &mockUserRepository{deleteFunc: func(context.Context, string) error {
			called = true

			return nil
		}}
		svc, _ := newTestUserService(repo)

		if err := svc.Delete(context.Background(), testPrincipal(t, model.RoleAdmin), "не-uuid"); !errors.Is(err, model.ErrInvalidUUID) {
			t.Errorf("Delete() error = %v, want %v", err, model.ErrInvalidUUID)
		}
		if called {
			t.Error("Delete() вызвал репозиторий с невалидным UUID")
		}
	})
}

// TestUserService_AssignGlobalOperator проверяет выдачу прав глобального
// оператора: назначает только суперпользователь, а суперпользователя изменить
// нельзя.
func TestUserService_AssignGlobalOperator(t *testing.T) {
	const id = "3f4a1e2c-5b6d-4e8f-9a0b-1c2d3e4f5a6b"

	t.Run("назначает глобального оператора", func(t *testing.T) {
		target := storedUser(t, id)

		var updated *model.User

		repo := &mockUserRepository{
			getByIDFunc: func(context.Context, string) (*model.User, error) { return target, nil },
			updateFunc: func(_ context.Context, user *model.User) error {
				updated = user

				return nil
			},
		}
		svc, events := newTestUserService(repo)

		got, err := svc.AssignGlobalOperator(context.Background(), testSuperuserPrincipal(t), id)
		if err != nil {
			t.Fatalf("AssignGlobalOperator() unexpected error = %v", err)
		}
		if !got.IsGlobalOperator {
			t.Error("IsGlobalOperator = false, want true")
		}
		if updated != target {
			t.Error("AssignGlobalOperator() не сохранил пользователя")
		}
		if len(events.events) != 1 || events.events[0] != EventGlobalOperatorAssigned {
			t.Errorf("события = %v, want [%s]", events.events, EventGlobalOperatorAssigned)
		}
	})

	t.Run("повторное назначение идемпотентно", func(t *testing.T) {
		target := storedUser(t, id)
		target.IsGlobalOperator = true

		updateCalled := false

		repo := &mockUserRepository{
			getByIDFunc: func(context.Context, string) (*model.User, error) { return target, nil },
			updateFunc: func(context.Context, *model.User) error {
				updateCalled = true

				return nil
			},
		}
		svc, events := newTestUserService(repo)

		got, err := svc.AssignGlobalOperator(context.Background(), testSuperuserPrincipal(t), id)
		if err != nil {
			t.Fatalf("AssignGlobalOperator() unexpected error = %v", err)
		}
		if got != target {
			t.Error("AssignGlobalOperator() вернул другого пользователя")
		}
		if updateCalled {
			t.Error("AssignGlobalOperator() записал уже назначенного оператора")
		}
		if len(events.events) != 0 {
			t.Errorf("события = %v, want пусто", events.events)
		}
	})

	t.Run("ошибки", func(t *testing.T) {
		superuserTarget := storedUser(t, id)
		superuserTarget.IsSuperuser = true

		tests := []struct {
			name    string
			actor   *Principal
			id      string
			target  *model.User
			wantErr error
		}{
			{
				name:    "назначает не суперпользователь",
				actor:   testPrincipal(t, model.RoleAdmin),
				id:      id,
				target:  storedUser(t, id),
				wantErr: ErrForbidden,
			},
			{
				name:    "глобальный оператор не назначает другого",
				actor:   testGlobalOperatorPrincipal(t),
				id:      id,
				target:  storedUser(t, id),
				wantErr: ErrForbidden,
			},
			{
				name:    "цель — суперпользователь",
				actor:   testSuperuserPrincipal(t),
				id:      id,
				target:  superuserTarget,
				wantErr: ErrForbidden,
			},
			{
				name:    "невалидный UUID",
				actor:   testSuperuserPrincipal(t),
				id:      "не-uuid",
				target:  storedUser(t, id),
				wantErr: model.ErrInvalidUUID,
			},
			{
				name:    "нет действующего пользователя",
				actor:   nil,
				id:      id,
				target:  storedUser(t, id),
				wantErr: ErrUnauthorized,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				repo := &mockUserRepository{
					getByIDFunc: func(context.Context, string) (*model.User, error) { return tt.target, nil },
				}
				svc, _ := newTestUserService(repo)

				if _, err := svc.AssignGlobalOperator(context.Background(), tt.actor, tt.id); !errors.Is(err, tt.wantErr) {
					t.Errorf("AssignGlobalOperator() error = %v, want %v", err, tt.wantErr)
				}
			})
		}
	})
}

// TestUserService_RevokeGlobalOperator проверяет снятие прав: доступно только
// суперпользователю, суперпользователя изменить нельзя, повторное снятие
// идемпотентно.
func TestUserService_RevokeGlobalOperator(t *testing.T) {
	const id = "3f4a1e2c-5b6d-4e8f-9a0b-1c2d3e4f5a6b"

	t.Run("снимает права глобального оператора", func(t *testing.T) {
		target := storedUser(t, id)
		target.IsGlobalOperator = true

		var updated *model.User

		repo := &mockUserRepository{
			getByIDFunc: func(context.Context, string) (*model.User, error) { return target, nil },
			updateFunc: func(_ context.Context, user *model.User) error {
				updated = user

				return nil
			},
		}
		svc, events := newTestUserService(repo)

		if err := svc.RevokeGlobalOperator(context.Background(), testSuperuserPrincipal(t), id); err != nil {
			t.Fatalf("RevokeGlobalOperator() unexpected error = %v", err)
		}
		if target.IsGlobalOperator {
			t.Error("IsGlobalOperator = true, want false")
		}
		if updated != target {
			t.Error("RevokeGlobalOperator() не сохранил пользователя")
		}
		if len(events.events) != 1 || events.events[0] != EventGlobalOperatorRevoked {
			t.Errorf("события = %v, want [%s]", events.events, EventGlobalOperatorRevoked)
		}
	})

	t.Run("повторное снятие идемпотентно", func(t *testing.T) {
		target := storedUser(t, id)

		updateCalled := false

		repo := &mockUserRepository{
			getByIDFunc: func(context.Context, string) (*model.User, error) { return target, nil },
			updateFunc: func(context.Context, *model.User) error {
				updateCalled = true

				return nil
			},
		}
		svc, events := newTestUserService(repo)

		if err := svc.RevokeGlobalOperator(context.Background(), testSuperuserPrincipal(t), id); err != nil {
			t.Fatalf("RevokeGlobalOperator() unexpected error = %v", err)
		}
		if updateCalled {
			t.Error("RevokeGlobalOperator() записал пользователя без прав оператора")
		}
		if len(events.events) != 0 {
			t.Errorf("события = %v, want пусто", events.events)
		}
	})

	t.Run("ошибки", func(t *testing.T) {
		superuserTarget := storedUser(t, id)
		superuserTarget.IsSuperuser = true

		tests := []struct {
			name    string
			actor   *Principal
			id      string
			target  *model.User
			wantErr error
		}{
			{
				name:    "снимает не суперпользователь",
				actor:   testPrincipal(t, model.RoleAdmin),
				id:      id,
				target:  storedUser(t, id),
				wantErr: ErrForbidden,
			},
			{
				name:    "глобальный оператор не снимает права",
				actor:   testGlobalOperatorPrincipal(t),
				id:      id,
				target:  storedUser(t, id),
				wantErr: ErrForbidden,
			},
			{
				name:    "цель — суперпользователь",
				actor:   testSuperuserPrincipal(t),
				id:      id,
				target:  superuserTarget,
				wantErr: ErrForbidden,
			},
			{
				name:    "невалидный UUID",
				actor:   testSuperuserPrincipal(t),
				id:      "не-uuid",
				target:  storedUser(t, id),
				wantErr: model.ErrInvalidUUID,
			},
			{
				name:    "нет действующего пользователя",
				actor:   nil,
				id:      id,
				target:  storedUser(t, id),
				wantErr: ErrUnauthorized,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				repo := &mockUserRepository{
					getByIDFunc: func(context.Context, string) (*model.User, error) { return tt.target, nil },
				}
				svc, _ := newTestUserService(repo)

				if err := svc.RevokeGlobalOperator(context.Background(), tt.actor, tt.id); !errors.Is(err, tt.wantErr) {
					t.Errorf("RevokeGlobalOperator() error = %v, want %v", err, tt.wantErr)
				}
			})
		}
	})
}

// TestUserService_Permissions проверяет ролевую модель: права решает сервис, а
// не транспорт, поэтому одна таблица случаев покрывает и HTTP, и gRPC.
func TestUserService_Permissions(t *testing.T) {
	const id = "3f4a1e2c-5b6d-4e8f-9a0b-1c2d3e4f5a6b"

	// target — чужой профиль: действующий пользователь в тестах имеет другой UUID.
	target := storedUser(t, id)

	newRepo := func() *mockUserRepository {
		return &mockUserRepository{
			getByIDFunc: func(context.Context, string) (*model.User, error) { return target, nil },
			createFunc:  func(context.Context, *model.User) error { return nil },
			updateFunc:  func(context.Context, *model.User) error { return nil },
			deleteFunc:  func(context.Context, string) error { return nil },
		}
	}

	tests := []struct {
		name    string
		role    model.Role
		wantErr error
	}{
		{name: "клиент видит только свой профиль", role: model.RoleClient, wantErr: ErrForbidden},
		{name: "оператор видит чужой профиль", role: model.RoleOperator},
		{name: "администратор видит чужой профиль", role: model.RoleAdmin},
	}

	for _, tt := range tests {
		t.Run("Get: "+tt.name, func(t *testing.T) {
			svc := NewUserService(context.Background(), newRepo(), &mockUserMembershipRepository{}, &mockUserEventPublisher{})

			_, err := svc.Get(context.Background(), testPrincipal(t, tt.role), id)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Get() error = %v, want %v", err, tt.wantErr)
			}
		})
	}

	tests = []struct {
		name    string
		role    model.Role
		wantErr error
	}{
		{name: "клиент не заводит пользователей", role: model.RoleClient, wantErr: ErrForbidden},
		{name: "оператор заводит пользователей", role: model.RoleOperator},
		{name: "администратор заводит пользователей", role: model.RoleAdmin},
	}

	for _, tt := range tests {
		t.Run("Create: "+tt.name, func(t *testing.T) {
			svc := NewUserService(context.Background(), newRepo(), &mockUserMembershipRepository{}, &mockUserEventPublisher{})

			_, err := svc.Create(context.Background(), testPrincipal(t, tt.role), testCreateInput())
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Create() error = %v, want %v", err, tt.wantErr)
			}
		})
	}

	tests = []struct {
		name    string
		role    model.Role
		wantErr error
	}{
		{name: "клиент не правит чужой профиль", role: model.RoleClient, wantErr: ErrForbidden},
		{name: "оператор не правит чужой профиль", role: model.RoleOperator, wantErr: ErrForbidden},
		{name: "администратор правит чужой профиль", role: model.RoleAdmin},
	}

	for _, tt := range tests {
		t.Run("Update: "+tt.name, func(t *testing.T) {
			svc := NewUserService(context.Background(), newRepo(), &mockUserMembershipRepository{}, &mockUserEventPublisher{})

			_, err := svc.Update(context.Background(), testPrincipal(t, tt.role), id, model.UpdateUserInput{})
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Update() error = %v, want %v", err, tt.wantErr)
			}
		})
	}

	tests = []struct {
		name    string
		role    model.Role
		wantErr error
	}{
		{name: "клиент не удаляет", role: model.RoleClient, wantErr: ErrForbidden},
		{name: "оператор не удаляет", role: model.RoleOperator, wantErr: ErrForbidden},
		{name: "администратор удаляет", role: model.RoleAdmin},
	}

	for _, tt := range tests {
		t.Run("Delete: "+tt.name, func(t *testing.T) {
			svc := NewUserService(context.Background(), newRepo(), &mockUserMembershipRepository{}, &mockUserEventPublisher{})

			err := svc.Delete(context.Background(), testPrincipal(t, tt.role), id)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Delete() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// TestUserService_OwnProfile разрешает изменять свой профиль: клиент и оператор
// видят и правят себя без дополнительных прав.
func TestUserService_OwnProfile(t *testing.T) {
	for _, role := range []model.Role{model.RoleClient, model.RoleOperator, model.RoleAdmin} {
		t.Run(string(role), func(t *testing.T) {
			actor := testPrincipal(t, role)

			repo := &mockUserRepository{
				getByIDFunc: func(context.Context, string) (*model.User, error) { return actor.User, nil },
				updateFunc:  func(context.Context, *model.User) error { return nil },
			}
			svc := NewUserService(context.Background(), repo, &mockUserMembershipRepository{}, &mockUserEventPublisher{})

			if _, err := svc.Get(context.Background(), actor, actor.ID()); err != nil {
				t.Errorf("Get() своего профиля unexpected error = %v", err)
			}

			if _, err := svc.Update(context.Background(), actor, actor.ID(), model.UpdateUserInput{}); err != nil {
				t.Errorf("Update() своего профиля unexpected error = %v", err)
			}
		})
	}
}

// TestUserService_NoPrincipal проверяет запрос без действующего пользователя:
// сервис отвечает ErrUnauthorized, а не падает с разыменованием nil.
func TestUserService_NoPrincipal(t *testing.T) {
	svc := NewUserService(context.Background(), &mockUserRepository{}, &mockUserMembershipRepository{}, &mockUserEventPublisher{})

	if _, err := svc.Create(context.Background(), nil, testCreateInput()); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("Create() error = %v, want %v", err, ErrUnauthorized)
	}

	if _, err := svc.Get(context.Background(), nil, testCreateInput().Email); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("Get() error = %v, want %v", err, ErrUnauthorized)
	}

	if _, err := svc.Update(context.Background(), nil, testCreateInput().Email, model.UpdateUserInput{}); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("Update() error = %v, want %v", err, ErrUnauthorized)
	}

	if err := svc.Delete(context.Background(), nil, testCreateInput().Email); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("Delete() error = %v, want %v", err, ErrUnauthorized)
	}

	if _, err := svc.AssignGlobalOperator(context.Background(), nil, testCreateInput().Email); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("AssignGlobalOperator() error = %v, want %v", err, ErrUnauthorized)
	}

	if err := svc.RevokeGlobalOperator(context.Background(), nil, testCreateInput().Email); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("RevokeGlobalOperator() error = %v, want %v", err, ErrUnauthorized)
	}
}
