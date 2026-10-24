package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/domain"
	"github.com/botbooker/bb-core/internal/model"
	"github.com/botbooker/bb-core/pkg/observability"
)

// CatalogDB — интерфейс базы данных, который нужен CatalogRepo.
//
// Набор методов совпадает с dbConn: внутри UnitOfWork запрос уходит в
// транзакцию из контекста, поэтому репозиторий должен уметь работать и с
// транзакцией, и с пулом.
type CatalogDB interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// CatalogRepo — алиас доменного порта CatalogRepository: репозиторий реализует
// контракт публичного каталога, объявленный в internal/domain.
type CatalogRepo = domain.CatalogRepository

// deletedObjectStatusID — статус объекта «удалён» (ERDB 2.9, seed миграции
// 00001_foundation). Мягко удалённые объекты публично не показываются.
const deletedObjectStatusID = 4

// catalogRepo — конкретная реализация, скрыта от внешних пакетов.
type catalogRepo struct {
	db CatalogDB
	codec
}

// NewCatalogRepo создаёт репозиторий публичного каталога.
//
// Диалект обязателен, как и у остальных репозиториев: запросы пишутся с
// плейсхолдерами «?», а PostgreSQL ждёт «$1». Без codec запрос отправлялся бы
// как есть, и на PostgreSQL каталог отвечал бы синтаксической ошибкой.
func NewCatalogRepo(ctx context.Context, db CatalogDB, dialect string) CatalogRepo {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "NewCatalogRepo")
	defer spanEnd()

	slog.DebugContext(traceCtx, "catalog repository created", "dialect", dialect)

	return &catalogRepo{db: db, codec: codec{dialect: dialect}}
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

	rows, err := conn(traceCtx, r.db).QueryContext(traceCtx, r.rebind(query), tenantID, deletedObjectStatusID, limit, offset)
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
