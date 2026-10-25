package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/model"
)

func TestUserService_CreateAddsTenantMembership(t *testing.T) {
	var added *model.TenantMember

	memberships := &mockUserMembershipRepository{
		addMemberFunc: func(_ context.Context, member *model.TenantMember) error {
			added = member

			return nil
		},
	}
	events := &mockUserEventPublisher{}

	svc := newUserService(&mockUserRepository{}, memberships, events)

	user, err := svc.Create(context.Background(), tenantActorIn(t, model.RoleOperator, testTenantID), testCreateInput())
	if err != nil {
		t.Fatalf("Create() unexpected error = %v", err)
	}

	if added == nil || added.UserID != user.ID || added.TenantID != testTenantID || added.Role != model.RoleClient {
		t.Errorf("membership = %+v", added)
	}

	if len(events.events) != 1 || events.events[0] != EventUserCreated {
		t.Errorf("events = %v", events.events)
	}
}

// TestUserService_CreateFailsWithoutMembership проверяет, что ошибка членства
// отменяет создание пользователя.
//
// Откат делает UnitOfWork: обе вставки идут в одной транзакции, поэтому
// компенсирующего удаления в сервисе нет — иначе на ошибке транзакции оно
// выполнялось бы повторно. Здесь проверяется, что ошибка пробрасывается и
// событие не публикуется.
func TestUserService_CreateFailsWithoutMembership(t *testing.T) {
	created := false

	repo := &mockUserRepository{
		createFunc: func(context.Context, *model.User) error {
			created = true

			return nil
		},
	}
	memberships := &mockUserMembershipRepository{
		addMemberFunc: func(context.Context, *model.TenantMember) error {
			return database.ErrDuplicate
		},
	}
	events := &mockUserEventPublisher{}

	svc := newUserService(repo, memberships, events)

	_, err := svc.Create(context.Background(), testPrincipal(t, model.RoleAdmin), testCreateInput())
	if !errors.Is(err, database.ErrDuplicate) {
		t.Fatalf("Create() error = %v, want %v", err, database.ErrDuplicate)
	}

	if !created {
		t.Error("Create() не дошёл до вставки пользователя")
	}

	if len(events.events) != 0 {
		t.Errorf("события = %v, want пусто: транзакция не завершилась", events.events)
	}
}

func TestUserService_ListVisibility(t *testing.T) {
	t.Run("глобальный администратор — все", func(t *testing.T) {
		repo := &mockUserRepository{
			listAllFunc: func(_ context.Context, limit, offset int) ([]*model.User, error) {
				if limit != 2 || offset != 0 {
					t.Errorf("ListAll() = %d/%d", limit, offset)
				}

				return []*model.User{storedUser(t, testUserID), storedUser(t, otherUserID)}, nil
			},
		}

		svc := newUserService(repo, &mockUserMembershipRepository{}, &mockUserEventPublisher{})

		users, next, err := svc.List(context.Background(), testSuperuserPrincipal(t), model.Page{Limit: 1})
		if err != nil || len(users) != 1 || next == "" {
			t.Fatalf("List() = %d, %q, %v", len(users), next, err)
		}
	})

	t.Run("staff — только видимые в тенанте", func(t *testing.T) {
		repo := &mockUserRepository{
			listVisibleFunc: func(_ context.Context, tenantID string, limit, offset int) ([]*model.User, error) {
				if tenantID != testTenantID {
					t.Errorf("ListVisible() tenant = %q", tenantID)
				}

				return []*model.User{storedUser(t, otherUserID)}, nil
			},
		}

		svc := newUserService(repo, &mockUserMembershipRepository{}, &mockUserEventPublisher{})

		users, next, err := svc.List(
			context.Background(), tenantActorIn(t, model.RoleOperator, testTenantID), model.Page{Limit: 10},
		)
		if err != nil || len(users) != 1 || next != "" {
			t.Fatalf("List() = %d, %q, %v", len(users), next, err)
		}
	})

	t.Run("клиент видит только себя", func(t *testing.T) {
		svc := newUserService(&mockUserRepository{}, &mockUserMembershipRepository{}, &mockUserEventPublisher{})
		actor := tenantActorIn(t, model.RoleClient, testTenantID)

		users, next, err := svc.List(context.Background(), actor, model.Page{Limit: 10})
		if err != nil || len(users) != 1 || users[0].ID != actor.User.ID || next != "" {
			t.Fatalf("List() = %v, %q, %v", users, next, err)
		}

		users, _, err = svc.List(context.Background(), actor, model.Page{Limit: 10, Offset: 1})
		if err != nil || len(users) != 0 {
			t.Fatalf("List(offset) = %v, %v", users, err)
		}
	})
}

func TestUserService_GetVisibility(t *testing.T) {
	t.Run("staff читает через GetVisible", func(t *testing.T) {
		repo := &mockUserRepository{
			getVisibleFunc: func(_ context.Context, tenantID, id string) (*model.User, error) {
				if tenantID != testTenantID || id != otherUserID {
					t.Errorf("GetVisible() = %q/%q", tenantID, id)
				}

				return storedUser(t, id), nil
			},
		}

		svc := newUserService(repo, &mockUserMembershipRepository{}, &mockUserEventPublisher{})

		user, err := svc.Get(
			context.Background(), tenantActorIn(t, model.RoleOperator, testTenantID), otherUserID,
		)
		if err != nil || user.ID != otherUserID {
			t.Fatalf("Get() = %v, %v", user, err)
		}
	})

	t.Run("клиент не читает чужой профиль", func(t *testing.T) {
		svc := newUserService(&mockUserRepository{}, &mockUserMembershipRepository{}, &mockUserEventPublisher{})

		_, err := svc.Get(
			context.Background(), tenantActorIn(t, model.RoleClient, testTenantID), otherUserID,
		)
		if !errors.Is(err, ErrForbidden) {
			t.Errorf("Get() error = %v, want %v", err, ErrForbidden)
		}
	})
}

// TestUserService_DeleteRequiresGlobalAdmin проверяет границу удаления аккаунта:
// профиль общий для всех тенантов, поэтому администратор тенанта его не удаляет
// (исключение участника — TenantService.RemoveMember), а глобальный оператор —
// удаляет, закрывая доступ: учётные данные деактивируются, сессии отзываются.
func TestUserService_DeleteRequiresGlobalAdmin(t *testing.T) {
	t.Run("администратор тенанта не удаляет аккаунт", func(t *testing.T) {
		deleteCalled := false

		repo := &mockUserRepository{deleteFunc: func(context.Context, string) error {
			deleteCalled = true

			return nil
		}}

		svc := newUserService(repo, &mockUserMembershipRepository{}, &mockUserEventPublisher{})

		err := svc.Delete(context.Background(), tenantActorIn(t, model.RoleAdmin, testTenantID), otherUserID)
		if !errors.Is(err, ErrForbidden) {
			t.Errorf("Delete() error = %v, want %v", err, ErrForbidden)
		}

		if deleteCalled {
			t.Error("Delete() удалил аккаунт без прав глобального администратора")
		}
	})

	t.Run("глобальный оператор удаляет и закрывает доступ", func(t *testing.T) {
		var (
			deletedID     string
			deactivatedID string
			revokedID     string
		)

		users := &mockUserRepository{deleteFunc: func(_ context.Context, id string) error {
			deletedID = id

			return nil
		}}
		credentials := &mockAuthCredentialRepository{deactivateFunc: func(_ context.Context, userID string, _ time.Time) error {
			deactivatedID = userID

			return nil
		}}
		tokens := &mockAuthTokenRepository{revokeByUserFunc: func(_ context.Context, userID string, _ time.Time) (int64, error) {
			revokedID = userID

			return 1, nil
		}}
		events := &mockUserEventPublisher{}

		svc := newUserServiceWith(users, &mockUserMembershipRepository{}, credentials, tokens, events)

		if err := svc.Delete(context.Background(), testGlobalOperatorPrincipal(t), otherUserID); err != nil {
			t.Fatalf("Delete() unexpected error = %v", err)
		}

		if deletedID != otherUserID {
			t.Errorf("Delete() id = %q, want %q", deletedID, otherUserID)
		}

		if deactivatedID != otherUserID {
			t.Errorf("DeactivateByUser() id = %q, want %q", deactivatedID, otherUserID)
		}

		if revokedID != otherUserID {
			t.Errorf("RevokeByUser() id = %q, want %q", revokedID, otherUserID)
		}

		if len(events.events) != 1 || events.events[0] != EventUserDeleted {
			t.Errorf("события = %v, want [%s]", events.events, EventUserDeleted)
		}
	})
}
