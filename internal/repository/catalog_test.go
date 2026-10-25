package repository

import (
	"context"
	"testing"

	"github.com/botbooker/bb-core/internal/database"
)

func TestNewCatalogRepo(t *testing.T) {
	tests := []struct {
		name    string
		dialect string
	}{
		{name: "sqlite", dialect: database.DialectSQLite},
		{name: "postgres", dialect: database.DialectPostgres},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewCatalogRepo(context.Background(), nil, tt.dialect); got == nil {
				t.Error("NewCatalogRepo() returned nil")
			}
		})
	}
}

func TestCatalogRepoSQLite(t *testing.T) {
	runCatalogRepoScenario(t, newSQLiteTestDB(t), database.DialectSQLite)
}

// TestCatalogRepoPostgres проверяет каталог на PostgreSQL: там другие
// плейсхолдеры ($n вместо «?»), и без перебиндинга запрос падал
// синтаксической ошибкой, которую SQLite-тест не ловит.
func TestCatalogRepoPostgres(t *testing.T) {
	runCatalogRepoScenario(t, newPostgresTestDB(t), database.DialectPostgres)
}

// runCatalogRepoScenario — общий сценарий для обоих диалектов: публичный
// каталог отдаёт только объекты своего тенанта и только не удалённые.
func runCatalogRepoScenario(t *testing.T, conn database.DB, dialect string) {
	t.Helper()

	ctx := context.Background()

	tenantRepo := NewTenantRepo(ctx, conn, dialect)
	catalogRepo := NewCatalogRepo(ctx, conn, dialect)
	userRepo := NewUserRepo(ctx, conn, dialect)

	tenant := testTenant(t, tenantRepo, "catalog-public")

	client := testUser(t, "catalog-client@example.com", "+79990000051")
	if err := userRepo.Create(ctx, client); err != nil {
		t.Fatalf("UserRepo.Create() unexpected error = %v", err)
	}

	if err := insertBookingForClient(ctx, conn, dialect, tenant.ID, client.ID); err != nil {
		t.Fatalf("insertBookingForClient() unexpected error = %v", err)
	}

	t.Run("действующий объект виден", func(t *testing.T) {
		objects, err := catalogRepo.ListPublicObjects(ctx, tenant.ID, 10, 0)
		if err != nil {
			t.Fatalf("ListPublicObjects() unexpected error = %v", err)
		}

		if len(objects) != 1 {
			t.Fatalf("ListPublicObjects() = %d, want 1", len(objects))
		}

		if objects[0].Title != "Объект видимости" || objects[0].PriceMinor != 100000 || objects[0].Currency != "RUB" {
			t.Errorf("object = %+v", objects[0])
		}
	})

	t.Run("удалённый объект не публикуется", func(t *testing.T) {
		c := codec{dialect: dialect}

		if _, err := conn.ExecContext(ctx,
			c.rebind("UPDATE rental_objects SET status_id = ? WHERE tenant_id = ?"), deletedObjectStatusID, tenant.ID,
		); err != nil {
			t.Fatalf("update object status: %v", err)
		}

		objects, err := catalogRepo.ListPublicObjects(ctx, tenant.ID, 10, 0)
		if err != nil {
			t.Fatalf("ListPublicObjects() unexpected error = %v", err)
		}

		if len(objects) != 0 {
			t.Errorf("ListPublicObjects() = %v, want пусто: удалённый объект не публикуется", objects)
		}
	})

	if _, err := catalogRepo.ListPublicObjects(ctx, "not-a-uuid", 10, 0); err == nil {
		t.Error("ListPublicObjects() error = nil, want ошибку валидации")
	}
}
