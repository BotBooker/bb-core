package model

import (
	"fmt"
	"strings"
)

// Role — роль пользователя в тенанте: код справочника roles (колонка code).
//
// Роль не хранится в профиле (users) и не является его атрибутом: она
// принадлежит членству (tenant_members.role_id), поэтому у одного пользователя
// в разных тенантах роли независимы. В таблицах роль хранится числовым внешним
// ключом на справочник roles, а на границе API (и в claim role JWT) — строковым
// кодом: соответствие «код <-> id» задано здесь и совпадает с seed-значениями
// миграции 00001_foundation.
type Role string

// Известные роли: значения совпадают с roles.code.
const (
	// RoleAdmin — администратор тенанта.
	RoleAdmin Role = "admin"
	// RoleOperator — оператор тенанта.
	RoleOperator Role = "operator"
	// RoleManager — менеджер тенанта.
	RoleManager Role = "manager"
	// RoleClient — пользователь (клиент) тенанта.
	RoleClient Role = "client"
)

// Идентификаторы ролей в справочнике roles.
//
// Значения назначены миграцией явно и одинаковы в обоих диалектах, потому что
// на них ссылаются DEFAULT у tenant_members.role_id (RoleDefaultID). Хранятся
// здесь, чтобы модель умела переводить код в id и обратно без запроса в базу:
// справочник стабилен и расширяется миграцией.
const (
	RoleIDAdmin    int16 = 1
	RoleIDOperator int16 = 2
	RoleIDManager  int16 = 3
	RoleIDClient   int16 = 32767
)

// RoleDefaultID — роль по умолчанию для нового членства: совпадает с
// DEFAULT tenant_members.role_id в миграции 00003_tenants.
const RoleDefaultID = RoleIDClient

// Valid сообщает, что значение роли известно модели.
func (r Role) Valid() bool {
	switch r {
	case RoleAdmin, RoleOperator, RoleManager, RoleClient:
		return true
	default:
		return false
	}
}

// String реализует fmt.Stringer.
func (r Role) String() string { return string(r) }

// ID возвращает идентификатор роли в справочнике roles.
//
// Неизвестная роль — ошибка: записать в БД «роль, которой нет в справочнике»
// нельзя, внешний ключ это отклонит, а понятнее отказать до запроса.
func (r Role) ID() (int16, error) {
	switch r {
	case RoleAdmin:
		return RoleIDAdmin, nil
	case RoleOperator:
		return RoleIDOperator, nil
	case RoleManager:
		return RoleIDManager, nil
	case RoleClient:
		return RoleIDClient, nil
	default:
		return 0, fmt.Errorf("%w: %q", ErrInvalidRole, r)
	}
}

// ParseRole приводит строку к значению роли.
// Пустая строка — роль по умолчанию (client).
func ParseRole(value string) (Role, error) {
	role := Role(strings.TrimSpace(value))
	if role == "" {
		return RoleClient, nil
	}

	if !role.Valid() {
		return "", fmt.Errorf("%w: %q", ErrInvalidRole, value)
	}

	return role, nil
}

// ParseRoleID переводит идентификатор из справочника roles в код роли.
//
// Используется репозиториями при чтении tenant_members.role_id и
// user_tokens.role_id: неизвестный id означает, что справочник и код
// разошлись, и это ошибка, а не «пустая роль».
func ParseRoleID(id int16) (Role, error) {
	switch id {
	case RoleIDAdmin:
		return RoleAdmin, nil
	case RoleIDOperator:
		return RoleOperator, nil
	case RoleIDManager:
		return RoleManager, nil
	case RoleIDClient:
		return RoleClient, nil
	default:
		return "", fmt.Errorf("%w: id %d is not in the roles reference table", ErrInvalidRole, id)
	}
}
