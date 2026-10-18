package repository

import (
	"context"
	"testing"

	"github.com/botbooker/bb-core/internal/database"
)

// TestCatalogRepo_ListPublicObjects проверяет публичный каталог: только
// объекты своего тенанта и только не удалённые.
func TestCatalogRepo_ListPublicObjects(t *testing.T) {
	ctx := context.Background()
	conn := newSQLiteTestDB(t)

	tenantRepo := NewTenantRepo(ctx, conn, database.DialectSQLite)
	catalogRepo := NewCatalogRepo(ctx, conn)

	tenant := testTenant(t, tenantRepo, "catalog-public")

	userRepo := NewUserRepo(ctx, conn, database.DialectSQLite)
	client := testUser(t, "catalog-client@example.com", "+79990000051")
	if err := userRepo.Create(ctx, client); err != nil {
		t.Fatalf("UserRepo.Create() unexpected error = %v", err)
	}

	if err := insertBookingForClient(ctx, conn, tenant.ID, client.ID); err != nil {
		t.Fatalf("insertBookingForClient() unexpected error = %v", err)
	}

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

	if _, err := conn.ExecContext(ctx,
		"UPDATE rental_objects SET status_id = ? WHERE tenant_id = ?", deletedObjectStatusID, tenant.ID,
	); err != nil {
		t.Fatalf("update object status: %v", err)
	}

	objects, err = catalogRepo.ListPublicObjects(ctx, tenant.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListPublicObjects() unexpected error = %v", err)
	}

	if len(objects) != 0 {
		t.Errorf("ListPublicObjects() = %v, want пусто: удалённый объект не публикуется", objects)
	}

	if _, err := catalogRepo.ListPublicObjects(ctx, "not-a-uuid", 10, 0); err == nil {
		t.Error("ListPublicObjects() error = nil, want ошибку валидации")
	}
}
