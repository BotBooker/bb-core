package model

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// tenantNow — фиксированное «сейчас» для тестов тенанта.
var tenantNow = time.Date(2026, time.October, 12, 10, 0, 0, 0, time.UTC)

// testTenantUUID / testUserUUID — идентификаторы для тестов членства.
const (
	testTenantUUID = "0f0c8f0e-9f60-4e0e-9d5f-1f6f1f6f1f6f"
	testUserUUID   = "3f0c8f0e-9f60-4e0e-9d5f-1f6f1f6f1f6f"
)

// newValidTenant возвращает валидный тенант: тест портит только то поле,
// которое проверяет.
func newValidTenant(t *testing.T) *Tenant {
	t.Helper()

	tenant, err := NewTenant("ООО «Ромашка»", "romashka", tenantNow)
	if err != nil {
		t.Fatalf("NewTenant() unexpected error = %v", err)
	}

	return tenant
}

func TestNewTenant(t *testing.T) {
	t.Run("заполняет поля и служебные значения", func(t *testing.T) {
		tenant := newValidTenant(t)

		if err := ValidateUUID(tenant.ID); err != nil {
			t.Errorf("ID = %q: %v", tenant.ID, err)
		}
		if tenant.Name != "ООО «Ромашка»" {
			t.Errorf("Name = %q", tenant.Name)
		}
		if tenant.Slug != "romashka" {
			t.Errorf("Slug = %q, want %q", tenant.Slug, "romashka")
		}
		if tenant.Status != TenantStatusActive || !tenant.Active() {
			t.Errorf("Status = %q, want %q", tenant.Status, TenantStatusActive)
		}
		if tenant.Metadata != nil {
			t.Errorf("Metadata = %v, want nil: настройки задаются отдельно", tenant.Metadata)
		}
		if !tenant.CreatedAt.Equal(tenantNow) || !tenant.UpdatedAt.Equal(tenantNow) {
			t.Errorf("CreatedAt/UpdatedAt = %v/%v, want %v", tenant.CreatedAt, tenant.UpdatedAt, tenantNow)
		}
		if err := tenant.Validate(); err != nil {
			t.Errorf("Validate() unexpected error = %v", err)
		}
	})

	t.Run("нормализует название и slug", func(t *testing.T) {
		tenant, err := NewTenant("  Ромашка  ", "  RoMashka  ", tenantNow)
		if err != nil {
			t.Fatalf("NewTenant() unexpected error = %v", err)
		}

		if tenant.Name != "Ромашка" {
			t.Errorf("Name = %q, want %q", tenant.Name, "Ромашка")
		}
		if tenant.Slug != "romashka" {
			t.Errorf("Slug = %q, want %q", tenant.Slug, "romashka")
		}
	})

	t.Run("генерирует разные идентификаторы", func(t *testing.T) {
		first := newValidTenant(t)
		second := newValidTenant(t)

		if first.ID == second.ID {
			t.Errorf("ID совпадают: %q", first.ID)
		}
	})

	t.Run("время приводится к UTC", func(t *testing.T) {
		zone := time.FixedZone("MSK", 3*60*60)

		tenant, err := NewTenant("Ромашка", "romashka", tenantNow.In(zone))
		if err != nil {
			t.Fatalf("NewTenant() unexpected error = %v", err)
		}

		if tenant.CreatedAt.Location() != time.UTC || tenant.UpdatedAt.Location() != time.UTC {
			t.Errorf("время не в UTC: %v/%v", tenant.CreatedAt, tenant.UpdatedAt)
		}
	})

	t.Run("отказы", func(t *testing.T) {
		tests := []struct {
			scenario string
			name     string
			slug     string
			want     error
		}{
			{scenario: "пустое название", name: "", slug: "romashka", want: ErrInvalidTenant},
			{scenario: "название только из пробелов", name: "   ", slug: "romashka", want: ErrInvalidTenant},
			{
				scenario: "название 101 руна",
				name:     strings.Repeat("я", TenantNameMaxRunes+1),
				slug:     "romashka",
				want:     ErrInvalidTenant,
			},
			{scenario: "пустой slug", name: "Ромашка", slug: "", want: ErrInvalidTenant},
			{scenario: "slug только из пробелов", name: "Ромашка", slug: "   ", want: ErrInvalidTenant},
			{scenario: "пробел внутри slug", name: "Ромашка", slug: "roma shka", want: ErrInvalidTenant},
			{
				scenario: "slug 101 руна",
				name:     "Ромашка",
				slug:     strings.Repeat("a", TenantSlugMaxRunes+1),
				want:     ErrInvalidTenant,
			},
		}

		for _, tt := range tests {
			t.Run(tt.scenario, func(t *testing.T) {
				_, err := NewTenant(tt.name, tt.slug, tenantNow)
				if !errors.Is(err, tt.want) {
					t.Errorf("NewTenant() error = %v, want %v", err, tt.want)
				}
			})
		}
	})
}

// TestTenant_Validate проверяет инварианты хранимой записи тенанта.
func TestTenant_Validate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(tenant *Tenant)
		wantErr error
	}{
		{name: "валидный тенант"},
		{name: "метаданные заданы", mutate: func(x *Tenant) { x.Metadata = []byte(`{"plan":"pro"}`) }},
		{name: "приостановлен", mutate: func(x *Tenant) { x.Status = TenantStatusSuspended }},
		{name: "удалён", mutate: func(x *Tenant) { x.Status = TenantStatusDeleted }},
		{name: "пустой uuid", mutate: func(x *Tenant) { x.ID = "" }, wantErr: ErrInvalidTenant},
		{name: "uuid не парсится", mutate: func(x *Tenant) { x.ID = "not-a-uuid" }, wantErr: ErrInvalidTenant},
		{name: "пустое название", mutate: func(x *Tenant) { x.Name = "" }, wantErr: ErrInvalidTenant},
		{
			name:    "название 101 руна",
			mutate:  func(x *Tenant) { x.Name = strings.Repeat("я", TenantNameMaxRunes+1) },
			wantErr: ErrInvalidTenant,
		},
		{name: "пустой slug", mutate: func(x *Tenant) { x.Slug = "" }, wantErr: ErrInvalidTenant},
		{name: "пробел в slug", mutate: func(x *Tenant) { x.Slug = "roma shka" }, wantErr: ErrInvalidTenant},
		{name: "неизвестный статус", mutate: func(x *Tenant) { x.Status = "wat" }, wantErr: ErrInvalidTenant},
		{name: "статус не задан", mutate: func(x *Tenant) { x.Status = "" }, wantErr: ErrInvalidTenant},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tenant := newValidTenant(t)
			if tt.mutate != nil {
				tt.mutate(tenant)
			}

			err := tenant.Validate()

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Validate() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("Validate() unexpected error = %v", err)
			}
		})
	}
}

// TestTenant_Status проверяет предикаты статуса: на них опирается доступ
// участников (приостановленный и удалённый тенант не действует).
func TestTenant_Status(t *testing.T) {
	tests := []struct {
		status      TenantStatus
		active      bool
		suspended   bool
		deleted     bool
		statusValid bool
	}{
		{status: TenantStatusActive, active: true, statusValid: true},
		{status: TenantStatusSuspended, suspended: true, statusValid: true},
		{status: TenantStatusDeleted, deleted: true, statusValid: true},
		{status: TenantStatus("wat"), statusValid: false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			tenant := Tenant{Status: tt.status}

			if got := tenant.Active(); got != tt.active {
				t.Errorf("Active() = %v, want %v", got, tt.active)
			}
			if got := tenant.Suspended(); got != tt.suspended {
				t.Errorf("Suspended() = %v, want %v", got, tt.suspended)
			}
			if got := tenant.Deleted(); got != tt.deleted {
				t.Errorf("Deleted() = %v, want %v", got, tt.deleted)
			}
			if got := tt.status.Valid(); got != tt.statusValid {
				t.Errorf("Valid() = %v, want %v", got, tt.statusValid)
			}
			if got := tt.status.String(); got != string(tt.status) {
				t.Errorf("String() = %q, want %q", got, tt.status)
			}
		})
	}
}

// TestNormalizeSlug проверяет приведение slug к хранимому виду: тот же вид
// нужен репозиторию при поиске тенанта по slug.
func TestNormalizeSlug(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "уже нормализован", value: "romashka", want: "romashka"},
		{name: "верхний регистр", value: "ROMASHKA", want: "romashka"},
		{name: "смешанный регистр и пробелы", value: "  RoMashka  ", want: "romashka"},
		{name: "табуляция по краям", value: "\tromashka\t", want: "romashka"},
		{name: "кириллица", value: "Ромашка", want: "ромашка"},
		{name: "пустая строка", value: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeSlug(tt.value); got != tt.want {
				t.Errorf("NormalizeSlug(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

// TestNewTenantMember проверяет создание членства: роль по умолчанию, флаг
// владельца и время вступления.
func TestNewTenantMember(t *testing.T) {
	t.Run("заполняет поля", func(t *testing.T) {
		member, err := NewTenantMember(testTenantUUID, testUserUUID, RoleManager, true, tenantNow)
		if err != nil {
			t.Fatalf("NewTenantMember() unexpected error = %v", err)
		}

		if member.TenantID != testTenantUUID || member.UserID != testUserUUID {
			t.Errorf("TenantID/UserID = %q/%q, want %q/%q",
				member.TenantID, member.UserID, testTenantUUID, testUserUUID)
		}
		if member.Role != RoleManager {
			t.Errorf("Role = %q, want %q", member.Role, RoleManager)
		}
		if !member.IsOwner {
			t.Error("IsOwner = false, want true")
		}
		if !member.JoinedAt.Equal(tenantNow) {
			t.Errorf("JoinedAt = %v, want %v", member.JoinedAt, tenantNow)
		}
		if err := member.Validate(); err != nil {
			t.Errorf("Validate() unexpected error = %v", err)
		}
	})

	t.Run("пустая роль означает роль по умолчанию", func(t *testing.T) {
		member, err := NewTenantMember(testTenantUUID, testUserUUID, "", false, tenantNow)
		if err != nil {
			t.Fatalf("NewTenantMember() unexpected error = %v", err)
		}

		if member.Role != RoleClient {
			t.Errorf("Role = %q, want %q", member.Role, RoleClient)
		}

		id, err := member.Role.ID()
		if err != nil {
			t.Fatalf("Role.ID() unexpected error = %v", err)
		}
		if id != RoleDefaultID {
			t.Errorf("Role.ID() = %d, want %d (DEFAULT tenant_members.role_id)", id, RoleDefaultID)
		}
	})

	t.Run("время вступления приводится к UTC", func(t *testing.T) {
		zone := time.FixedZone("MSK", 3*60*60)

		member, err := NewTenantMember(testTenantUUID, testUserUUID, RoleClient, false, tenantNow.In(zone))
		if err != nil {
			t.Fatalf("NewTenantMember() unexpected error = %v", err)
		}

		if member.JoinedAt.Location() != time.UTC {
			t.Errorf("JoinedAt в поясе %v, want UTC", member.JoinedAt.Location())
		}
	})

	t.Run("отказы", func(t *testing.T) {
		tests := []struct {
			scenario string
			tenantID string
			userID   string
			role     Role
		}{
			{scenario: "тенант не UUID", tenantID: "not-a-uuid", userID: testUserUUID, role: RoleClient},
			{scenario: "пустой тенант", tenantID: "", userID: testUserUUID, role: RoleClient},
			{scenario: "пользователь не UUID", tenantID: testTenantUUID, userID: "not-a-uuid", role: RoleClient},
			{scenario: "пустой пользователь", tenantID: testTenantUUID, userID: "", role: RoleClient},
			{scenario: "неизвестная роль", tenantID: testTenantUUID, userID: testUserUUID, role: Role("root")},
		}

		for _, tt := range tests {
			t.Run(tt.scenario, func(t *testing.T) {
				_, err := NewTenantMember(tt.tenantID, tt.userID, tt.role, false, tenantNow)
				if !errors.Is(err, ErrInvalidTenantMember) {
					t.Errorf("NewTenantMember() error = %v, want %v", err, ErrInvalidTenantMember)
				}
			})
		}
	})
}

// TestTenantMember_Validate проверяет инварианты хранимой записи членства.
func TestTenantMember_Validate(t *testing.T) {
	valid := TenantMember{
		TenantID: testTenantUUID,
		UserID:   testUserUUID,
		Role:     RoleClient,
		JoinedAt: tenantNow,
	}

	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(member *TenantMember)
	}{
		{name: "пустой тенант", mutate: func(m *TenantMember) { m.TenantID = "" }},
		{name: "пустой пользователь", mutate: func(m *TenantMember) { m.UserID = "" }},
		{name: "роль не задана", mutate: func(m *TenantMember) { m.Role = "" }},
		{name: "неизвестная роль", mutate: func(m *TenantMember) { m.Role = Role("root") }},
		{name: "нет времени вступления", mutate: func(m *TenantMember) { m.JoinedAt = time.Time{} }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			member := valid
			tt.mutate(&member)

			if err := member.Validate(); !errors.Is(err, ErrInvalidTenantMember) {
				t.Errorf("Validate() error = %v, want %v", err, ErrInvalidTenantMember)
			}
		})
	}

	t.Run("неизвестная роль оборачивает ErrInvalidRole", func(t *testing.T) {
		member := valid
		member.Role = Role("root")

		if err := member.Validate(); !errors.Is(err, ErrInvalidRole) {
			t.Errorf("Validate() error = %v, want обёртку %v", err, ErrInvalidRole)
		}
	})
}
