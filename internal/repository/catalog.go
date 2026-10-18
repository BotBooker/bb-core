package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/model"
	"github.com/botbooker/bb-core/pkg/observability"
)

// CatalogDB — интерфейс базы данных, который нужен CatalogRepo.
type CatalogDB interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// CatalogRepo — репозиторий публичного каталога объектов.
//
// Это read-only слой для публичных ручек: он возвращает только поля, которые
// разрешены к публикации (ADR 0001 §8), и не отдаёт внутренние идентификаторы,
// владельца и служебные признаки.
type CatalogRepo interface {
	// ListPublicObjects возвращает опубликованные объекты тенанта с пагинацией.
	// Удалённые объекты (status_id = 4) не попадают в каталог.
	ListPublicObjects(ctx context.Context, tenantID string, limit, offset int) ([]*model.PublicObject, error)
}

// deletedObjectStatusID — статус объекта «удалён» (ERDB 2.9, seed миграции
// 00001_foundation). Мягко удалённые объекты публично не показываются.
const deletedObjectStatusID = 4

// catalogRepo — конкретная реализация, скрыта от внешних пакетов.
type catalogRepo struct {
	db CatalogDB
}

// NewCatalogRepo создаёт репозиторий публичного каталога.
func NewCatalogRepo(ctx context.Context, db CatalogDB) CatalogRepo {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "NewCatalogRepo")
	defer spanEnd()

	slog.DebugContext(traceCtx, "catalog repository created")

	return &catalogRepo{db: db}
}

// ListPublicObjects возвращает опубликованные объекты тенанта.
func (r *catalogRepo) ListPublicObjects(
	ctx context.Context,
	tenantID string,
	limit, offset int,
) ([]*model.PublicObject, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "ListPublicObjects")
	defer spanEnd()

	if err := model.ValidateUUID(tenantID); err != nil {
		return nil, fmt.Errorf("select public objects: %w", err)
	}

	const query = "SELECT title, description, price_minor, currency, address" +
		" FROM rental_objects WHERE tenant_id = ? AND status_id <> ?" +
		" ORDER BY created_at DESC LIMIT ? OFFSET ?"

	rows, err := r.db.QueryContext(traceCtx, query, tenantID, deletedObjectStatusID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("select public objects: %w", database.MapError(err))
	}
	defer func() { _ = rows.Close() }()

	objects := make([]*model.PublicObject, 0, limit)

	for rows.Next() {
		var (
			object      model.PublicObject
			description sql.NullString
			address     sql.NullString
		)

		if err := rows.Scan(&object.Title, &description, &object.PriceMinor, &object.Currency, &address); err != nil {
			return nil, fmt.Errorf("select public objects: %w", database.MapError(err))
		}

		object.Description = description.String
		object.Address = address.String

		objects = append(objects, &object)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("select public objects: %w", database.MapError(err))
	}

	return objects, nil
}
