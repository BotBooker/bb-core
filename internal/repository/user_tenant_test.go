package repository

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/model"
)

// TestUserRepo_Visibility проверяет список и чтение пользователей в области
// видимости staff тенанта: сотрудники видны всегда, клиент — только при
// наличии брони в этом тенанте.
func TestUserRepo_Visibility(t *testing.T) {
	ctx := context.Background()
	conn := newSQLiteTestDB(t)

	userRepo := NewUserRepo(ctx, conn, database.DialectSQLite)
	tenantRepo := NewTenantRepo(ctx, conn, database.DialectSQLite)

	staff := testUser(t, "visibility-staff@example.com", "+79990000041")
	client := testUser(t, "visibility-client@example.com", "+79990000042")
	stranger := testUser(t, "visibility-stranger@example.com", "+79990000043")

	for _, user := range []*model.User{staff, client, stranger} {
		if err := userRepo.Create(ctx, user); err != nil {
			t.Fatalf("UserRepo.Create() unexpected error = %v", err)
		}
	}

	tenant := testTenant(t, tenantRepo, "visibility")

	now := time.Now().UTC().Truncate(time.Microsecond)

	if err := tenantRepo.AddMember(ctx, mustMember(t, tenant.ID, staff.ID, model.RoleOperator, now)); err != nil {
		t.Fatalf("AddMember(staff) unexpected error = %v", err)
	}

	if err := tenantRepo.AddMember(ctx, mustMember(t, tenant.ID, client.ID, model.RoleClient, now)); err != nil {
		t.Fatalf("AddMember(client) unexpected error = %v", err)
	}

	t.Run("ListAll возвращает активных", func(t *testing.T) {
		users, err := userRepo.ListAll(ctx, 10, 0)
		if err != nil {
			t.Fatalf("ListAll() unexpected error = %v", err)
		}

		if len(users) != 3 {
			t.Errorf("ListAll() = %d, want 3", len(users))
		}
	})

	t.Run("клиент без брони не виден staff", func(t *testing.T) {
		users, err := userRepo.ListVisible(ctx, tenant.ID, 10, 0)
		if err != nil {
			t.Fatalf("ListVisible() unexpected error = %v", err)
		}

		if len(users) != 1 || users[0].ID != staff.ID {
			t.Errorf("ListVisible() = %v, want только staff", users)
		}

		if _, err := userRepo.GetVisible(ctx, tenant.ID, client.ID); !errors.Is(err, database.ErrNotFound) {
			t.Errorf("GetVisible(client) error = %v, want %v", err, database.ErrNotFound)
		}

		if _, err := userRepo.GetVisible(ctx, tenant.ID, stranger.ID); !errors.Is(err, database.ErrNotFound) {
			t.Errorf("GetVisible(stranger) error = %v, want %v", err, database.ErrNotFound)
		}
	})

	if err := insertBookingForClient(ctx, conn, database.DialectSQLite, tenant.ID, client.ID); err != nil {
		t.Fatalf("insertBookingForClient() unexpected error = %v", err)
	}

	t.Run("клиент с бронью виден staff", func(t *testing.T) {
		users, err := userRepo.ListVisible(ctx, tenant.ID, 10, 0)
		if err != nil {
			t.Fatalf("ListVisible() unexpected error = %v", err)
		}

		if len(users) != 2 {
			t.Errorf("ListVisible() = %d, want 2", len(users))
		}

		got, err := userRepo.GetVisible(ctx, tenant.ID, client.ID)
		if err != nil || got.ID != client.ID {
			t.Fatalf("GetVisible() = %v, %v", got, err)
		}
	})

	t.Run("невалидные идентификаторы", func(t *testing.T) {
		if _, err := userRepo.ListVisible(ctx, "not-a-uuid", 10, 0); err == nil {
			t.Error("ListVisible() error = nil, want ошибку валидации")
		}

		if _, err := userRepo.GetVisible(ctx, tenant.ID, "not-a-uuid"); err == nil {
			t.Error("GetVisible() error = nil, want ошибку валидации")
		}
	})
}

// insertBookingForClient создаёт объект и бронь: основание видимости клиента.
//
// Запросы пишутся с «?» и перебинживаются кодеком диалекта: на PostgreSQL «?»
// не плейсхолдер, и вставка raw-строкой падала бы синтаксической ошибкой.
func insertBookingForClient(
	ctx context.Context,
	conn database.DB,
	dialect, tenantID, clientID string,
) error {
	c := codec{dialect: dialect}

	if _, err := conn.ExecContext(ctx,
		c.rebind("INSERT INTO type_objects (title) VALUES (?)"), "visibility-flat"); err != nil {
		return fmt.Errorf("insert type object: %w", err)
	}

	var typeID int
	if err := conn.QueryRowContext(ctx,
		c.rebind("SELECT id FROM type_objects WHERE title = ?"), "visibility-flat").Scan(&typeID); err != nil {
		return fmt.Errorf("select type object: %w", err)
	}

	const objectID = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"

	now := time.Now().UTC()

	if _, err := conn.ExecContext(ctx,
		c.rebind("INSERT INTO rental_objects (id, type, title, price_minor, currency, min_duration_minutes,"+
			" status_id, created_at, updated_at, tenant_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"),
		objectID, typeID, "Объект видимости", 100000, "RUB", 60, 1,
		c.timeArg(now), c.timeArg(now), tenantID,
	); err != nil {
		return fmt.Errorf("insert object: %w", err)
	}

	if _, err := conn.ExecContext(ctx,
		c.rebind("INSERT INTO bookings (id, object_id, client_id, slot_start, slot_end, created_at, updated_at, tenant_id)"+
			" VALUES (?, ?, ?, ?, ?, ?, ?, ?)"),
		"bbbbbbbb-cccc-4ddd-8eee-ffffffffffff", objectID, clientID,
		c.timeArg(now.Add(time.Hour)), c.timeArg(now.Add(2*time.Hour)),
		c.timeArg(now), c.timeArg(now), tenantID,
	); err != nil {
		return fmt.Errorf("insert booking: %w", err)
	}

	return nil
}
