package servers

import (
	"context"
	"encoding/json"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/botbooker/bb-core/internal/model"
	"github.com/botbooker/bb-core/internal/service"
	"github.com/botbooker/bb-core/pkg/observability"
	tenant_v1 "github.com/botbooker/bb-core/pkg/proto/tenant/v1"
)

// TenantManagementService — то, что транспорту нужно от сервиса тенантов.
type TenantManagementService interface {
	Create(ctx context.Context, actor *service.Principal, in model.CreateTenantInput) (*model.Tenant, error)
	Get(ctx context.Context, actor *service.Principal, tenantID string) (*model.Tenant, error)
	List(ctx context.Context, actor *service.Principal, page model.Page) ([]*model.Tenant, string, error)
	Delete(ctx context.Context, actor *service.Principal, tenantID string) error
	UpdateStatus(
		ctx context.Context,
		actor *service.Principal,
		tenantID string,
		status model.TenantStatus,
	) (*model.Tenant, error)
	ListMembers(
		ctx context.Context,
		actor *service.Principal,
		tenantID string,
		page model.Page,
	) ([]*model.TenantMemberView, string, error)
	AddMember(
		ctx context.Context,
		actor *service.Principal,
		tenantID, userID string,
		role model.Role,
	) (*model.TenantMemberView, error)
	RemoveMember(ctx context.Context, actor *service.Principal, tenantID, userID string) error
	ChangeMemberRole(
		ctx context.Context,
		actor *service.Principal,
		tenantID, userID string,
		role model.Role,
	) (*model.TenantMemberView, error)
	TransferOwnership(
		ctx context.Context,
		actor *service.Principal,
		tenantID, userID string,
	) (*model.TenantMemberView, error)
}

// tenantServiceServer — адаптер proto-сервиса управления тенантами.
type tenantServiceServer struct {
	tenant_v1.UnimplementedTenantServiceServer

	tenants TenantManagementService
}

// Проверяем на этапе компиляции, что адаптер реализует proto-сервис целиком.
var _ tenant_v1.TenantServiceServer = (*tenantServiceServer)(nil)

// NewTenantServiceServer создаёт адаптер proto-сервиса тенантов.
func NewTenantServiceServer(tenants TenantManagementService) tenant_v1.TenantServiceServer {
	return &tenantServiceServer{tenants: tenants}
}

// GetTenant возвращает тенант по UUID.
func (s *tenantServiceServer) GetTenant(
	ctx context.Context,
	req *tenant_v1.GetTenantRequest,
) (*tenant_v1.TenantResponse, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "GetTenant")
	defer spanEnd()

	actor, err := tenantPrincipal(traceCtx)
	if err != nil {
		return nil, err
	}

	if err := req.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	tenant, err := s.tenants.Get(traceCtx, actor, req.GetUuid())
	if err != nil {
		return nil, toStatusError(traceCtx, err)
	}

	return &tenant_v1.TenantResponse{Tenant: tenantToProto(tenant)}, nil
}

// CreateTenant создаёт тенант; создатель становится владельцем.
func (s *tenantServiceServer) CreateTenant(
	ctx context.Context,
	req *tenant_v1.CreateTenantRequest,
) (*tenant_v1.TenantResponse, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "CreateTenant")
	defer spanEnd()

	actor, err := tenantPrincipal(traceCtx)
	if err != nil {
		return nil, err
	}

	if err := req.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	tenant, err := s.tenants.Create(traceCtx, actor, model.CreateTenantInput{
		Name:     req.GetName(),
		Slug:     req.GetSlug(),
		Metadata: req.GetMetadata(),
	})
	if err != nil {
		return nil, toStatusError(traceCtx, err)
	}

	return &tenant_v1.TenantResponse{Tenant: tenantToProto(tenant)}, nil
}

// ListTenants возвращает доступные тенанты с пагинацией.
func (s *tenantServiceServer) ListTenants(
	ctx context.Context,
	req *tenant_v1.ListTenantsRequest,
) (*tenant_v1.ListTenantsResponse, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "ListTenants")
	defer spanEnd()

	actor, err := tenantPrincipal(traceCtx)
	if err != nil {
		return nil, err
	}

	if err := req.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	page, err := model.NewPage(int(req.GetPageSize()), req.GetPageToken())
	if err != nil {
		return nil, toStatusError(traceCtx, err)
	}

	tenants, next, err := s.tenants.List(traceCtx, actor, page)
	if err != nil {
		return nil, toStatusError(traceCtx, err)
	}

	items := make([]*tenant_v1.Tenant, 0, len(tenants))
	for _, tenant := range tenants {
		items = append(items, tenantToProto(tenant))
	}

	return &tenant_v1.ListTenantsResponse{Tenants: items, NextPageToken: next}, nil
}

// DeleteTenant мягко удаляет тенант.
func (s *tenantServiceServer) DeleteTenant(
	ctx context.Context,
	req *tenant_v1.DeleteTenantRequest,
) (*emptypb.Empty, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "DeleteTenant")
	defer spanEnd()

	actor, err := tenantPrincipal(traceCtx)
	if err != nil {
		return nil, err
	}

	if err := req.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err := s.tenants.Delete(traceCtx, actor, req.GetUuid()); err != nil {
		return nil, toStatusError(traceCtx, err)
	}

	return &emptypb.Empty{}, nil
}

// UpdateTenantStatus меняет статус тенанта.
func (s *tenantServiceServer) UpdateTenantStatus(
	ctx context.Context,
	req *tenant_v1.UpdateTenantStatusRequest,
) (*tenant_v1.TenantResponse, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "UpdateTenantStatus")
	defer spanEnd()

	actor, err := tenantPrincipal(traceCtx)
	if err != nil {
		return nil, err
	}

	if err := req.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	statusValue, err := tenantStatusFromProto(req.GetStatus())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	tenant, err := s.tenants.UpdateStatus(traceCtx, actor, req.GetUuid(), statusValue)
	if err != nil {
		return nil, toStatusError(traceCtx, err)
	}

	return &tenant_v1.TenantResponse{Tenant: tenantToProto(tenant)}, nil
}

// ListMembers возвращает участников тенанта.
func (s *tenantServiceServer) ListMembers(
	ctx context.Context,
	req *tenant_v1.ListMembersRequest,
) (*tenant_v1.ListMembersResponse, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "ListMembers")
	defer spanEnd()

	actor, err := tenantPrincipal(traceCtx)
	if err != nil {
		return nil, err
	}

	if err := req.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	page, err := model.NewPage(int(req.GetPageSize()), req.GetPageToken())
	if err != nil {
		return nil, toStatusError(traceCtx, err)
	}

	members, next, err := s.tenants.ListMembers(traceCtx, actor, req.GetTenantUuid(), page)
	if err != nil {
		return nil, toStatusError(traceCtx, err)
	}

	items := make([]*tenant_v1.TenantMember, 0, len(members))
	for _, member := range members {
		items = append(items, memberToProto(member))
	}

	return &tenant_v1.ListMembersResponse{Members: items, NextPageToken: next}, nil
}

// AddMember добавляет участника в тенант.
func (s *tenantServiceServer) AddMember(
	ctx context.Context,
	req *tenant_v1.AddMemberRequest,
) (*tenant_v1.MemberResponse, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "AddMember")
	defer spanEnd()

	actor, err := tenantPrincipal(traceCtx)
	if err != nil {
		return nil, err
	}

	if err := req.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	role, err := model.ParseRole(req.GetRole())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	member, err := s.tenants.AddMember(traceCtx, actor, req.GetTenantUuid(), req.GetUserUuid(), role)
	if err != nil {
		return nil, toStatusError(traceCtx, err)
	}

	return &tenant_v1.MemberResponse{Member: memberToProto(member)}, nil
}

// RemoveMember исключает участника из тенанта.
func (s *tenantServiceServer) RemoveMember(
	ctx context.Context,
	req *tenant_v1.RemoveMemberRequest,
) (*emptypb.Empty, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "RemoveMember")
	defer spanEnd()

	actor, err := tenantPrincipal(traceCtx)
	if err != nil {
		return nil, err
	}

	if err := req.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err := s.tenants.RemoveMember(traceCtx, actor, req.GetTenantUuid(), req.GetUserUuid()); err != nil {
		return nil, toStatusError(traceCtx, err)
	}

	return &emptypb.Empty{}, nil
}

// ChangeMemberRole меняет роль участника.
func (s *tenantServiceServer) ChangeMemberRole(
	ctx context.Context,
	req *tenant_v1.ChangeMemberRoleRequest,
) (*tenant_v1.MemberResponse, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "ChangeMemberRole")
	defer spanEnd()

	actor, err := tenantPrincipal(traceCtx)
	if err != nil {
		return nil, err
	}

	if err := req.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	role, err := model.ParseRole(req.GetRole())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	member, err := s.tenants.ChangeMemberRole(
		traceCtx, actor, req.GetTenantUuid(), req.GetUserUuid(), role,
	)
	if err != nil {
		return nil, toStatusError(traceCtx, err)
	}

	return &tenant_v1.MemberResponse{Member: memberToProto(member)}, nil
}

// TransferOwnership передаёт владение тенантом.
func (s *tenantServiceServer) TransferOwnership(
	ctx context.Context,
	req *tenant_v1.TransferOwnershipRequest,
) (*tenant_v1.MemberResponse, error) {
	traceCtx, spanEnd := observability.TraceSpan(ctx, "TransferOwnership")
	defer spanEnd()

	actor, err := tenantPrincipal(traceCtx)
	if err != nil {
		return nil, err
	}

	if err := req.Validate(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	member, err := s.tenants.TransferOwnership(traceCtx, actor, req.GetTenantUuid(), req.GetUserUuid())
	if err != nil {
		return nil, toStatusError(traceCtx, err)
	}

	return &tenant_v1.MemberResponse{Member: memberToProto(member)}, nil
}

// tenantPrincipal достаёт действующего пользователя из контекста.
func tenantPrincipal(ctx context.Context) (*service.Principal, error) {
	principal, ok := service.PrincipalFromContext(ctx)
	if !ok || principal.User == nil {
		return nil, status.Error(codes.Unauthenticated, unauthorizedMessage)
	}

	return principal, nil
}

// tenantToProto конвертирует тенант в proto-сообщение.
func tenantToProto(tenant *model.Tenant) *tenant_v1.Tenant {
	return &tenant_v1.Tenant{
		Uuid:      tenant.ID,
		Name:      tenant.Name,
		Slug:      tenant.Slug,
		Status:    tenantStatusToProto(tenant.Status),
		Metadata:  tenantMetadata(tenant.Metadata),
		CreatedAt: timestamppb.New(tenant.CreatedAt.UTC()),
		UpdatedAt: timestamppb.New(tenant.UpdatedAt.UTC()),
	}
}

// memberToProto конвертирует участника тенанта в proto-сообщение.
func memberToProto(member *model.TenantMemberView) *tenant_v1.TenantMember {
	if member == nil {
		return nil
	}

	return &tenant_v1.TenantMember{
		TenantUuid: member.TenantID,
		UserUuid:   member.UserID,
		Role:       member.Role.String(),
		IsOwner:    member.IsOwner,
		JoinedAt:   timestamppb.New(member.JoinedAt.UTC()),
		Email:      member.Email,
		FirstName:  member.FirstName,
		LastName:   member.LastName,
	}
}

// tenantMetadata декодирует JSON-настройки тенанта; повреждённое значение
// считается отсутствующим — публичные ручки уже работают по той же стратегии.
func tenantMetadata(raw []byte) map[string]string {
	if len(raw) == 0 {
		return nil
	}

	var metadata map[string]string
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return nil
	}

	return metadata
}

// tenantStatusToProto конвертирует статус тенанта в enum.
func tenantStatusToProto(status model.TenantStatus) tenant_v1.TenantStatus {
	switch status {
	case model.TenantStatusActive:
		return tenant_v1.TenantStatus_TENANT_STATUS_ACTIVE
	case model.TenantStatusSuspended:
		return tenant_v1.TenantStatus_TENANT_STATUS_SUSPENDED
	case model.TenantStatusDeleted:
		return tenant_v1.TenantStatus_TENANT_STATUS_DELETED
	default:
		return tenant_v1.TenantStatus_TENANT_STATUS_UNSPECIFIED
	}
}

// tenantStatusFromProto конвертирует enum статуса в модель.
func tenantStatusFromProto(value tenant_v1.TenantStatus) (model.TenantStatus, error) {
	switch value {
	case tenant_v1.TenantStatus_TENANT_STATUS_ACTIVE:
		return model.TenantStatusActive, nil
	case tenant_v1.TenantStatus_TENANT_STATUS_SUSPENDED:
		return model.TenantStatusSuspended, nil
	case tenant_v1.TenantStatus_TENANT_STATUS_DELETED:
		return model.TenantStatusDeleted, nil
	default:
		return "", fmt.Errorf("%w: status %s", model.ErrInvalidTenant, value.String())
	}
}
