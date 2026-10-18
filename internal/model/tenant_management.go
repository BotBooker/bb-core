package model

import "encoding/json"

// TenantMembership — членство пользователя вместе с тенантом.
//
// Нужно списку «мои тенанты»: клиенту мало tenant_id из токена, ему нужны slug
// и название, чтобы выбрать тенант и сформировать ссылку. Роль и признак
// владельца остаются атрибутами членства.
type TenantMembership struct {
	// Tenant — тенант, в котором состоит пользователь.
	Tenant Tenant
	// Member — членство пользователя в этом тенанте.
	Member TenantMember
}

// TenantMemberView — участник тенанта вместе с профилем пользователя.
//
// Профиль нужен интерфейсу управления участниками: без имени и email
// администратор видит только UUID. Поля заполняются только списочными
// запросами; в остальных операциях они пусты.
type TenantMemberView struct {
	TenantMember
	// Email — адрес участника.
	Email string
	// FirstName — имя участника.
	FirstName string
	// LastName — фамилия участника.
	LastName string
}

// CreateTenantInput — данные создания тенанта.
//
// Metadata — произвольные настройки (например, description и location для
// публичной страницы). В БД они попадают JSON-объектом.
type CreateTenantInput struct {
	// Name — человекочитаемое название организации.
	Name string
	// Slug — машинное имя тенанта, уникальное.
	Slug string
	// Metadata — произвольные настройки тенанта.
	Metadata map[string]string
}

// MetadataJSON кодирует metadata в JSON: nil превращается в NULL.
func (in CreateTenantInput) MetadataJSON() ([]byte, error) {
	if len(in.Metadata) == 0 {
		return nil, nil
	}

	data, err := json.Marshal(in.Metadata)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// PublicTenant — публичный профиль тенанта.
//
// Внутренний UUID и служебные поля сюда не попадают: публичная страница
// показывает только название, slug, описание и локацию (см. ADR 0001 §8).
type PublicTenant struct {
	// Name — название тенанта.
	Name string
	// Slug — slug тенанта.
	Slug string
	// Description — описание для публичной страницы.
	Description string
	// Location — локация для публичной страницы.
	Location string
}

// PublicObject — публичная карточка объекта аренды.
//
// Идентификаторы, владелец, статус и расписание наружу не отдаются: это
// внутренние данные тенанта (см. ADR 0001 §8).
type PublicObject struct {
	// Title — название объекта.
	Title string
	// Description — описание объекта.
	Description string
	// PriceMinor — цена в минорных единицах валюты.
	PriceMinor int64
	// Currency — код валюты ISO-4217.
	Currency string
	// Address — адрес объекта.
	Address string
}
