package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/model"
)

// otherTenantID — тенант, отличный от тенанта сессии: нужен проверкам
// «членство есть, но сессия в другом тенанте».
const otherTenantID = "11111111-2222-4333-8444-555555555555"

// testUserID — идентификатор действующего пользователя в тестах (см.
// testPrincipal); otherUserID — цель операций управления участниками.
const (
	testUserID  = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	otherUserID = "22222222-3333-4444-8555-666666666666"
)

// mockTenantRepository — мок репозитория тенантов.
type mockTenantRepository struct {
	createWithOwnerFunc   func(ctx context.Context, tenant *model.Tenant, member *model.TenantMember) error
	getByIDFunc           func(ctx context.Context, id string) (*model.Tenant, error)
	listFunc              func(ctx context.Context, limit, offset int) ([]*model.Tenant, error)
	updateStatusFunc      func(ctx context.Context, id string, status model.TenantStatus, now time.Time) error
	addMemberFunc         func(ctx context.Context, member *model.TenantMember) error
	getMemberFunc         func(ctx context.Context, tenantID, userID string) (*model.TenantMember, error)
	listMembershipsFunc   func(ctx context.Context, userID string) ([]*model.TenantMembership, error)
	listMembersFunc       func(ctx context.Context, tenantID string, limit, offset int) ([]*model.TenantMemberView, error)
	getMemberViewFunc     func(ctx context.Context, tenantID, userID string) (*model.TenantMemberView, error)
	updateMemberRoleFunc  func(ctx context.Context, tenantID, userID string, role model.Role) error
	removeMemberFunc      func(ctx context.Context, tenantID, userID string) error
	transferOwnershipFunc func(ctx context.Context, tenantID, userID string) error
}

func (m *mockTenantRepository) CreateWithOwner(
	ctx context.Context,
	tenant *model.Tenant,
	member *model.TenantMember,
) error {
	if m.createWithOwnerFunc != nil {
		return m.createWithOwnerFunc(ctx, tenant, member)
	}

	return nil
}

func (m *mockTenantRepository) GetByID(ctx context.Context, id string) (*model.Tenant, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}

	return &model.Tenant{ID: id, Status: model.TenantStatusActive}, nil
}

func (m *mockTenantRepository) List(
	ctx context.Context,
	limit, offset int,
) ([]*model.Tenant, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, limit, offset)
	}

	return nil, nil
}

func (m *mockTenantRepository) UpdateStatus(
	ctx context.Context,
	id string,
	status model.TenantStatus,
	now time.Time,
) error {
	if m.updateStatusFunc != nil {
		return m.updateStatusFunc(ctx, id, status, now)
	}

	return nil
}

func (m *mockTenantRepository) AddMember(ctx context.Context, member *model.TenantMember) error {
	if m.addMemberFunc != nil {
		return m.addMemberFunc(ctx, member)
	}

	return nil
}

func (m *mockTenantRepository) GetMember(
	ctx context.Context,
	tenantID, userID string,
) (*model.TenantMember, error) {
	if m.getMemberFunc != nil {
		return m.getMemberFunc(ctx, tenantID, userID)
	}

	return nil, database.ErrNotFound
}

func (m *mockTenantRepository) ListMemberships(
	ctx context.Context,
	userID string,
) ([]*model.TenantMembership, error) {
	if m.listMembershipsFunc != nil {
		return m.listMembershipsFunc(ctx, userID)
	}

	return nil, nil
}

func (m *mockTenantRepository) ListMembers(
	ctx context.Context,
	tenantID string,
	limit, offset int,
) ([]*model.TenantMemberView, error) {
	if m.listMembersFunc != nil {
		return m.listMembersFunc(ctx, tenantID, limit, offset)
	}

	return nil, nil
}

func (m *mockTenantRepository) GetMemberView(
	ctx context.Context,
	tenantID, userID string,
) (*model.TenantMemberView, error) {
	if m.getMemberViewFunc != nil {
		return m.getMemberViewFunc(ctx, tenantID, userID)
	}

	return &model.TenantMemberView{
		TenantMember: model.TenantMember{
			TenantID: tenantID,
			UserID:   userID,
			Role:     model.RoleClient,
			JoinedAt: testNow,
		},
	}, nil
}

func (m *mockTenantRepository) UpdateMemberRole(
	ctx context.Context,
	tenantID, userID string,
	role model.Role,
) error {
	if m.updateMemberRoleFunc != nil {
		return m.updateMemberRoleFunc(ctx, tenantID, userID, role)
	}

	return nil
}

func (m *mockTenantRepository) RemoveMember(ctx context.Context, tenantID, userID string) error {
	if m.removeMemberFunc != nil {
		return m.removeMemberFunc(ctx, tenantID, userID)
	}

	return nil
}

func (m *mockTenantRepository) TransferOwnership(ctx context.Context, tenantID, userID string) error {
	if m.transferOwnershipFunc != nil {
		return m.transferOwnershipFunc(ctx, tenantID, userID)
	}

	return nil
}

// mockTenantSessionRepository — мок отзыва сессий.
type mockTenantSessionRepository struct {
	revokeTenantFunc func(ctx context.Context, tenantID string, at time.Time) (int64, error)
	revokeMemberFunc func(ctx context.Context, userID, tenantID string, at time.Time) (int64, error)
}

func (m *mockTenantSessionRepository) RevokeByTenant(
	ctx context.Context,
	tenantID string,
	at time.Time,
) (int64, error) {
	if m.revokeTenantFunc != nil {
		return m.revokeTenantFunc(ctx, tenantID, at)
	}

	return 0, nil
}

func (m *mockTenantSessionRepository) RevokeByUserTenant(
	ctx context.Context,
	userID, tenantID string,
	at time.Time,
) (int64, error) {
	if m.revokeMemberFunc != nil {
		return m.revokeMemberFunc(ctx, userID, tenantID, at)
	}

	return 0, nil
}

// newTestTenantService создаёт сервис с моками и фиксированным временем.
func newTestTenantService(
	t *testing.T,
	tenants TenantRepository,
	users TenantUserRepository,
	tokens TenantSessionRepository,
) (*tenantService, *mockUserEventPublisher) {
	t.Helper()

	events := &mockUserEventPublisher{}

	created := NewTenantService(context.Background(), tenants, users, tokens, events)

	svc, ok := created.(*tenantService)
	if !ok {
		t.Fatalf("NewTenantService() returned %T, want *tenantService", created)
	}

	svc.now = func() time.Time { return testNow }

	return svc, events
}

// tenantActorIn возвращает Principal с сессией в заданном тенанте.
func tenantActorIn(t *testing.T, role model.Role, tenantID string) *Principal {
	t.Helper()

	principal := testPrincipal(t, role)
	principal.Token.TenantID = tenantID

	return principal
}

// activeTenant возвращает действующий тенант с заданным идентификатором.
func activeTenant(id string) *model.Tenant {
	return &model.Tenant{ID: id, Name: "Тенант", Slug: "tenant", Status: model.TenantStatusActive}
}

func TestTenantService_Create(t *testing.T) {
	t.Run("создаёт тенант и владельца", func(t *testing.T) {
		var (
			storedTenant *model.Tenant
			storedMember *model.TenantMember
		)

		repo := &mockTenantRepository{
			createWithOwnerFunc: func(_ context.Context, tenant *model.Tenant, member *model.TenantMember) error {
				storedTenant = tenant
				storedMember = member

				return nil
			},
		}

		svc, events := newTestTenantService(t, repo, &mockUserRepository{}, &mockTenantSessionRepository{})
		actor := testSuperuserPrincipal(t)

		tenant, err := svc.Create(context.Background(), actor, model.CreateTenantInput{
			Name: "  Acme  ",
			Slug: "  ACME ",
			Metadata: map[string]string{
				"description": "Аренда",
				"location":    "Москва",
			},
		})
		if err != nil {
			t.Fatalf("Create() unexpected error = %v", err)
		}

		if tenant.Slug != "acme" || tenant.Name != "Acme" {
			t.Errorf("tenant = %q/%q, want Acme/acme", tenant.Name, tenant.Slug)
		}

		if storedTenant != tenant || storedMember == nil {
			t.Fatal("Create() не передал тенант и владельца в репозиторий")
		}

		if storedMember.UserID != actor.User.ID || !storedMember.IsOwner || storedMember.Role != model.RoleAdmin {
			t.Errorf("owner = %+v, want admin-владельца", storedMember)
		}

		if len(tenant.Metadata) == 0 {
			t.Error("Metadata = nil, want JSON настроек")
		}

		if len(events.events) != 1 || events.events[0] != EventTenantCreated {
			t.Errorf("events = %v, want [%s]", events.events, EventTenantCreated)
		}
	})

	t.Run("не глобальный администратор — запрещено", func(t *testing.T) {
		svc, _ := newTestTenantService(t, &mockTenantRepository{}, &mockUserRepository{}, &mockTenantSessionRepository{})

		_, err := svc.Create(context.Background(), testPrincipal(t, model.RoleAdmin), model.CreateTenantInput{
			Name: "Acme",
			Slug: "acme",
		})
		if !errors.Is(err, ErrForbidden) {
			t.Errorf("Create() error = %v, want %v", err, ErrForbidden)
		}
	})

	t.Run("невалидный slug", func(t *testing.T) {
		svc, _ := newTestTenantService(t, &mockTenantRepository{}, &mockUserRepository{}, &mockTenantSessionRepository{})

		_, err := svc.Create(context.Background(), testSuperuserPrincipal(t), model.CreateTenantInput{
			Name: "Acme",
			Slug: "bad slug",
		})
		if !errors.Is(err, model.ErrInvalidTenant) {
			t.Errorf("Create() error = %v, want %v", err, model.ErrInvalidTenant)
		}
	})
}

func TestTenantService_Get(t *testing.T) {
	t.Run("глобальный администратор читает любой", func(t *testing.T) {
		repo := &mockTenantRepository{
			getByIDFunc: func(_ context.Context, id string) (*model.Tenant, error) {
				return activeTenant(id), nil
			},
		}

		svc, _ := newTestTenantService(t, repo, &mockUserRepository{}, &mockTenantSessionRepository{})

		tenant, err := svc.Get(context.Background(), testSuperuserPrincipal(t), otherTenantID)
		if err != nil || tenant.ID != otherTenantID {
			t.Fatalf("Get() = %v, %v", tenant, err)
		}
	})

	t.Run("участник читает свой тенант", func(t *testing.T) {
		repo := &mockTenantRepository{
			getMemberFunc: func(_ context.Context, tenantID, userID string) (*model.TenantMember, error) {
				return &model.TenantMember{TenantID: tenantID, UserID: userID, Role: model.RoleClient}, nil
			},
			getByIDFunc: func(_ context.Context, id string) (*model.Tenant, error) {
				return activeTenant(id), nil
			},
		}

		svc, _ := newTestTenantService(t, repo, &mockUserRepository{}, &mockTenantSessionRepository{})
		actor := tenantActorIn(t, model.RoleClient, testTenantID)

		if _, err := svc.Get(context.Background(), actor, otherTenantID); err != nil {
			t.Fatalf("Get() unexpected error = %v", err)
		}
	})

	t.Run("чужой тенант — запрещено", func(t *testing.T) {
		svc, _ := newTestTenantService(t, &mockTenantRepository{}, &mockUserRepository{}, &mockTenantSessionRepository{})
		actor := tenantActorIn(t, model.RoleClient, testTenantID)

		if _, err := svc.Get(context.Background(), actor, otherTenantID); !errors.Is(err, ErrForbidden) {
			t.Errorf("Get() error = %v, want %v", err, ErrForbidden)
		}
	})

	t.Run("удалённый скрыт от участника", func(t *testing.T) {
		repo := &mockTenantRepository{
			getByIDFunc: func(_ context.Context, id string) (*model.Tenant, error) {
				return &model.Tenant{ID: id, Status: model.TenantStatusDeleted}, nil
			},
		}

		svc, _ := newTestTenantService(t, repo, &mockUserRepository{}, &mockTenantSessionRepository{})

		_, err := svc.Get(context.Background(), tenantActorIn(t, model.RoleClient, testTenantID), testTenantID)
		if !errors.Is(err, database.ErrNotFound) {
			t.Errorf("Get() error = %v, want %v", err, database.ErrNotFound)
		}
	})
}

func TestTenantService_List(t *testing.T) {
	t.Run("глобальный администратор — все", func(t *testing.T) {
		repo := &mockTenantRepository{
			listFunc: func(_ context.Context, limit, offset int) ([]*model.Tenant, error) {
				if limit != 2 || offset != 0 {
					t.Errorf("List() limit/offset = %d/%d, want 2/0", limit, offset)
				}

				return []*model.Tenant{activeTenant(testTenantID), activeTenant(otherTenantID)}, nil
			},
		}

		svc, _ := newTestTenantService(t, repo, &mockUserRepository{}, &mockTenantSessionRepository{})

		tenants, next, err := svc.List(context.Background(), testSuperuserPrincipal(t), model.Page{Limit: 1})
		if err != nil || len(tenants) != 1 || next == "" {
			t.Fatalf("List() = %d, %q, %v", len(tenants), next, err)
		}
	})

	t.Run("участник — только свои членства", func(t *testing.T) {
		repo := &mockTenantRepository{
			listMembershipsFunc: func(context.Context, string) ([]*model.TenantMembership, error) {
				return []*model.TenantMembership{
					{Tenant: *activeTenant(testTenantID)},
					{Tenant: *activeTenant(otherTenantID)},
				}, nil
			},
		}

		svc, _ := newTestTenantService(t, repo, &mockUserRepository{}, &mockTenantSessionRepository{})

		tenants, next, err := svc.List(context.Background(), testPrincipal(t, model.RoleClient), model.Page{Limit: 10, Offset: 1})
		if err != nil || len(tenants) != 1 || tenants[0].ID != otherTenantID || next != "" {
			t.Fatalf("List() = %v, %q, %v", tenants, next, err)
		}
	})
}

func TestTenantService_Delete(t *testing.T) {
	t.Run("администратор тенанта удаляет и отзывает сессии", func(t *testing.T) {
		var (
			updatedStatus model.TenantStatus
			revokedTenant string
		)

		repo := &mockTenantRepository{
			updateStatusFunc: func(_ context.Context, _ string, status model.TenantStatus, _ time.Time) error {
				updatedStatus = status

				return nil
			},
		}
		tokens := &mockTenantSessionRepository{
			revokeTenantFunc: func(_ context.Context, tenantID string, _ time.Time) (int64, error) {
				revokedTenant = tenantID

				return 1, nil
			},
		}

		svc, events := newTestTenantService(t, repo, &mockUserRepository{}, tokens)

		err := svc.Delete(context.Background(), tenantActorIn(t, model.RoleAdmin, testTenantID), testTenantID)
		if err != nil {
			t.Fatalf("Delete() unexpected error = %v", err)
		}

		if updatedStatus != model.TenantStatusDeleted || revokedTenant != testTenantID {
			t.Errorf("status/revoked = %q/%q", updatedStatus, revokedTenant)
		}

		if len(events.events) != 1 || events.events[0] != EventTenantStatusChanged {
			t.Errorf("events = %v", events.events)
		}
	})

	t.Run("клиент не может удалить", func(t *testing.T) {
		svc, _ := newTestTenantService(t, &mockTenantRepository{}, &mockUserRepository{}, &mockTenantSessionRepository{})

		err := svc.Delete(context.Background(), tenantActorIn(t, model.RoleClient, testTenantID), testTenantID)
		if !errors.Is(err, ErrForbidden) {
			t.Errorf("Delete() error = %v, want %v", err, ErrForbidden)
		}
	})

	t.Run("повторное удаление идемпотентно", func(t *testing.T) {
		repo := &mockTenantRepository{
			getByIDFunc: func(_ context.Context, id string) (*model.Tenant, error) {
				return &model.Tenant{ID: id, Status: model.TenantStatusDeleted}, nil
			},
		}

		svc, _ := newTestTenantService(t, repo, &mockUserRepository{}, &mockTenantSessionRepository{})

		if err := svc.Delete(context.Background(), tenantActorIn(t, model.RoleAdmin, testTenantID), testTenantID); err != nil {
			t.Fatalf("Delete() unexpected error = %v", err)
		}
	})
}

func TestTenantService_UpdateStatus(t *testing.T) {
	t.Run("приостановка отзывает сессии", func(t *testing.T) {
		var revoked string

		repo := &mockTenantRepository{
			getByIDFunc: func(_ context.Context, id string) (*model.Tenant, error) {
				return activeTenant(id), nil
			},
		}
		tokens := &mockTenantSessionRepository{
			revokeTenantFunc: func(_ context.Context, tenantID string, _ time.Time) (int64, error) {
				revoked = tenantID

				return 2, nil
			},
		}

		svc, _ := newTestTenantService(t, repo, &mockUserRepository{}, tokens)

		tenant, err := svc.UpdateStatus(
			context.Background(), testSuperuserPrincipal(t), testTenantID, model.TenantStatusSuspended,
		)
		if err != nil || tenant == nil {
			t.Fatalf("UpdateStatus() = %v, %v", tenant, err)
		}

		if revoked != testTenantID {
			t.Errorf("revoked = %q, want %q", revoked, testTenantID)
		}
	})

	t.Run("администратор тенанта не меняет статус", func(t *testing.T) {
		svc, _ := newTestTenantService(t, &mockTenantRepository{}, &mockUserRepository{}, &mockTenantSessionRepository{})

		_, err := svc.UpdateStatus(
			context.Background(),
			tenantActorIn(t, model.RoleAdmin, testTenantID),
			testTenantID,
			model.TenantStatusSuspended,
		)
		if !errors.Is(err, ErrForbidden) {
			t.Errorf("UpdateStatus() error = %v, want %v", err, ErrForbidden)
		}
	})

	t.Run("неизвестный статус", func(t *testing.T) {
		svc, _ := newTestTenantService(t, &mockTenantRepository{}, &mockUserRepository{}, &mockTenantSessionRepository{})

		_, err := svc.UpdateStatus(context.Background(), testSuperuserPrincipal(t), testTenantID, "wat")
		if !errors.Is(err, model.ErrInvalidTenant) {
			t.Errorf("UpdateStatus() error = %v, want %v", err, model.ErrInvalidTenant)
		}
	})
}

func TestTenantService_ListMembers(t *testing.T) {
	t.Run("staff тенанта видит участников", func(t *testing.T) {
		repo := &mockTenantRepository{
			listMembersFunc: func(_ context.Context, tenantID string, limit, offset int) ([]*model.TenantMemberView, error) {
				if tenantID != testTenantID || limit != 2 || offset != 0 {
					t.Errorf("ListMembers() args = %q, %d, %d", tenantID, limit, offset)
				}

				return []*model.TenantMemberView{
					{TenantMember: model.TenantMember{TenantID: tenantID, UserID: testUserID, Role: model.RoleClient}},
					{TenantMember: model.TenantMember{TenantID: tenantID, UserID: otherUserID, Role: model.RoleClient}},
				}, nil
			},
		}

		svc, _ := newTestTenantService(t, repo, &mockUserRepository{}, &mockTenantSessionRepository{})

		members, next, err := svc.ListMembers(
			context.Background(), tenantActorIn(t, model.RoleOperator, testTenantID), testTenantID, model.Page{Limit: 1},
		)
		if err != nil || len(members) != 1 || next == "" {
			t.Fatalf("ListMembers() = %d, %q, %v", len(members), next, err)
		}
	})

	t.Run("клиент не видит состав", func(t *testing.T) {
		svc, _ := newTestTenantService(t, &mockTenantRepository{}, &mockUserRepository{}, &mockTenantSessionRepository{})

		_, _, err := svc.ListMembers(
			context.Background(), tenantActorIn(t, model.RoleClient, testTenantID), testTenantID, model.Page{Limit: 10},
		)
		if !errors.Is(err, ErrForbidden) {
			t.Errorf("ListMembers() error = %v, want %v", err, ErrForbidden)
		}
	})
}

func TestTenantService_AddMember(t *testing.T) {
	t.Run("добавляет клиента", func(t *testing.T) {
		var added *model.TenantMember

		repo := &mockTenantRepository{
			addMemberFunc: func(_ context.Context, member *model.TenantMember) error {
				added = member

				return nil
			},
		}

		svc, events := newTestTenantService(t, repo, &mockUserRepository{
			getByIDFunc: func(_ context.Context, id string) (*model.User, error) {
				return storedUser(t, id), nil
			},
		}, &mockTenantSessionRepository{})

		view, err := svc.AddMember(
			context.Background(), tenantActorIn(t, model.RoleAdmin, testTenantID), testTenantID, otherUserID, model.RoleClient,
		)
		if err != nil || view == nil {
			t.Fatalf("AddMember() = %v, %v", view, err)
		}

		if added == nil || added.Role != model.RoleClient || added.IsOwner {
			t.Errorf("member = %+v", added)
		}

		if len(events.events) != 1 || events.events[0] != EventTenantMemberAdded {
			t.Errorf("events = %v", events.events)
		}
	})

	t.Run("невалидная роль", func(t *testing.T) {
		svc, _ := newTestTenantService(t, &mockTenantRepository{}, &mockUserRepository{}, &mockTenantSessionRepository{})

		_, err := svc.AddMember(
			context.Background(), tenantActorIn(t, model.RoleAdmin, testTenantID), testTenantID, otherUserID, "wat",
		)
		if !errors.Is(err, model.ErrInvalidRole) {
			t.Errorf("AddMember() error = %v, want %v", err, model.ErrInvalidRole)
		}
	})

	t.Run("не администратор — запрещено", func(t *testing.T) {
		svc, _ := newTestTenantService(t, &mockTenantRepository{}, &mockUserRepository{}, &mockTenantSessionRepository{})

		_, err := svc.AddMember(
			context.Background(), tenantActorIn(t, model.RoleOperator, testTenantID), testTenantID, otherUserID, model.RoleClient,
		)
		if !errors.Is(err, ErrForbidden) {
			t.Errorf("AddMember() error = %v, want %v", err, ErrForbidden)
		}
	})
}

func TestTenantService_RemoveMember(t *testing.T) {
	t.Run("исключает участника и отзывает его сессии", func(t *testing.T) {
		var revokedUser, revokedTenant string

		repo := &mockTenantRepository{
			getMemberFunc: func(_ context.Context, tenantID, userID string) (*model.TenantMember, error) {
				return &model.TenantMember{TenantID: tenantID, UserID: userID, Role: model.RoleClient}, nil
			},
		}
		tokens := &mockTenantSessionRepository{
			revokeMemberFunc: func(_ context.Context, userID, tenantID string, _ time.Time) (int64, error) {
				revokedUser, revokedTenant = userID, tenantID

				return 1, nil
			},
		}

		svc, events := newTestTenantService(t, repo, &mockUserRepository{}, tokens)

		err := svc.RemoveMember(
			context.Background(), tenantActorIn(t, model.RoleAdmin, testTenantID), testTenantID, otherUserID,
		)
		if err != nil {
			t.Fatalf("RemoveMember() unexpected error = %v", err)
		}

		if revokedUser != otherUserID || revokedTenant != testTenantID {
			t.Errorf("revoked = %q/%q", revokedUser, revokedTenant)
		}

		if len(events.events) != 1 || events.events[0] != EventTenantMemberRemoved {
			t.Errorf("events = %v", events.events)
		}
	})

	t.Run("владельца удалить нельзя", func(t *testing.T) {
		repo := &mockTenantRepository{
			getMemberFunc: func(_ context.Context, tenantID, userID string) (*model.TenantMember, error) {
				return &model.TenantMember{TenantID: tenantID, UserID: userID, Role: model.RoleAdmin, IsOwner: true}, nil
			},
		}

		svc, _ := newTestTenantService(t, repo, &mockUserRepository{}, &mockTenantSessionRepository{})

		err := svc.RemoveMember(
			context.Background(), tenantActorIn(t, model.RoleAdmin, testTenantID), testTenantID, otherUserID,
		)
		if !errors.Is(err, ErrForbidden) {
			t.Errorf("RemoveMember() error = %v, want %v", err, ErrForbidden)
		}
	})
}

func TestTenantService_ChangeMemberRole(t *testing.T) {
	t.Run("меняет роль и отзывает сессии", func(t *testing.T) {
		var revoked bool

		repo := &mockTenantRepository{
			getMemberFunc: func(_ context.Context, tenantID, userID string) (*model.TenantMember, error) {
				return &model.TenantMember{TenantID: tenantID, UserID: userID, Role: model.RoleClient}, nil
			},
			updateMemberRoleFunc: func(_ context.Context, tenantID, userID string, role model.Role) error {
				if role != model.RoleManager {
					t.Errorf("role = %q, want manager", role)
				}

				return nil
			},
		}
		tokens := &mockTenantSessionRepository{
			revokeMemberFunc: func(context.Context, string, string, time.Time) (int64, error) {
				revoked = true

				return 1, nil
			},
		}

		svc, events := newTestTenantService(t, repo, &mockUserRepository{}, tokens)

		view, err := svc.ChangeMemberRole(
			context.Background(),
			tenantActorIn(t, model.RoleAdmin, testTenantID),
			testTenantID,
			otherUserID,
			model.RoleManager,
		)
		if err != nil || view == nil {
			t.Fatalf("ChangeMemberRole() = %v, %v", view, err)
		}

		if !revoked {
			t.Error("ChangeMemberRole() не отозвал сессии участника")
		}

		if len(events.events) != 1 || events.events[0] != EventTenantMemberRoleChanged {
			t.Errorf("events = %v", events.events)
		}
	})

	t.Run("роль владельца меняется только передачей владения", func(t *testing.T) {
		repo := &mockTenantRepository{
			getMemberFunc: func(_ context.Context, tenantID, userID string) (*model.TenantMember, error) {
				return &model.TenantMember{TenantID: tenantID, UserID: userID, Role: model.RoleAdmin, IsOwner: true}, nil
			},
		}

		svc, _ := newTestTenantService(t, repo, &mockUserRepository{}, &mockTenantSessionRepository{})

		_, err := svc.ChangeMemberRole(
			context.Background(),
			tenantActorIn(t, model.RoleAdmin, testTenantID),
			testTenantID,
			otherUserID,
			model.RoleClient,
		)
		if !errors.Is(err, ErrForbidden) {
			t.Errorf("ChangeMemberRole() error = %v, want %v", err, ErrForbidden)
		}
	})
}

func TestTenantService_TransferOwnership(t *testing.T) {
	t.Run("владелец передаёт владение", func(t *testing.T) {
		var transferred string
		revoked := 0

		repo := &mockTenantRepository{
			getMemberFunc: func(_ context.Context, tenantID, userID string) (*model.TenantMember, error) {
				return &model.TenantMember{TenantID: tenantID, UserID: userID, Role: model.RoleAdmin, IsOwner: userID == testUserID}, nil
			},
			transferOwnershipFunc: func(_ context.Context, _ string, userID string) error {
				transferred = userID

				return nil
			},
		}
		tokens := &mockTenantSessionRepository{
			revokeMemberFunc: func(context.Context, string, string, time.Time) (int64, error) {
				revoked++

				return 1, nil
			},
		}

		svc, events := newTestTenantService(t, repo, &mockUserRepository{}, tokens)

		view, err := svc.TransferOwnership(
			context.Background(), tenantActorIn(t, model.RoleAdmin, testTenantID), testTenantID, otherUserID,
		)
		if err != nil || view == nil {
			t.Fatalf("TransferOwnership() = %v, %v", view, err)
		}

		if transferred != otherUserID || revoked != 2 {
			t.Errorf("transferred/revoked = %q/%d", transferred, revoked)
		}

		if len(events.events) != 1 || events.events[0] != EventTenantOwnershipTransferred {
			t.Errorf("events = %v", events.events)
		}
	})

	t.Run("администратор не владелец — запрещено", func(t *testing.T) {
		repo := &mockTenantRepository{
			getMemberFunc: func(_ context.Context, tenantID, userID string) (*model.TenantMember, error) {
				return &model.TenantMember{TenantID: tenantID, UserID: userID, Role: model.RoleAdmin}, nil
			},
		}

		svc, _ := newTestTenantService(t, repo, &mockUserRepository{}, &mockTenantSessionRepository{})

		_, err := svc.TransferOwnership(
			context.Background(), tenantActorIn(t, model.RoleAdmin, testTenantID), testTenantID, otherUserID,
		)
		if !errors.Is(err, ErrForbidden) {
			t.Errorf("TransferOwnership() error = %v, want %v", err, ErrForbidden)
		}
	})

	t.Run("глобальный администратор передаёт владение", func(t *testing.T) {
		repo := &mockTenantRepository{
			getMemberFunc: func(_ context.Context, tenantID, userID string) (*model.TenantMember, error) {
				return &model.TenantMember{TenantID: tenantID, UserID: userID, Role: model.RoleClient}, nil
			},
		}

		svc, _ := newTestTenantService(t, repo, &mockUserRepository{}, &mockTenantSessionRepository{})

		if _, err := svc.TransferOwnership(
			context.Background(), testSuperuserPrincipal(t), testTenantID, otherUserID,
		); err != nil {
			t.Fatalf("TransferOwnership() unexpected error = %v", err)
		}
	})
}
