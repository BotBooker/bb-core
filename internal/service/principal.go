package service

import (
	"context"

	"github.com/botbooker/bb-core/internal/model"
)

// Principal — тот, от чьего имени выполняется запрос: пользователь и токен, по
// которому он вошёл.
//
// Транспорт передаёт Principal в сервисы явным аргументом, поэтому проверка
// прав не зависит от транспорта: одни и те же правила работают и в HTTP, и в
// gRPC, а сервисы можно тестировать без middleware.
type Principal struct {
	// User — действующий пользователь: профиль и роль нужны для проверки прав.
	User *model.User
	// Token — запись сессии: видно, когда токен выдан и когда использовался.
	Token *model.Token
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

// Role возвращает роль действующего пользователя.
func (p *Principal) Role() model.Role {
	if p == nil || p.User == nil {
		return ""
	}

	return p.User.Role
}

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
