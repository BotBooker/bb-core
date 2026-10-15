package model

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Ограничения полей тенанта: совпадают с proto-валидацией (tenant.v1).
const (
	// TenantNameMaxRunes — максимальная длина названия тенанта в рунах.
	TenantNameMaxRunes = 100
	// TenantSlugMaxRunes — максимальная длина slug в рунах.
	TenantSlugMaxRunes = 100
)

// Ошибки валидации тенанта и членства.
var (
	// ErrInvalidTenant — данные тенанта не проходят валидацию модели.
	ErrInvalidTenant = errors.New("tenant is invalid")
	// ErrInvalidTenantMember — данные членства в тенанте не проходят валидацию.
	ErrInvalidTenantMember = errors.New("tenant member is invalid")
)

// TenantStatus — статус тенанта (tenant_status в PostgreSQL, CHECK в SQLite).
type TenantStatus string

// Известные статусы тенанта.
const (
	// TenantStatusActive — действующий тенант.
	TenantStatusActive TenantStatus = "active"
	// TenantStatusSuspended — приостановленный: доступ участников закрыт.
	TenantStatusSuspended TenantStatus = "suspended"
	// TenantStatusDeleted — удалённый (мягкое удаление).
	TenantStatusDeleted TenantStatus = "deleted"
)

// Valid сообщает, что значение статуса известно модели.
func (s TenantStatus) Valid() bool {
	switch s {
	case TenantStatusActive, TenantStatusSuspended, TenantStatusDeleted:
		return true
	default:
		return false
	}
}

// String реализует fmt.Stringer.
func (s TenantStatus) String() string { return string(s) }

// Tenant — тенант (арендатор): организация, объединяющая группу пользователей.
//
// Отдельного поля «владелец» у тенанта нет: владелец — участник с флагом
// is_owner (см. TenantMember), а один пользователь может состоять в нескольких
// тенантах, поэтому состав живёт в tenant_members (M:N). Роль тоже принадлежит
// членству, а не тенанту.
type Tenant struct {
	// ID — канонический UUID (PK tenants.id), генерирует NewTenant.
	ID string
	// Name — человекочитаемое название организации.
	Name string
	// Slug — машинное имя тенанта (уникально, tenants_slug_idx).
	Slug string
	// Status — статус тенанта.
	Status TenantStatus
	// Metadata — произвольные настройки (JSONB в PostgreSQL, JSON-строка в
	// SQLite); nil означает NULL.
	Metadata  []byte
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewTenant создаёт действующий тенант: генерирует идентификатор (UUIDv4),
// нормализует slug, проставляет статус active и время создания/обновления.
//
// Название и slug проверяются теми же правилами, что и proto-запрос
// (tenant.v1): 1..100 рун каждое. Остальные поля (metadata) заполняются
// отдельно, поэтому в конструкторе их нет.
func NewTenant(name, slug string, now time.Time) (*Tenant, error) {
	ts := now.UTC()

	tenant := &Tenant{
		ID:        uuid.NewString(),
		Name:      strings.TrimSpace(name),
		Slug:      NormalizeSlug(slug),
		Status:    TenantStatusActive,
		CreatedAt: ts,
		UpdatedAt: ts,
	}

	if err := tenant.Validate(); err != nil {
		return nil, err
	}

	return tenant, nil
}

// Validate проверяет инварианты хранимой записи тенанта.
func (t *Tenant) Validate() error {
	var errs []error

	if err := validateUUID(t.ID); err != nil {
		errs = append(errs, fmt.Errorf("%w: id: %w", ErrInvalidTenant, err))
	}

	if length := utf8.RuneCountInString(t.Name); length == 0 || length > TenantNameMaxRunes {
		errs = append(errs, fmt.Errorf("%w: name must be 1..%d runes, got %d",
			ErrInvalidTenant, TenantNameMaxRunes, length))
	}

	if err := validateSlug(t.Slug); err != nil {
		errs = append(errs, err)
	}

	if !t.Status.Valid() {
		errs = append(errs, fmt.Errorf("%w: status %q", ErrInvalidTenant, t.Status))
	}

	return errors.Join(errs...)
}

// Active сообщает, что тенант действующий.
func (t Tenant) Active() bool { return t.Status == TenantStatusActive }

// Suspended сообщает, что тенант приостановлен: доступ участников закрыт.
func (t Tenant) Suspended() bool { return t.Status == TenantStatusSuspended }

// Deleted сообщает, что тенант удалён (мягкое удаление).
func (t Tenant) Deleted() bool { return t.Status == TenantStatusDeleted }

// NormalizeSlug приводит slug к виду, в котором он хранится: без пробелов по
// краям и в нижнем регистре. Экспортируется, потому что тот же вид нужен
// репозиторию при поиске тенанта по slug.
func NormalizeSlug(slug string) string {
	return strings.ToLower(strings.TrimSpace(slug))
}

// validateSlug проверяет slug: 1..100 рун и отсутствие пробелов внутри — slug
// попадает в URL и в уникальный индекс tenants_slug_idx.
func validateSlug(slug string) error {
	length := utf8.RuneCountInString(slug)
	if length == 0 || length > TenantSlugMaxRunes {
		return fmt.Errorf("%w: slug must be 1..%d runes, got %d", ErrInvalidTenant, TenantSlugMaxRunes, length)
	}

	if strings.ContainsAny(slug, " \t\n\r") {
		return fmt.Errorf("%w: slug %q contains whitespace", ErrInvalidTenant, slug)
	}

	return nil
}

// TenantMember — членство пользователя в тенанте (tenant_members).
//
// Составной первичный ключ (tenant_id, user_id) не даёт добавить пользователя
// в тенант дважды, поэтому отдельного идентификатора у членства нет. Роль —
// атрибут членства: в разных тенантах у одного пользователя роли независимы.
type TenantMember struct {
	// TenantID — тенант (PK, FK -> tenants.id).
	TenantID string
	// UserID — участник (PK, FK -> users.id).
	UserID string
	// Role — роль участника в этом тенанте (FK -> roles.id).
	Role Role
	// IsOwner — владелец тенанта: ровно один участник на тенант
	// (частичный уникальный индекс tenant_members_owner_idx).
	IsOwner bool
	// JoinedAt — когда пользователь вступил в тенант.
	JoinedAt time.Time
}

// NewTenantMember создаёт членство пользователя в тенанте.
//
// Пустая роль означает роль по умолчанию (client) — то же значение, что стоит
// в DEFAULT tenant_members.role_id, поэтому явно её задавать не обязательно.
func NewTenantMember(tenantID, userID string, role Role, isOwner bool, now time.Time) (*TenantMember, error) {
	if role == "" {
		role = RoleClient
	}

	member := &TenantMember{
		TenantID: tenantID,
		UserID:   userID,
		Role:     role,
		IsOwner:  isOwner,
		JoinedAt: now.UTC(),
	}

	if err := member.Validate(); err != nil {
		return nil, err
	}

	return member, nil
}

// Validate проверяет инварианты членства: идентификаторы — UUID, роль известна
// модели, время вступления задано.
func (m *TenantMember) Validate() error {
	var errs []error

	if err := validateUUID(m.TenantID); err != nil {
		errs = append(errs, fmt.Errorf("%w: tenant_id: %w", ErrInvalidTenantMember, err))
	}

	if err := validateUUID(m.UserID); err != nil {
		errs = append(errs, fmt.Errorf("%w: user_id: %w", ErrInvalidTenantMember, err))
	}

	if !m.Role.Valid() {
		errs = append(errs, fmt.Errorf("%w: role %q: %w", ErrInvalidTenantMember, m.Role, ErrInvalidRole))
	}

	if m.JoinedAt.IsZero() {
		errs = append(errs, fmt.Errorf("%w: joined_at is not set", ErrInvalidTenantMember))
	}

	return errors.Join(errs...)
}
