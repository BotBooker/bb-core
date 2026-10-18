package servers

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/botbooker/bb-core/internal/model"
	"github.com/botbooker/bb-core/pkg/observability"
	public_v1 "github.com/botbooker/bb-core/pkg/proto/public/v1"
)

// PublicCatalogService — то, что транспорту нужно от публичного каталога.
type PublicCatalogService interface {
	GetTenant(ctx context.Context, slug string) (*model.PublicTenant, error)
	ListTenants(ctx context.Context, page model.Page) ([]*model.PublicTenant, string, error)
	ListObjects(ctx context.Context, slug string, page model.Page) ([]*model.PublicObject, string, error)
}

// publicCatalogServer — адаптер публичного proto-сервиса.
type publicCatalogServer struct {
	public_v1.UnimplementedPublicCatalogServiceServer

	catalog PublicCatalogService
}

// Проверяем на этапе компиляции, что адаптер реализует proto-сервис целиком.
var _ public_v1.PublicCatalogServiceServer = (*publicCatalogServer)(nil)

// NewPublicCatalogServer создаёт адаптер публичного каталога.
func NewPublicCatalogServer(catalog PublicCatalogService) public_v1.PublicCatalogServiceServer {
	return &publicCatalogServer{catalog: catalog}
}

// GetTenant возвращает публичный профиль тенанта по slug.
func (s *publicCatalogServer) GetTenant(
	ctx context.Context,
	req *public_v1.GetTenantRequest,
) (*public_v1.PublicTenantResponse, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "GetPublicTenant")
	defer spanEnd()

	if err := req.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	tenant, err := s.catalog.GetTenant(traceCtx, req.GetSlug())
	if err != nil {
		return nil, toStatusError(traceCtx, err)
	}

	return &public_v1.PublicTenantResponse{Tenant: publicTenantToProto(tenant)}, nil
}

// ListTenants возвращает каталог действующих тенантов.
func (s *publicCatalogServer) ListTenants(
	ctx context.Context,
	req *public_v1.ListTenantsRequest,
) (*public_v1.ListTenantsResponse, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "ListPublicTenants")
	defer spanEnd()

	if err := req.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	page, err := model.NewPage(int(req.GetPageSize()), req.GetPageToken())
	if err != nil {
		return nil, toStatusError(traceCtx, err)
	}

	tenants, next, err := s.catalog.ListTenants(traceCtx, page)
	if err != nil {
		return nil, toStatusError(traceCtx, err)
	}

	items := make([]*public_v1.PublicTenant, 0, len(tenants))
	for _, tenant := range tenants {
		items = append(items, publicTenantToProto(tenant))
	}

	return &public_v1.ListTenantsResponse{Tenants: items, NextPageToken: next}, nil
}

// ListObjects возвращает публичный каталог объектов тенанта.
func (s *publicCatalogServer) ListObjects(
	ctx context.Context,
	req *public_v1.ListObjectsRequest,
) (*public_v1.ListObjectsResponse, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "ListPublicObjects")
	defer spanEnd()

	if err := req.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	page, err := model.NewPage(int(req.GetPageSize()), req.GetPageToken())
	if err != nil {
		return nil, toStatusError(traceCtx, err)
	}

	objects, next, err := s.catalog.ListObjects(traceCtx, req.GetSlug(), page)
	if err != nil {
		return nil, toStatusError(traceCtx, err)
	}

	items := make([]*public_v1.PublicObject, 0, len(objects))
	for _, object := range objects {
		items = append(items, publicObjectToProto(object))
	}

	return &public_v1.ListObjectsResponse{Objects: items, NextPageToken: next}, nil
}

// publicTenantToProto конвертирует публичный профиль тенанта.
func publicTenantToProto(tenant *model.PublicTenant) *public_v1.PublicTenant {
	return &public_v1.PublicTenant{
		Name:        tenant.Name,
		Slug:        tenant.Slug,
		Description: tenant.Description,
		Location:    tenant.Location,
	}
}

// publicObjectToProto конвертирует публичную карточку объекта.
func publicObjectToProto(object *model.PublicObject) *public_v1.PublicObject {
	return &public_v1.PublicObject{
		Title:       object.Title,
		Description: object.Description,
		PriceMinor:  object.PriceMinor,
		Currency:    object.Currency,
		Address:     object.Address,
	}
}
