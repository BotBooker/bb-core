package service

import (
	"context"
	"testing"
	"time"

	"github.com/botbooker/bb-core/internal/model"
)

// TestPrincipal_ID проверяет идентификаторы действующего пользователя и тенанта
// сессии: обе части Principal могут отсутствовать, и это не должно паниковать.
func TestPrincipal_ID(t *testing.T) {
	user := testAuthUser(t)

	tests := []struct {
		name       string
		principal  *Principal
		wantID     string
		wantTenant string
	}{
		{name: "nil-получатель", principal: nil},
		{
			name:       "без пользователя",
			principal:  &Principal{Token: testSession(user, testNow.Add(time.Hour))},
			wantTenant: testTenantID,
		},
		{name: "без сессии", principal: &Principal{User: user}, wantID: user.ID},
		{
			name:       "полный principal",
			principal:  &Principal{User: user, Token: testSession(user, testNow.Add(time.Hour))},
			wantID:     user.ID,
			wantTenant: testTenantID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.principal.ID(); got != tt.wantID {
				t.Errorf("ID() = %q, want %q", got, tt.wantID)
			}
			if got := tt.principal.TenantID(); got != tt.wantTenant {
				t.Errorf("TenantID() = %q, want %q", got, tt.wantTenant)
			}
		})
	}
}

// TestPrincipal_Roles проверяет разбор роли тенанта: администратор управляет
// тенантом, сотрудник (оператор, менеджер, администратор) работает с профилями,
// клиент видит только себя. Удаление аккаунта роли не даёт: таблица users общая
// для всех тенантов, поэтому CanDelete() есть только у глобальных
// администраторов (см. TestPrincipal_GlobalAdministrators).
func TestPrincipal_Roles(t *testing.T) {
	tests := []struct {
		name      string
		role      model.Role
		admin     bool
		staff     bool
		canCreate bool
	}{
		{name: "администратор", role: model.RoleAdmin, admin: true, staff: true, canCreate: true},
		{name: "оператор", role: model.RoleOperator, staff: true, canCreate: true},
		{name: "менеджер", role: model.RoleManager, staff: true, canCreate: true},
		{name: "клиент", role: model.RoleClient},
		{name: "роль не задана", role: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			principal := &Principal{Role: tt.role}

			if got := principal.IsAdmin(); got != tt.admin {
				t.Errorf("IsAdmin() = %v, want %v", got, tt.admin)
			}
			if got := principal.IsStaff(); got != tt.staff {
				t.Errorf("IsStaff() = %v, want %v", got, tt.staff)
			}
			if got := principal.CanCreate(); got != tt.canCreate {
				t.Errorf("CanCreate() = %v, want %v", got, tt.canCreate)
			}
			if principal.CanDelete() {
				t.Error("CanDelete() = true для роли тенанта, want false")
			}
		})
	}
}

// TestPrincipal_GlobalAdministrators проверяет системный уровень доступа:
// суперпользователь и глобальный оператор администрируют все тенанты, даже если
// роль в членстве — client.
func TestPrincipal_GlobalAdministrators(t *testing.T) {
	tests := []struct {
		name         string
		superuser    bool
		globalOp     bool
		role         model.Role
		wantSuper    bool
		wantGlobalOp bool
		wantAdmin    bool
		wantStaff    bool
	}{
		{
			name:      "суперпользователь с ролью клиента",
			superuser: true,
			role:      model.RoleClient,
			wantSuper: true,
			wantAdmin: true,
			wantStaff: true,
		},
		{
			name:         "глобальный оператор с ролью клиента",
			globalOp:     true,
			role:         model.RoleClient,
			wantGlobalOp: true,
			wantAdmin:    true,
			wantStaff:    true,
		},
		{
			name: "обычный клиент",
			role: model.RoleClient,
		},
		{
			name:      "администратор тенанта",
			role:      model.RoleAdmin,
			wantAdmin: true,
			wantStaff: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := testAuthUser(t)
			user.IsSuperuser = tt.superuser
			user.IsGlobalOperator = tt.globalOp

			principal := &Principal{
				User:  user,
				Token: testSession(user, testNow.Add(time.Hour)),
				Role:  tt.role,
			}

			if got := principal.IsSuperuser(); got != tt.wantSuper {
				t.Errorf("IsSuperuser() = %v, want %v", got, tt.wantSuper)
			}
			if got := principal.IsGlobalOperator(); got != tt.wantGlobalOp {
				t.Errorf("IsGlobalOperator() = %v, want %v", got, tt.wantGlobalOp)
			}
			if got := principal.IsAdmin(); got != tt.wantAdmin {
				t.Errorf("IsAdmin() = %v, want %v", got, tt.wantAdmin)
			}
			if got := principal.IsStaff(); got != tt.wantStaff {
				t.Errorf("IsStaff() = %v, want %v", got, tt.wantStaff)
			}
			// Удалять аккаунты может только глобальный администратор:
			// таблица users общая для всех тенантов.
			if got, want := principal.CanDelete(), tt.superuser || tt.globalOp; got != want {
				t.Errorf("CanDelete() = %v, want %v", got, want)
			}
		})
	}
}

// TestPrincipal_CanAdminTenant проверяет tenant-scoped проверку: администратор
// управляет только тенантом сессии, глобальные администраторы — любым.
func TestPrincipal_CanAdminTenant(t *testing.T) {
	const otherTenantID = "8f0c8f0e-9f60-4e0e-9d5f-1f6f1f6f1f6f"

	user := testAuthUser(t)
	globalOperator := testAuthUser(t)
	globalOperator.IsGlobalOperator = true

	tests := []struct {
		name      string
		principal *Principal
		tenantID  string
		want      bool
	}{
		{
			name:      "администратор тенанта сессии",
			principal: &Principal{User: user, Token: testSession(user, testNow.Add(time.Hour)), Role: model.RoleAdmin},
			tenantID:  testTenantID,
			want:      true,
		},
		{
			name:      "администратор чужого тенанта",
			principal: &Principal{User: user, Token: testSession(user, testNow.Add(time.Hour)), Role: model.RoleAdmin},
			tenantID:  otherTenantID,
		},
		{
			name: "глобальный оператор администрирует любой тенант",
			principal: &Principal{
				User:  globalOperator,
				Token: testSession(globalOperator, testNow.Add(time.Hour)),
				Role:  model.RoleClient,
			},
			tenantID: otherTenantID,
			want:     true,
		},
		{
			name:      "оператор не администрирует тенант",
			principal: &Principal{User: user, Token: testSession(user, testNow.Add(time.Hour)), Role: model.RoleOperator},
			tenantID:  testTenantID,
		},
		{
			name:     "nil-получатель",
			tenantID: testTenantID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.principal.CanAdminTenant(tt.tenantID); got != tt.want {
				t.Errorf("CanAdminTenant() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestPrincipal_CanRead проверяет доступ к профилю: свой профиль доступен
// всегда, чужой — только сотрудникам тенанта.
func TestPrincipal_CanRead(t *testing.T) {
	owner := testAuthUser(t)
	target := testAuthUser(t)

	if owner.ID == target.ID {
		t.Fatal("в тесте нужны два разных пользователя")
	}

	tests := []struct {
		name      string
		principal *Principal
		target    *model.User
		want      bool
	}{
		{name: "клиент читает свой профиль", principal: &Principal{User: owner, Role: model.RoleClient}, target: owner, want: true},
		{name: "клиент не читает чужой", principal: &Principal{User: owner, Role: model.RoleClient}, target: target},
		{name: "оператор читает чужой", principal: &Principal{User: owner, Role: model.RoleOperator}, target: target, want: true},
		{name: "менеджер читает чужой", principal: &Principal{User: owner, Role: model.RoleManager}, target: target, want: true},
		{name: "администратор читает чужой", principal: &Principal{User: owner, Role: model.RoleAdmin}, target: target, want: true},
		{name: "nil-получатель", principal: nil, target: owner},
		{name: "без цели", principal: &Principal{User: owner, Role: model.RoleAdmin}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.principal.CanRead(tt.target); got != tt.want {
				t.Errorf("CanRead() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestPrincipal_CanUpdate проверяет правку профиля: свой профиль может менять
// каждый, чужой — только администратор тенанта.
func TestPrincipal_CanUpdate(t *testing.T) {
	owner := testAuthUser(t)
	target := testAuthUser(t)

	tests := []struct {
		name      string
		principal *Principal
		target    *model.User
		want      bool
	}{
		{name: "клиент правит свой профиль", principal: &Principal{User: owner, Role: model.RoleClient}, target: owner, want: true},
		{name: "клиент не правит чужой", principal: &Principal{User: owner, Role: model.RoleClient}, target: target},
		{name: "оператор не правит чужой", principal: &Principal{User: owner, Role: model.RoleOperator}, target: target},
		{name: "администратор правит чужой", principal: &Principal{User: owner, Role: model.RoleAdmin}, target: target, want: true},
		{name: "nil-получатель", principal: nil, target: owner},
		{name: "без цели", principal: &Principal{User: owner, Role: model.RoleAdmin}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.principal.CanUpdate(tt.target); got != tt.want {
				t.Errorf("CanUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestPrincipalContext проверяет перенос действующего пользователя через
// контекст: так его получают адаптеры proto-сервисов, которым транспорт не
// может передать Principal аргументом.
func TestPrincipalContext(t *testing.T) {
	t.Run("без principal в контексте", func(t *testing.T) {
		principal, ok := PrincipalFromContext(context.Background())
		if ok || principal != nil {
			t.Errorf("PrincipalFromContext() = %v/%v, want nil/false", principal, ok)
		}
	})

	t.Run("nil-значение не считается добавленным", func(t *testing.T) {
		ctx := WithPrincipal(context.Background(), nil)

		principal, ok := PrincipalFromContext(ctx)
		if ok || principal != nil {
			t.Errorf("PrincipalFromContext() = %v/%v, want nil/false", principal, ok)
		}
	})

	t.Run("возвращает добавленного пользователя", func(t *testing.T) {
		want := testPrincipal(t, model.RoleOperator)
		ctx := WithPrincipal(context.Background(), want)

		got, ok := PrincipalFromContext(ctx)
		if !ok {
			t.Fatal("PrincipalFromContext() ok = false, want true")
		}
		if got != want {
			t.Errorf("PrincipalFromContext() = %+v, want %+v", got, want)
		}
	})

	t.Run("свой ключ контекста не конфликтует с чужим", func(t *testing.T) {
		// Значение чужого пакета под тем же строковым ключом не должно
		// подменять действующего пользователя.
		ctx := context.WithValue(context.Background(), "principal", testPrincipal(t, model.RoleAdmin))

		if _, ok := PrincipalFromContext(ctx); ok {
			t.Error("PrincipalFromContext() нашёл principal по чужому ключу")
		}
	})
}
