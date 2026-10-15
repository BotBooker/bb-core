package model

import (
	"errors"
	"testing"
)

// TestRoleValid проверяет известность роли и её строковое представление.

func TestRoleValid(t *testing.T) {
	tests := []struct {
		name  string
		value Role
		want  bool
	}{
		{name: "admin", value: RoleAdmin, want: true},
		{name: "operator", value: RoleOperator, want: true},
		{name: "manager", value: RoleManager, want: true},
		{name: "client", value: RoleClient, want: true},
		{name: "пустое значение", value: "", want: false},
		{name: "неизвестное значение", value: Role("root"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.value.Valid(); got != tt.want {
				t.Errorf("Role(%q).Valid() = %v, want %v", tt.value, got, tt.want)
			}
			if got := tt.value.String(); got != string(tt.value) {
				t.Errorf("Role(%q).String() = %q, want %q", tt.value, got, tt.value)
			}
		})
	}
}

// TestParseRole проверяет разбор кода роли (граница API): пустое значение —
// роль по умолчанию, неизвестный код — ошибка.
func TestParseRole(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    Role
		wantErr error
	}{
		{name: "admin", value: "admin", want: RoleAdmin},
		{name: "operator", value: "operator", want: RoleOperator},
		{name: "manager", value: "manager", want: RoleManager},
		{name: "client", value: "client", want: RoleClient},
		{name: "значение с пробелами", value: " admin ", want: RoleAdmin},
		{name: "пустая строка", value: "", want: RoleClient},
		{name: "неизвестная роль", value: "root", wantErr: ErrInvalidRole},
		{name: "другой регистр", value: "CLIENT", wantErr: ErrInvalidRole},
		{name: "числовой идентификатор вместо кода", value: "1", wantErr: ErrInvalidRole},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRole(tt.value)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ParseRole(%q) error = %v, want %v", tt.value, err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("ParseRole(%q) unexpected error = %v", tt.value, err)
			}
			if got != tt.want {
				t.Errorf("ParseRole(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

// TestRole_ID проверяет перевод кода роли в идентификатор справочника roles:
// значения должны совпадать с seed миграции 00001_foundation.
func TestRole_ID(t *testing.T) {
	tests := []struct {
		name    string
		role    Role
		want    int16
		wantErr error
	}{
		{name: "admin", role: RoleAdmin, want: 1},
		{name: "operator", role: RoleOperator, want: 2},
		{name: "manager", role: RoleManager, want: 3},
		{name: "client", role: RoleClient, want: 32767},
		{name: "пустая роль", role: "", wantErr: ErrInvalidRole},
		{name: "неизвестная роль", role: Role("root"), wantErr: ErrInvalidRole},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.role.ID()

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Role(%q).ID() error = %v, want %v", tt.role, err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("Role(%q).ID() unexpected error = %v", tt.role, err)
			}
			if got != tt.want {
				t.Errorf("Role(%q).ID() = %d, want %d", tt.role, got, tt.want)
			}
		})
	}
}

// TestParseRoleID проверяет обратный перевод идентификатора справочника в код:
// неизвестный id — ошибка (справочник и модель разошлись).
func TestParseRoleID(t *testing.T) {
	tests := []struct {
		name    string
		id      int16
		want    Role
		wantErr error
	}{
		{name: "admin", id: RoleIDAdmin, want: RoleAdmin},
		{name: "operator", id: RoleIDOperator, want: RoleOperator},
		{name: "manager", id: RoleIDManager, want: RoleManager},
		{name: "client", id: RoleIDClient, want: RoleClient},
		{name: "ноль", id: 0, wantErr: ErrInvalidRole},
		{name: "отрицательный", id: -1, wantErr: ErrInvalidRole},
		{name: "неизвестный", id: 42, wantErr: ErrInvalidRole},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRoleID(tt.id)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ParseRoleID(%d) error = %v, want %v", tt.id, err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("ParseRoleID(%d) unexpected error = %v", tt.id, err)
			}
			if got != tt.want {
				t.Errorf("ParseRoleID(%d) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

// TestRoleRoundTrip проверяет, что код и идентификатор переводятся друг в друга
// без потерь: на этом держится запись/чтение tenant_members.role_id.
func TestRoleRoundTrip(t *testing.T) {
	for _, role := range []Role{RoleAdmin, RoleOperator, RoleManager, RoleClient} {
		id, err := role.ID()
		if err != nil {
			t.Fatalf("Role(%q).ID() unexpected error = %v", role, err)
		}

		back, err := ParseRoleID(id)
		if err != nil {
			t.Fatalf("ParseRoleID(%d) unexpected error = %v", id, err)
		}

		if back != role {
			t.Errorf("round trip: %q -> %d -> %q", role, id, back)
		}
	}
}

// TestRoleDefaultID проверяет, что роль по умолчанию совпадает с DEFAULT
// tenant_members.role_id в миграции 00003_tenants.
func TestRoleDefaultID(t *testing.T) {
	if RoleDefaultID != RoleIDClient {
		t.Errorf("RoleDefaultID = %d, want %d", RoleDefaultID, RoleIDClient)
	}

	role, err := ParseRoleID(RoleDefaultID)
	if err != nil {
		t.Fatalf("ParseRoleID(RoleDefaultID) unexpected error = %v", err)
	}

	if role != RoleClient {
		t.Errorf("роль по умолчанию = %q, want %q", role, RoleClient)
	}
}
