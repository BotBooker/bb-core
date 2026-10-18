package servers

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/botbooker/bb-core/internal/database"
	"github.com/botbooker/bb-core/internal/model"
	"github.com/botbooker/bb-core/internal/service"
	tenant_v1 "github.com/botbooker/bb-core/pkg/proto/tenant/v1"
)

// mockTenantAPI — полный мок сервиса тенантов для адаптера.
type mockTenantAPI struct {
	createFunc   func(ctx context.Context, actor *service.Principal, in model.CreateTenantInput) (*model.Tenant, error)
	getFunc      func(ctx context.Context, actor *service.Principal, tenantID string) (*model.Tenant, error)
	listFunc     func(ctx context.Context, actor *service.Principal, page model.Page) ([]*model.Tenant, string, error)
	deleteFunc   func(ctx context.Context, actor *service.Principal, tenantID string) error
	statusFunc   func(ctx context.Context, actor *service.Principal, tenantID string, status model.TenantStatus) (*model.Tenant, error)
	membersFunc  func(ctx context.Context, actor *service.Principal, tenantID string, page model.Page) ([]*model.TenantMemberView, string, error)
	addFunc      func(ctx context.Context, actor *service.Principal, tenantID, userID string, role model.Role) (*model.TenantMemberView, error)
	removeFunc   func(ctx context.Context, actor *service.Principal, tenantID, userID string) error
	roleFunc     func(ctx context.Context, actor *service.Principal, tenantID, userID string, role model.Role) (*model.TenantMemberView, error)
	transferFunc func(ctx context.Context, actor *service.Principal, tenantID, userID string) (*model.TenantMemberView, error)
}

func (m *mockTenantAPI) Create(ctx context.Context, actor *service.Principal, in model.CreateTenantInput) (*model.Tenant, error) {
	if m.createFunc != nil {
		return m.createFunc(ctx, actor, in)
	}

	return nil, errors.New("unexpected Create()")
}

func (m *mockTenantAPI) Get(ctx context.Context, actor *service.Principal, tenantID string) (*model.Tenant, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, actor, tenantID)
	}

	return nil, errors.New("unexpected Get()")
}

func (m *mockTenantAPI) List(ctx context.Context, actor *service.Principal, page model.Page) ([]*model.Tenant, string, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, actor, page)
	}

	return nil, "", errors.New("unexpected List()")
}

func (m *mockTenantAPI) Delete(ctx context.Context, actor *service.Principal, tenantID string) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, actor, tenantID)
	}

	return errors.New("unexpected Delete()")
}

func (m *mockTenantAPI) UpdateStatus(
	ctx context.Context,
	actor *service.Principal,
	tenantID string,
	value model.TenantStatus,
) (*model.Tenant, error) {
	if m.statusFunc != nil {
		return m.statusFunc(ctx, actor, tenantID, value)
	}

	return nil, errors.New("unexpected UpdateStatus()")
}

func (m *mockTenantAPI) ListMembers(
	ctx context.Context,
	actor *service.Principal,
	tenantID string,
	page model.Page,
) ([]*model.TenantMemberView, string, error) {
	if m.membersFunc != nil {
		return m.membersFunc(ctx, actor, tenantID, page)
	}

	return nil, "", errors.New("unexpected ListMembers()")
}

func (m *mockTenantAPI) AddMember(
	ctx context.Context,
	actor *service.Principal,
	tenantID, userID string,
	role model.Role,
) (*model.TenantMemberView, error) {
	if m.addFunc != nil {
		return m.addFunc(ctx, actor, tenantID, userID, role)
	}

	return nil, errors.New("unexpected AddMember()")
}

func (m *mockTenantAPI) RemoveMember(ctx context.Context, actor *service.Principal, tenantID, userID string) error {
	if m.removeFunc != nil {
		return m.removeFunc(ctx, actor, tenantID, userID)
	}

	return errors.New("unexpected RemoveMember()")
}

func (m *mockTenantAPI) ChangeMemberRole(
	ctx context.Context,
	actor *service.Principal,
	tenantID, userID string,
	role model.Role,
) (*model.TenantMemberView, error) {
	if m.roleFunc != nil {
		return m.roleFunc(ctx, actor, tenantID, userID, role)
	}

	return nil, errors.New("unexpected ChangeMemberRole()")
}

func (m *mockTenantAPI) TransferOwnership(
	ctx context.Context,
	actor *service.Principal,
	tenantID, userID string,
) (*model.TenantMemberView, error) {
	if m.transferFunc != nil {
		return m.transferFunc(ctx, actor, tenantID, userID)
	}

	return nil, errors.New("unexpected TransferOwnership()")
}

// tenantTestContext — контекст с действующим администратором.
func tenantTestContext(t *testing.T) context.Context {
	t.Helper()

	return service.WithPrincipal(context.Background(), testPrincipal(t, handlerTestUser(t), model.RoleAdmin))
}

// testTenantModel — тенант для ответов адаптера.
func testTenantModel() *model.Tenant {
	return &model.Tenant{
		ID:        testTenantID,
		Name:      "Acme",
		Slug:      "acme",
		Status:    model.TenantStatusActive,
		Metadata:  []byte(`{"description":"Аренда"}`),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
}

func TestTenantServiceServer_GetAndCreate(t *testing.T) {
	api := &mockTenantAPI{
		getFunc: func(_ context.Context, actor *service.Principal, tenantID string) (*model.Tenant, error) {
			if actor == nil || tenantID != testTenantID {
				t.Errorf("Get() actor=%v tenant=%q", actor, tenantID)
			}

			return testTenantModel(), nil
		},
		createFunc: func(_ context.Context, _ *service.Principal, in model.CreateTenantInput) (*model.Tenant, error) {
			if in.Name != "Acme" || in.Slug != "acme" || in.Metadata["description"] != "Аренда" {
				t.Errorf("Create() input = %+v", in)
			}

			return testTenantModel(), nil
		},
	}

	server := NewTenantServiceServer(api)

	resp, err := server.GetTenant(tenantTestContext(t), &tenant_v1.GetTenantRequest{Uuid: testTenantID})
	if err != nil || resp.GetTenant().GetSlug() != "acme" {
		t.Fatalf("GetTenant() = %v, %v", resp, err)
	}

	created, err := server.CreateTenant(tenantTestContext(t), &tenant_v1.CreateTenantRequest{
		Name:     "Acme",
		Slug:     "acme",
		Metadata: map[string]string{"description": "Аренда"},
	})
	if err != nil || created.GetTenant().GetUuid() != testTenantID {
		t.Fatalf("CreateTenant() = %v, %v", created, err)
	}

	if _, err := server.GetTenant(context.Background(), &tenant_v1.GetTenantRequest{Uuid: testTenantID}); status.Code(err) != codes.Unauthenticated {
		t.Errorf("GetTenant() без principal code = %v", status.Code(err))
	}

	if _, err := server.GetTenant(tenantTestContext(t), &tenant_v1.GetTenantRequest{Uuid: "bad"}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("GetTenant() c плохим UUID code = %v", status.Code(err))
	}
}

func TestTenantServiceServer_ListAndStatus(t *testing.T) {
	api := &mockTenantAPI{
		listFunc: func(_ context.Context, _ *service.Principal, page model.Page) ([]*model.Tenant, string, error) {
			if page.Limit != 10 {
				t.Errorf("List() limit = %d", page.Limit)
			}

			return []*model.Tenant{testTenantModel()}, "next", nil
		},
		statusFunc: func(_ context.Context, _ *service.Principal, _ string, value model.TenantStatus) (*model.Tenant, error) {
			if value != model.TenantStatusSuspended {
				t.Errorf("UpdateStatus() = %q", value)
			}

			tenant := testTenantModel()
			tenant.Status = value

			return tenant, nil
		},
		deleteFunc: func(context.Context, *service.Principal, string) error { return database.ErrNotFound },
	}

	server := NewTenantServiceServer(api)

	list, err := server.ListTenants(tenantTestContext(t), &tenant_v1.ListTenantsRequest{PageSize: 10})
	if err != nil || len(list.GetTenants()) != 1 || list.GetNextPageToken() != "next" {
		t.Fatalf("ListTenants() = %v, %v", list, err)
	}

	updated, err := server.UpdateTenantStatus(tenantTestContext(t), &tenant_v1.UpdateTenantStatusRequest{
		Uuid:   testTenantID,
		Status: tenant_v1.TenantStatus_TENANT_STATUS_SUSPENDED,
	})
	if err != nil || updated.GetTenant().GetStatus() != tenant_v1.TenantStatus_TENANT_STATUS_SUSPENDED {
		t.Fatalf("UpdateTenantStatus() = %v, %v", updated, err)
	}

	if _, err := server.UpdateTenantStatus(tenantTestContext(t), &tenant_v1.UpdateTenantStatusRequest{
		Uuid: testTenantID,
	}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("UpdateTenantStatus() unspecified code = %v", status.Code(err))
	}

	if _, err := server.DeleteTenant(tenantTestContext(t), &tenant_v1.DeleteTenantRequest{Uuid: testTenantID}); status.Code(err) != codes.NotFound {
		t.Errorf("DeleteTenant() code = %v, want NotFound", status.Code(err))
	}
}

func TestTenantServiceServer_Members(t *testing.T) {
	member := &model.TenantMemberView{
		TenantMember: model.TenantMember{
			TenantID: testTenantID,
			UserID:   "22222222-3333-4444-8555-666666666666",
			Role:     model.RoleClient,
			JoinedAt: time.Now().UTC(),
		},
		Email:     "member@example.com",
		FirstName: "Иван",
		LastName:  "Иванов",
	}

	api := &mockTenantAPI{
		membersFunc: func(_ context.Context, _ *service.Principal, _ string, _ model.Page) ([]*model.TenantMemberView, string, error) {
			return []*model.TenantMemberView{member}, "next", nil
		},
		addFunc: func(_ context.Context, _ *service.Principal, _, _ string, role model.Role) (*model.TenantMemberView, error) {
			if role != model.RoleManager {
				t.Errorf("AddMember() role = %q", role)
			}

			return member, nil
		},
		roleFunc: func(_ context.Context, _ *service.Principal, _, _ string, role model.Role) (*model.TenantMemberView, error) {
			if role != model.RoleOperator {
				t.Errorf("ChangeMemberRole() role = %q", role)
			}

			return member, nil
		},
		removeFunc: func(context.Context, *service.Principal, string, string) error { return nil },
		transferFunc: func(context.Context, *service.Principal, string, string) (*model.TenantMemberView, error) {
			return member, nil
		},
	}

	server := NewTenantServiceServer(api)
	ctx := tenantTestContext(t)

	list, err := server.ListMembers(ctx, &tenant_v1.ListMembersRequest{TenantUuid: testTenantID, PageSize: 10})
	if err != nil || len(list.GetMembers()) != 1 || list.GetMembers()[0].GetEmail() != "member@example.com" {
		t.Fatalf("ListMembers() = %v, %v", list, err)
	}

	added, err := server.AddMember(ctx, &tenant_v1.AddMemberRequest{
		TenantUuid: testTenantID,
		UserUuid:   member.UserID,
		Role:       "manager",
	})
	if err != nil || added.GetMember().GetUserUuid() != member.UserID {
		t.Fatalf("AddMember() = %v, %v", added, err)
	}

	changed, err := server.ChangeMemberRole(ctx, &tenant_v1.ChangeMemberRoleRequest{
		TenantUuid: testTenantID,
		UserUuid:   member.UserID,
		Role:       "operator",
	})
	if err != nil || changed.GetMember().GetUserUuid() != member.UserID {
		t.Fatalf("ChangeMemberRole() = %v, %v", changed, err)
	}

	if _, err := server.RemoveMember(ctx, &tenant_v1.RemoveMemberRequest{
		TenantUuid: testTenantID,
		UserUuid:   member.UserID,
	}); err != nil {
		t.Fatalf("RemoveMember() unexpected error = %v", err)
	}

	if _, err := server.TransferOwnership(ctx, &tenant_v1.TransferOwnershipRequest{
		TenantUuid: testTenantID,
		UserUuid:   member.UserID,
	}); err != nil {
		t.Fatalf("TransferOwnership() unexpected error = %v", err)
	}

	if _, err := server.AddMember(ctx, &tenant_v1.AddMemberRequest{
		TenantUuid: testTenantID,
		UserUuid:   member.UserID,
		Role:       "wat",
	}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("AddMember() неизвестная роль code = %v", status.Code(err))
	}
}

func TestTenantMapping(t *testing.T) {
	tenant := testTenantModel()

	protoTenant := tenantToProto(tenant)
	if protoTenant.GetMetadata()["description"] != "Аренда" {
		t.Errorf("metadata = %v", protoTenant.GetMetadata())
	}

	tenant.Metadata = []byte("not-json")
	if got := tenantToProto(tenant).GetMetadata(); got != nil {
		t.Errorf("metadata повреждена, got %v, want nil", got)
	}

	statuses := map[model.TenantStatus]tenant_v1.TenantStatus{
		model.TenantStatusActive:    tenant_v1.TenantStatus_TENANT_STATUS_ACTIVE,
		model.TenantStatusSuspended: tenant_v1.TenantStatus_TENANT_STATUS_SUSPENDED,
		model.TenantStatusDeleted:   tenant_v1.TenantStatus_TENANT_STATUS_DELETED,
		"unknown":                   tenant_v1.TenantStatus_TENANT_STATUS_UNSPECIFIED,
	}

	for input, want := range statuses {
		if got := tenantStatusToProto(input); got != want {
			t.Errorf("tenantStatusToProto(%q) = %v, want %v", input, got, want)
		}
	}
}
