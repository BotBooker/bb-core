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
