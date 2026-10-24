package service

import (
	"context"

	"github.com/botbooker/bb-core/internal/model"
)

// Principal — тот, от чьего имени выполняется запрос: пользователь, сессия и
// его роль в тенанте сессии.
//
// Транспорт передаёт Principal в сервисы явным аргументом, поэтому проверка
// прав не зависит от транспорта: одни и те же правила работают и в HTTP, и в
// gRPC, а сервисы можно тестировать без middleware.
//
// Роль приходит не из профиля (в users её нет), а из членства в тенанте
// (tenant_members.role_id): у одного пользователя в разных тенантах роли
// независимы, а запрос действует в рамках одного тенанта — тенанта сессии.
//
// Системные флаги профиля (IsSuperuser, IsGlobalOperator) поднимают права над
// ролью членства: такие пользователи администрируют все тенанты.
type Principal struct {
	// User — действующий пользователь: профиль нужен для прав и ответов API.
	User *model.User
	// Token — запись сессии: видно, когда токен выдан, в каком тенанте
	// действует и когда использовался. Тенант сессии — Token.TenantID.
	Token *model.Token
	// Role — роль действующего пользователя в тенанте сессии.
	Role model.Role
}

// ID возвращает идентификатор действующего пользователя.
// Для nil-получателя возвращает пустую строку: так проверка прав не паникует,
// если запрос дошёл без аутентификации.
func (p *Principal) ID() string {
	if p == nil || p.User == nil {
		return ""
	}

	return p.User.ID
}

// TenantID возвращает тенант сессии: пустая строка, если сессии нет.
func (p *Principal) TenantID() string {
	if p == nil || p.Token == nil {
		return ""
	}

	return p.Token.TenantID
}

// IsSuperuser сообщает, что пользователь — суперпользователь, созданный
// bootstrap. Он управляет всеми объектами и тенантами.
func (p *Principal) IsSuperuser() bool {
	return p != nil && p.User != nil && p.User.IsSuperuser
}

// IsGlobalOperator сообщает, что пользователь назначен суперпользователем
// глобальным оператором: он администрирует все тенанты независимо от роли в
// членстве.
func (p *Principal) IsGlobalOperator() bool {
	return p != nil && p.User != nil && p.User.IsGlobalOperator
}

// CanAdminAllTenants сообщает, что пользователь администрирует все тенанты:
// суперпользователь или глобальный оператор.
func (p *Principal) CanAdminAllTenants() bool {
	return p != nil && p.User != nil && p.User.CanAdminAllTenants()
}

// IsAdmin сообщает, что пользователь — администратор: администратор тенанта
// сессии, суперпользователь или глобальный оператор.
//
// Глобальные администраторы получают права admin в любом тенанте, поэтому
// проверка «может управлять» не зависит от роли membership.
func (p *Principal) IsAdmin() bool {
	return p != nil && (p.Role == model.RoleAdmin || p.CanAdminAllTenants())
}

// IsStaff сообщает, что пользователь — сотрудник тенанта: оператор, менеджер
// или администратор. Суперпользователь и глобальный оператор — сотрудники в
// любом тенанте.
func (p *Principal) IsStaff() bool {
	if p == nil {
		return false
	}

	if p.CanAdminAllTenants() {
		return true
	}

	switch p.Role {
	case model.RoleAdmin, model.RoleOperator, model.RoleManager:
		return true
	default:
		return false
	}
}

// CanAdminTenant сообщает, что пользователь может администрировать тенант
// tenantID.
//
// Глобальные администраторы (superuser и global operator) администрируют все
// тенанты. Администратор тенанта — только свой: тенант сессии. Проверка
// вынесена сюда, чтобы tenant-scoped операции не повторяли её в каждом сервисе.
func (p *Principal) CanAdminTenant(tenantID string) bool {
	if p == nil {
		return false
	}

	if p.CanAdminAllTenants() {
		return true
	}

	return p.Role == model.RoleAdmin && tenantID != "" && p.TenantID() == tenantID
}

// CanCreate сообщает, что пользователь может заводить других пользователей.
// Это работа сотрудника: клиент регистрируется сам (POST /auth/register).
func (p *Principal) CanCreate() bool { return p.IsStaff() }

// CanRead сообщает, что пользователь может смотреть профиль target:
// свой профиль доступен всегда, чужой — сотрудникам.
func (p *Principal) CanRead(target *model.User) bool {
	if p == nil || target == nil {
		return false
	}

	return p.IsStaff() || p.ID() == target.ID
}

// CanUpdate сообщает, что пользователь может менять профиль target:
// свой профиль или любой профиль, если он администратор.
func (p *Principal) CanUpdate(target *model.User) bool {
	if p == nil || target == nil {
		return false
	}

	return p.IsAdmin() || p.ID() == target.ID
}

// CanDelete сообщает, что пользователь может удалять аккаунты: таблица users
// общая для всех тенантов, поэтому операция доступна только глобальным
// администраторам (см. UserService.Delete). Исключение из тенанта —
// TenantService.RemoveMember.
func (p *Principal) CanDelete() bool { return p.CanAdminAllTenants() }

// contextKey — собственный тип ключа контекста: с ним значение не сможет
// перезаписать чужой пакет (ключи разных типов не совпадают).
type contextKey struct{}

// principalKey — ключ, по которому Principal лежит в контексте.
var principalKey = contextKey{}

// WithPrincipal кладёт действующего пользователя в контекст.
//
// Контекст — единственный способ передать Principal дальше по цепочке:
// middleware HTTP и интерцептор gRPC не могут вернуть его вызывающему коду, а
// адаптер proto-сервиса получает только контекст.
func WithPrincipal(ctx context.Context, principal *Principal) context.Context {
	return context.WithValue(ctx, principalKey, principal)
}

// PrincipalFromContext возвращает действующего пользователя из контекста.
//
// Второе значение — признак того, что пользователь найден: его отсутствие
// означает, что запрос не прошёл проверку токена, и транспорт отвечает 401,
// не вызывая сервис.
func PrincipalFromContext(ctx context.Context) (*Principal, bool) {
	principal, ok := ctx.Value(principalKey).(*Principal)
	if !ok || principal == nil {
		return nil, false
	}

	return principal, true
}
