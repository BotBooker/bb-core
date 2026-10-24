package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/model"
)

// testTenant собирает и сохраняет тенант с заданным slug.
//
// Slug уникален (tenants_slug_idx), поэтому разные тенанты в одном тесте
// должны получать разные slug. Время усечено до микросекунд — предел точности
// TIMESTAMPTZ: с ним результат одинаков у обоих диалектов.
func testTenant(t *testing.T, repo TenantRepo, slug string) *model.Tenant {
	t.Helper()

	tenant, err := model.NewTenant("Тенант "+slug, slug, time.Now().UTC().Truncate(time.Microsecond))
	if err != nil {
		t.Fatalf("model.NewTenant() unexpected error = %v", err)
	}

	if err := repo.Create(context.Background(), tenant); err != nil {
		t.Fatalf("TenantRepo.Create() unexpected error = %v", err)
	}

	return tenant
}

// assertTenant сравнивает прочитанный тенант с ожидаемым.
func assertTenant(t *testing.T, want, got *model.Tenant) {
	t.Helper()

	if got == nil {
		t.Fatal("тенант не прочитан, want запись")
	}

	if got.ID != want.ID {
		t.Errorf("ID = %q, want %q", got.ID, want.ID)
	}
	if got.Name != want.Name {
		t.Errorf("Name = %q, want %q", got.Name, want.Name)
	}
	if got.Slug != want.Slug {
		t.Errorf("Slug = %q, want %q", got.Slug, want.Slug)
	}
	if got.Status != want.Status {
		t.Errorf("Status = %q, want %q", got.Status, want.Status)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) || got.CreatedAt.Location() != time.UTC {
		t.Errorf("CreatedAt = %v, want %v (UTC)", got.CreatedAt, want.CreatedAt)
	}
	if !got.UpdatedAt.Equal(want.UpdatedAt) || got.UpdatedAt.Location() != time.UTC {
		t.Errorf("UpdatedAt = %v, want %v (UTC)", got.UpdatedAt, want.UpdatedAt)
	}
}

func TestNewTenantRepo(t *testing.T) {
	tests := []struct {
		name    string
		dialect string
	}{
		{name: "sqlite", dialect: database.DialectSQLite},
		{name: "postgres", dialect: database.DialectPostgres},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewTenantRepo(context.Background(), nil, tt.dialect); got == nil {
				t.Error("NewTenantRepo() returned nil")
			}
		})
	}
}

func TestTenantRepoSQLite(t *testing.T) {
	runTenantRepoScenario(t, newSQLiteTestDB(t), database.DialectSQLite)
}

func TestTenantRepoPostgres(t *testing.T) {
	runTenantRepoScenario(t, newPostgresTestDB(t), database.DialectPostgres)
}

// runTenantRepoScenario — общий сценарий для обоих диалектов.
func runTenantRepoScenario(t *testing.T, conn database.DB, dialect string) {
	t.Helper()

	ctx := context.Background()

	userRepo := NewUserRepo(context.Background(), conn, dialect)
	tenantRepo := NewTenantRepo(context.Background(), conn, dialect)

	user := testUser(t, "tenant-owner@example.com", "+79990000020")
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("UserRepo.Create() unexpected error = %v", err)
	}

	t.Run("создание и чтение", func(t *testing.T) {
		tenant := testTenant(t, tenantRepo, "romashka")

		byID, err := tenantRepo.GetByID(ctx, tenant.ID)
		if err != nil {
			t.Fatalf("GetByID() unexpected error = %v", err)
		}

		assertTenant(t, tenant, byID)

		bySlug, err := tenantRepo.GetBySlug(ctx, "romashka")
		if err != nil {
			t.Fatalf("GetBySlug() unexpected error = %v", err)
		}

		assertTenant(t, tenant, bySlug)
	})

	t.Run("метаданные хранятся как JSON", func(t *testing.T) {
		tenant, err := model.NewTenant("Тенант Мета", "meta", time.Now().UTC().Truncate(time.Microsecond))
		if err != nil {
			t.Fatalf("model.NewTenant() unexpected error = %v", err)
		}

		tenant.Metadata = []byte(`{"plan":"pro"}`)

		if err := tenantRepo.Create(ctx, tenant); err != nil {
			t.Fatalf("Create() unexpected error = %v", err)
		}

		got, err := tenantRepo.GetByID(ctx, tenant.ID)
		if err != nil {
			t.Fatalf("GetByID() unexpected error = %v", err)
		}

		assertJSONEqual(t, got.Metadata, `{"plan":"pro"}`)
	})

	t.Run("повторный slug — ErrDuplicate", func(t *testing.T) {
		tenant, err := model.NewTenant("Ромашка", "romashka", time.Now().UTC().Truncate(time.Microsecond))
		if err != nil {
			t.Fatalf("model.NewTenant() unexpected error = %v", err)
		}

		if err := tenantRepo.Create(ctx, tenant); !errors.Is(err, database.ErrDuplicate) {
			t.Errorf("Create() error = %v, want %v", err, database.ErrDuplicate)
		}
	})

	t.Run("неизвестный тенант — ErrNotFound", func(t *testing.T) {
		const unknown = "3f0c8f0e-9f60-4e0e-9d5f-1f6f1f6f1f6f"

		if _, err := tenantRepo.GetByID(ctx, unknown); !errors.Is(err, database.ErrNotFound) {
			t.Errorf("GetByID() error = %v, want %v", err, database.ErrNotFound)
		}

		if _, err := tenantRepo.GetBySlug(ctx, "nobody"); !errors.Is(err, database.ErrNotFound) {
			t.Errorf("GetBySlug() error = %v, want %v", err, database.ErrNotFound)
		}
	})

	t.Run("членство", func(t *testing.T) {
		tenant := testTenant(t, tenantRepo, "membership")

		member, err := model.NewTenantMember(tenant.ID, user.ID, model.RoleAdmin, true,
			time.Now().UTC().Truncate(time.Microsecond))
		if err != nil {
			t.Fatalf("model.NewTenantMember() unexpected error = %v", err)
		}

		if err := tenantRepo.AddMember(ctx, member); err != nil {
			t.Fatalf("AddMember() unexpected error = %v", err)
		}

		got, err := tenantRepo.GetMember(ctx, tenant.ID, user.ID)
		if err != nil {
			t.Fatalf("GetMember() unexpected error = %v", err)
		}

		if got.Role != member.Role {
			t.Errorf("Role = %q, want %q", got.Role, member.Role)
		}
		if !got.IsOwner {
			t.Error("IsOwner = false, want true")
		}
		if !got.JoinedAt.Equal(member.JoinedAt) {
			t.Errorf("JoinedAt = %v, want %v", got.JoinedAt, member.JoinedAt)
		}

		if err := tenantRepo.AddMember(ctx, member); !errors.Is(err, database.ErrDuplicate) {
			t.Errorf("повторный AddMember() error = %v, want %v", err, database.ErrDuplicate)
		}
	})
}

// TestTenantRepo_MemberOfUser проверяет выбор тенанта для сессии: берётся самое
// раннее членство среди действующих тенантов, а членство в приостановленном
// тенанте сессии не даёт (user_tokens.tenant_id ссылается на tenants).
func TestTenantRepo_MemberOfUser(t *testing.T) {
	ctx := context.Background()

	conn := newSQLiteTestDB(t)
	userRepo := NewUserRepo(context.Background(), conn, database.DialectSQLite)
	tenantRepo := NewTenantRepo(context.Background(), conn, database.DialectSQLite)

	user := testUser(t, "member-of@example.com", "+79990000021")
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("UserRepo.Create() unexpected error = %v", err)
	}

	t.Run("нет членства — ErrNotFound", func(t *testing.T) {
		if _, err := tenantRepo.MemberOfUser(ctx, user.ID); !errors.Is(err, database.ErrNotFound) {
			t.Errorf("MemberOfUser() error = %v, want %v", err, database.ErrNotFound)
		}
	})

	t.Run("берётся самое раннее членство среди действующих", func(t *testing.T) {
		earlier := testTenant(t, tenantRepo, "member-earlier")
		later := testTenant(t, tenantRepo, "member-later")

		joinedAt := time.Now().UTC().Truncate(time.Microsecond)

		lateMember, err := model.NewTenantMember(later.ID, user.ID, model.RoleClient, false, joinedAt)
		if err != nil {
			t.Fatalf("model.NewTenantMember() unexpected error = %v", err)
		}

		earlyMember, err := model.NewTenantMember(earlier.ID, user.ID, model.RoleManager, false, joinedAt.Add(-time.Hour))
		if err != nil {
			t.Fatalf("model.NewTenantMember() unexpected error = %v", err)
		}

		for _, member := range []*model.TenantMember{lateMember, earlyMember} {
			if err := tenantRepo.AddMember(ctx, member); err != nil {
				t.Fatalf("AddMember() unexpected error = %v", err)
			}
		}

		got, err := tenantRepo.MemberOfUser(ctx, user.ID)
		if err != nil {
			t.Fatalf("MemberOfUser() unexpected error = %v", err)
		}

		if got.TenantID != earlier.ID {
			t.Errorf("TenantID = %q, want %q (самое раннее членство)", got.TenantID, earlier.ID)
		}
		// Роль — атрибут тенанта: в этом тенанте она своя.
		if got.Role != model.RoleManager {
			t.Errorf("Role = %q, want %q", got.Role, model.RoleManager)
		}

		if _, err := tenantRepo.GetMember(ctx, later.ID, user.ID); err != nil {
			t.Errorf("членство в другом тенанте unexpected error = %v", err)
		}
	})

	t.Run("членство в неизвестном тенанте — ErrNotFound", func(t *testing.T) {
		const (
			unknownTenant = "3f0c8f0e-9f60-4e0e-9d5f-1f6f1f6f1f6f"
			unknownUser   = "4f0c8f0e-9f60-4e0e-9d5f-1f6f1f6f1f6f"
		)

		if _, err := tenantRepo.GetMember(ctx, unknownTenant, unknownUser); !errors.Is(err, database.ErrNotFound) {
			t.Errorf("GetMember() error = %v, want %v", err, database.ErrNotFound)
		}
	})
}

// TestTenantRepoManagementSQLite проверяет tenant-scoped операции управления на
// SQLite: владелец, участники, роли, передача владения и статус.
func TestTenantRepoManagementSQLite(t *testing.T) {
	runTenantManagementScenario(t, newSQLiteTestDB(t), database.DialectSQLite)
}

// TestTenantRepoManagementPostgres — тот же сценарий на PostgreSQL: у диалекта
// другие плейсхолдеры и типы времени, а фильтр «пользователь жив» проверяется
// в JOIN, который на обоих диалектах должен вести себя одинаково.
func TestTenantRepoManagementPostgres(t *testing.T) {
	runTenantManagementScenario(t, newPostgresTestDB(t), database.DialectPostgres)
}

// runTenantManagementScenario — общий сценарий управления участниками для обоих
// диалектов.
func runTenantManagementScenario(t *testing.T, conn database.DB, dialect string) {
	t.Helper()

	ctx := context.Background()

	userRepo := NewUserRepo(ctx, conn, dialect)
	tenantRepo := NewTenantRepo(ctx, conn, dialect)

	owner := testUser(t, "tenant-mgmt-owner@example.com", "+79990000031")
	memberUser := testUser(t, "tenant-mgmt-member@example.com", "+79990000032")

	for _, user := range []*model.User{owner, memberUser} {
		if err := userRepo.Create(ctx, user); err != nil {
			t.Fatalf("UserRepo.Create() unexpected error = %v", err)
		}
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	tenant, err := model.NewTenant("Управление", "management", now)
	if err != nil {
		t.Fatalf("model.NewTenant() unexpected error = %v", err)
	}

	ownerMember, err := model.NewTenantMember(tenant.ID, owner.ID, model.RoleAdmin, true, now)
	if err != nil {
		t.Fatalf("model.NewTenantMember() unexpected error = %v", err)
	}

	if err := tenantRepo.CreateWithOwner(ctx, tenant, ownerMember); err != nil {
		t.Fatalf("CreateWithOwner() unexpected error = %v", err)
	}

	t.Run("владелец и список действующих", func(t *testing.T) {
		active, err := tenantRepo.ListActive(ctx, 10, 0)
		if err != nil {
			t.Fatalf("ListActive() unexpected error = %v", err)
		}

		if len(active) != 1 || active[0].ID != tenant.ID {
			t.Errorf("ListActive() = %v", active)
		}

		member, err := tenantRepo.GetMember(ctx, tenant.ID, owner.ID)
		if err != nil || !member.IsOwner || member.Role != model.RoleAdmin {
			t.Fatalf("GetMember() = %+v, %v", member, err)
		}
	})

	if err := tenantRepo.AddMember(ctx, mustMember(t, tenant.ID, memberUser.ID, model.RoleClient, now)); err != nil {
		t.Fatalf("AddMember() unexpected error = %v", err)
	}

	t.Run("список участников и представление", func(t *testing.T) {
		members, err := tenantRepo.ListMembers(ctx, tenant.ID, 10, 0)
		if err != nil {
			t.Fatalf("ListMembers() unexpected error = %v", err)
		}

		if len(members) != 2 {
			t.Fatalf("ListMembers() = %d, want 2", len(members))
		}

		view, err := tenantRepo.GetMemberView(ctx, tenant.ID, memberUser.ID)
		if err != nil {
			t.Fatalf("GetMemberView() unexpected error = %v", err)
		}

		if view.Email != memberUser.Email || view.FirstName != memberUser.FirstName {
			t.Errorf("view = %+v", view)
		}

		memberships, err := tenantRepo.ListMemberships(ctx, memberUser.ID)
		if err != nil || len(memberships) != 1 || memberships[0].Tenant.ID != tenant.ID {
			t.Fatalf("ListMemberships() = %v, %v", memberships, err)
		}

		bySlug, err := tenantRepo.GetMemberBySlug(ctx, "management", memberUser.ID)
		if err != nil || bySlug.Member.Role != model.RoleClient {
			t.Fatalf("GetMemberBySlug() = %+v, %v", bySlug, err)
		}
	})

	t.Run("смена роли и исключение", func(t *testing.T) {
		if err := tenantRepo.UpdateMemberRole(ctx, tenant.ID, memberUser.ID, model.RoleManager); err != nil {
			t.Fatalf("UpdateMemberRole() unexpected error = %v", err)
		}

		member, err := tenantRepo.GetMember(ctx, tenant.ID, memberUser.ID)
		if err != nil || member.Role != model.RoleManager {
			t.Fatalf("GetMember() = %+v, %v", member, err)
		}

		if err := tenantRepo.RemoveMember(ctx, tenant.ID, memberUser.ID); err != nil {
			t.Fatalf("RemoveMember() unexpected error = %v", err)
		}

		if _, err := tenantRepo.GetMember(ctx, tenant.ID, memberUser.ID); !errors.Is(err, database.ErrNotFound) {
			t.Errorf("GetMember() error = %v, want %v", err, database.ErrNotFound)
		}

		if err := tenantRepo.AddMember(ctx, mustMember(t, tenant.ID, memberUser.ID, model.RoleClient, now)); err != nil {
			t.Fatalf("AddMember() unexpected error = %v", err)
		}
	})

	t.Run("передача владения", func(t *testing.T) {
		if err := tenantRepo.TransferOwnership(ctx, tenant.ID, memberUser.ID); err != nil {
			t.Fatalf("TransferOwnership() unexpected error = %v", err)
		}

		previous, err := tenantRepo.GetMember(ctx, tenant.ID, owner.ID)
		if err != nil || previous.IsOwner {
			t.Fatalf("previous owner = %+v, %v", previous, err)
		}

		current, err := tenantRepo.GetMember(ctx, tenant.ID, memberUser.ID)
		if err != nil || !current.IsOwner {
			t.Fatalf("current owner = %+v, %v", current, err)
		}
	})

	t.Run("удалённый пользователь не виден в участниках", func(t *testing.T) {
		deleted := testUser(t, "tenant-deleted@example.com", "+79990000030")
		if err := userRepo.Create(ctx, deleted); err != nil {
			t.Fatalf("UserRepo.Create() unexpected error = %v", err)
		}

		if err := tenantRepo.AddMember(ctx, mustMember(t, tenant.ID, deleted.ID, model.RoleClient, now)); err != nil {
			t.Fatalf("AddMember() unexpected error = %v", err)
		}

		before, err := tenantRepo.ListMembers(ctx, tenant.ID, 10, 0)
		if err != nil {
			t.Fatalf("ListMembers() unexpected error = %v", err)
		}

		if len(before) != 3 {
			t.Fatalf("ListMembers() = %d, want 3 до удаления", len(before))
		}

		if err := userRepo.Delete(ctx, deleted.ID); err != nil {
			t.Fatalf("UserRepo.Delete() unexpected error = %v", err)
		}

		after, err := tenantRepo.ListMembers(ctx, tenant.ID, 10, 0)
		if err != nil {
			t.Fatalf("ListMembers() unexpected error = %v", err)
		}

		if len(after) != 2 {
			t.Errorf("ListMembers() = %d, want 2: удалённый пользователь не показывается", len(after))
		}

		for _, member := range after {
			if member.UserID == deleted.ID {
				t.Errorf("ListMembers() содержит удалённого пользователя %q", deleted.ID)
			}
		}

		if _, err := tenantRepo.GetMemberView(ctx, tenant.ID, deleted.ID); !errors.Is(err, database.ErrNotFound) {
			t.Errorf("GetMemberView() error = %v, want %v", err, database.ErrNotFound)
		}
	})

	t.Run("приостановка убирает из действующих", func(t *testing.T) {
		if err := tenantRepo.UpdateStatus(ctx, tenant.ID, model.TenantStatusSuspended, now); err != nil {
			t.Fatalf("UpdateStatus() unexpected error = %v", err)
		}

		active, err := tenantRepo.ListActive(ctx, 10, 0)
		if err != nil {
			t.Fatalf("ListActive() unexpected error = %v", err)
		}

		if len(active) != 0 {
			t.Errorf("ListActive() = %v, want пусто", active)
		}

		got, err := tenantRepo.GetByID(ctx, tenant.ID)
		if err != nil || got.Status != model.TenantStatusSuspended {
			t.Fatalf("GetByID() = %+v, %v", got, err)
		}
	})
}

// mustMember создаёт членство или роняет тест: вспомогательная функция для
// сценариев, где невалидные данные не являются предметом проверки.
func mustMember(t *testing.T, tenantID, userID string, role model.Role, now time.Time) *model.TenantMember {
	t.Helper()

	member, err := model.NewTenantMember(tenantID, userID, role, false, now)
	if err != nil {
		t.Fatalf("model.NewTenantMember() unexpected error = %v", err)
	}

	return member
}
