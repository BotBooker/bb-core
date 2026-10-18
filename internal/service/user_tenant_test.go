package service

import (
	"context"
	"errors"
	"testing"

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

	svc := NewUserService(context.Background(), &mockUserRepository{}, memberships, events)

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

func TestUserService_CreateRollsBackWithoutMembership(t *testing.T) {
	deleted := false

	repo := &mockUserRepository{
		deleteFunc: func(context.Context, string) error {
			deleted = true

			return nil
		},
	}
	memberships := &mockUserMembershipRepository{
		addMemberFunc: func(context.Context, *model.TenantMember) error {
			return database.ErrDuplicate
		},
	}

	svc := NewUserService(context.Background(), repo, memberships, &mockUserEventPublisher{})

	if _, err := svc.Create(context.Background(), testPrincipal(t, model.RoleAdmin), testCreateInput()); err == nil {
		t.Fatal("Create() error = nil, want ошибку членства")
	}

	if !deleted {
		t.Error("Create() не удалил пользователя после ошибки членства")
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

		svc := NewUserService(context.Background(), repo, &mockUserMembershipRepository{}, &mockUserEventPublisher{})

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

		svc := NewUserService(context.Background(), repo, &mockUserMembershipRepository{}, &mockUserEventPublisher{})

		users, next, err := svc.List(
			context.Background(), tenantActorIn(t, model.RoleOperator, testTenantID), model.Page{Limit: 10},
		)
		if err != nil || len(users) != 1 || next != "" {
			t.Fatalf("List() = %d, %q, %v", len(users), next, err)
		}
	})

	t.Run("клиент видит только себя", func(t *testing.T) {
		svc := NewUserService(
			context.Background(), &mockUserRepository{}, &mockUserMembershipRepository{}, &mockUserEventPublisher{},
		)
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

		svc := NewUserService(context.Background(), repo, &mockUserMembershipRepository{}, &mockUserEventPublisher{})

		user, err := svc.Get(
			context.Background(), tenantActorIn(t, model.RoleOperator, testTenantID), otherUserID,
		)
		if err != nil || user.ID != otherUserID {
			t.Fatalf("Get() = %v, %v", user, err)
		}
	})

	t.Run("клиент не читает чужой профиль", func(t *testing.T) {
		svc := NewUserService(
			context.Background(), &mockUserRepository{}, &mockUserMembershipRepository{}, &mockUserEventPublisher{},
		)

		_, err := svc.Get(
			context.Background(), tenantActorIn(t, model.RoleClient, testTenantID), otherUserID,
		)
		if !errors.Is(err, ErrForbidden) {
			t.Errorf("Get() error = %v, want %v", err, ErrForbidden)
		}
	})
}

func TestUserService_DeleteProtectsOwner(t *testing.T) {
	memberships := &mockUserMembershipRepository{
		getMemberFunc: func(_ context.Context, tenantID, userID string) (*model.TenantMember, error) {
			return &model.TenantMember{TenantID: tenantID, UserID: userID, Role: model.RoleAdmin, IsOwner: true}, nil
		},
	}

	svc := NewUserService(context.Background(), &mockUserRepository{}, memberships, &mockUserEventPublisher{})

	err := svc.Delete(context.Background(), tenantActorIn(t, model.RoleAdmin, testTenantID), otherUserID)
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("Delete() error = %v, want %v", err, ErrForbidden)
	}
}
