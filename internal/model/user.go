// Package model содержит доменные модели BotBooker.
//
// Модели не зависят от транспорта и хранилища: конвертация в proto живёт на
// границе транспорта (internal/serverapi), SQL-запросы — в репозиториях.
// Правила валидации повторяют proto-валидацию (protoc-gen-validate) и
// послабляют её только там, где это нужно для чтения legacy-данных из БД.
package model

import (
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/botbooker/bb-core/pkg/tools"
)

// Ограничения длины полей: совпадают с правилами proto-валидации.
const (
	// NameMaxRunes — максимальная длина имени, фамилии и отчества в рунах.
	NameMaxRunes = 100
	// EmailMaxLen — максимальная длина email в байтах.
	EmailMaxLen = 254
	// EmailLocalPartMaxLen — максимальная длина локальной части email в байтах.
	EmailLocalPartMaxLen = 64
)

// Ошибки валидации модели.
// Все они обёрнуты контекстом поля, поэтому проверять их нужно через errors.Is.
var (
	ErrInvalidUUID       = errors.New("user uuid is invalid")
	ErrInvalidEmail      = errors.New("user email is invalid")
	ErrNameRequired      = errors.New("user name is invalid")
	ErrInvalidMiddleName = errors.New("user middle name is invalid")
	ErrInvalidPhone      = errors.New("user phone number is invalid")
	ErrInvalidGender     = errors.New("user gender is invalid")
	ErrInvalidRole       = errors.New("user role is invalid")
	ErrInvalidBirthDate  = errors.New("user birth date is invalid")
)

// phonePattern — формат E.164 из proto-валидации: ^\+[1-9]\d{1,14}$.
var phonePattern = regexp.MustCompile(`^\+[1-9]\d{1,14}$`)

// Gender — пол пользователя.
type Gender string

// Известные значения пола.
const (
	GenderUnspecified Gender = "unspecified"
	GenderMale        Gender = "male"
	GenderFemale      Gender = "female"
)

// Valid сообщает, что значение пола известно модели.
func (g Gender) Valid() bool {
	switch g {
	case GenderUnspecified, GenderMale, GenderFemale:
		return true
	default:
		return false
	}
}

// String реализует fmt.Stringer.
func (g Gender) String() string { return string(g) }

// ParseGender приводит строку к значению пола.
// Пустая строка — значение по умолчанию, как в proto (GENDER_UNSPECIFIED).
func ParseGender(value string) (Gender, error) {
	gender := Gender(strings.TrimSpace(value))
	if gender == "" {
		return GenderUnspecified, nil
	}

	if !gender.Valid() {
		return "", fmt.Errorf("%w: %q", ErrInvalidGender, tools.SanitizeString(value))
	}

	return gender, nil
}

// Role — роль пользователя в системе.
type Role string

// Известные роли.
const (
	RoleClient   Role = "client"
	RoleOperator Role = "operator"
	RoleAdmin    Role = "admin"
)

// Valid сообщает, что значение роли известно модели.
func (r Role) Valid() bool {
	switch r {
	case RoleClient, RoleOperator, RoleAdmin:
		return true
	default:
		return false
	}
}

// String реализует fmt.Stringer.
func (r Role) String() string { return string(r) }

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

// User — доменная модель пользователя.
//
// Роль и время последнего входа — служебные поля: в proto v1 (UserService)
// они не отдаются, но нужны ролевой модели и аудиту из ERDB.
type User struct {
	// ID — канонический UUID (строка вида 8-4-4-4-12). Это и есть первичный
	// ключ: в БД он хранится в колонке id (UUID в PostgreSQL, TEXT в SQLite),
	// в proto v1 отдаётся полем uuid. Генерирует его NewUser (UUIDv4).
	// BirthDate — календарная дата без времени; nil означает «не задана».
	ID          string
	Email       string
	FirstName   string
	LastName    string
	MiddleName  string
	PhoneNumber string
	Gender      Gender
	BirthDate   *Date
	Role        Role
	CreatedAt   time.Time
	UpdatedAt   time.Time
	LastLoginAt *time.Time
	IsActive    bool
}

// CreateUserInput — данные для создания пользователя.
//
// Роль в API не задаётся: созданный через API пользователь всегда client.
// BirthDate — календарная дата без времени; nil означает «не задана».
type CreateUserInput struct {
	Email       string
	FirstName   string
	LastName    string
	MiddleName  string
	PhoneNumber string
	Gender      Gender
	BirthDate   *Date
}

// UpdateUserInput — частичное обновление: nil означает «не менять».
type UpdateUserInput struct {
	FirstName   *string
	LastName    *string
	MiddleName  *string
	PhoneNumber *string
	Gender      *Gender
	BirthDate   *Date
}

// RegisterInput — данные самостоятельной регистрации: профиль и пароль.
//
// Профиль описывается тем же CreateUserInput, что и создание пользователя
// сотрудником: правила валидации у этих операций одинаковые, отличается
// только наличие пароля.
type RegisterInput struct {
	CreateUserInput
	// Password — открытый пароль; в модели он не сохраняется, только хеш.
	Password string
}

// Validate проверяет профиль и пароль и возвращает все найденные ошибки сразу.
func (in RegisterInput) Validate() error {
	return errors.Join(in.CreateUserInput.Validate(), ValidatePassword(in.Password))
}

// NewUser создаёт активного пользователя: генерирует идентификатор (UUIDv4),
// задаёт роль client, проставляет время создания/обновления и признак
// is_active = true (удаление — это отдельная операция soft delete).
//
// Вход валидируется теми же правилами, что и proto-запрос на создание:
// email обязателен, имена — 1..100 рун, телефон необязателен, но если задан —
// в формате E.164, дата рождения — существующая календарная дата не в будущем.
func NewUser(in CreateUserInput, now time.Time) (*User, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}

	if err := validateBirthDate(in.BirthDate, now); err != nil {
		return nil, err
	}

	gender := in.Gender
	if gender == "" {
		gender = GenderUnspecified
	}

	ts := now.UTC()

	return &User{
		ID:          uuid.NewString(),
		Email:       NormalizeEmail(in.Email),
		FirstName:   in.FirstName,
		LastName:    in.LastName,
		MiddleName:  in.MiddleName,
		PhoneNumber: in.PhoneNumber,
		Gender:      gender,
		BirthDate:   cloneDate(in.BirthDate),
		Role:        RoleClient,
		CreatedAt:   ts,
		UpdatedAt:   ts,
		IsActive:    true,
	}, nil
}

// Validate проверяет вход создания пользователя.
//
// Возвращает все найденные ошибки сразу: несколько проблем удобнее показать
// пользователю одним ответом (errors.Join). Обязательны email, имя и фамилия;
// телефон необязателен, но если задан — должен быть в формате E.164.
func (in CreateUserInput) Validate() error {
	var errs []error

	if err := validateEmail(in.Email); err != nil {
		errs = append(errs, err)
	}

	if err := validateName("first_name", in.FirstName); err != nil {
		errs = append(errs, err)
	}

	if err := validateName("last_name", in.LastName); err != nil {
		errs = append(errs, err)
	}

	if err := validateMiddleName(in.MiddleName); err != nil {
		errs = append(errs, err)
	}

	if err := validatePhone(in.PhoneNumber); err != nil {
		errs = append(errs, err)
	}

	if err := validateGender(in.Gender); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Validate проверяет частичное обновление: непереданные поля (nil) не проверяются.
//
// Пустой телефон (явно переданная пустая строка в proto-wrapper) допустим:
// так поле очищается. Непустое значение проверяется как E.164.
func (in UpdateUserInput) Validate() error {
	var errs []error

	if in.FirstName != nil {
		if err := validateName("first_name", *in.FirstName); err != nil {
			errs = append(errs, err)
		}
	}

	if in.LastName != nil {
		if err := validateName("last_name", *in.LastName); err != nil {
			errs = append(errs, err)
		}
	}

	if in.MiddleName != nil {
		if err := validateMiddleName(*in.MiddleName); err != nil {
			errs = append(errs, err)
		}
	}

	if in.PhoneNumber != nil {
		if err := validatePhone(*in.PhoneNumber); err != nil {
			errs = append(errs, err)
		}
	}

	if in.Gender != nil {
		if err := validateGender(*in.Gender); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// Validate проверяет инварианты хранимой записи пользователя.
//
// Телефон может быть пустым: поле необязательное (в proto — ignore_empty),
// а в ERDB оно тоже не требуется. Проверка «дата рождения не в будущем»
// выполняется в NewUser и ApplyUpdate, где известен текущий момент.
func (u *User) Validate() error {
	var errs []error

	if err := validateUUID(u.ID); err != nil {
		errs = append(errs, err)
	}

	if err := validateEmail(u.Email); err != nil {
		errs = append(errs, err)
	}

	if err := validateName("first_name", u.FirstName); err != nil {
		errs = append(errs, err)
	}

	if err := validateName("last_name", u.LastName); err != nil {
		errs = append(errs, err)
	}

	if err := validateMiddleName(u.MiddleName); err != nil {
		errs = append(errs, err)
	}

	if err := validatePhone(u.PhoneNumber); err != nil {
		errs = append(errs, err)
	}

	if u.Gender == "" || !u.Gender.Valid() {
		errs = append(errs, fmt.Errorf("%w: %q", ErrInvalidGender, u.Gender))
	}

	if !u.Role.Valid() {
		errs = append(errs, fmt.Errorf("%w: %q", ErrInvalidRole, u.Role))
	}

	return errors.Join(errs...)
}

// ApplyUpdate применяет частичное обновление профиля.
//
// Вход проверяется до изменения: если обновление невалидно, пользователь
// остаётся нетронутым. nil-поле означает «не менять», поэтому сбросить
// дату рождения в NULL через этот метод нельзя.
func (u *User) ApplyUpdate(in UpdateUserInput, now time.Time) error {
	if err := in.Validate(); err != nil {
		return err
	}

	if err := validateBirthDate(in.BirthDate, now); err != nil {
		return err
	}

	if in.FirstName != nil {
		u.FirstName = *in.FirstName
	}

	if in.LastName != nil {
		u.LastName = *in.LastName
	}

	if in.MiddleName != nil {
		u.MiddleName = *in.MiddleName
	}

	if in.PhoneNumber != nil {
		u.PhoneNumber = *in.PhoneNumber
	}

	if in.Gender != nil {
		u.Gender = *in.Gender
	}

	if in.BirthDate != nil {
		u.BirthDate = cloneDate(in.BirthDate)
	}

	u.UpdatedAt = now.UTC()

	return u.Validate()
}

// FullName собирает ФИО из заполненных частей: «Иванов Иван Иванович»,
// лишние пробелы не добавляются.
func (u *User) FullName() string {
	parts := make([]string, 0, 3)

	for _, part := range []string{u.LastName, u.FirstName, u.MiddleName} {
		if part != "" {
			parts = append(parts, part)
		}
	}

	return strings.Join(parts, " ")
}

// SetLastLogin фиксирует время последнего входа.
func (u *User) SetLastLogin(at time.Time) {
	ts := at.UTC()
	u.LastLoginAt = &ts
}

// IsAdmin сообщает, что пользователь — администратор.
func (u *User) IsAdmin() bool { return u.Role == RoleAdmin }

// IsStaff сообщает, что пользователь — сотрудник: оператор или администратор.
func (u *User) IsStaff() bool { return u.Role == RoleOperator || u.Role == RoleAdmin }

// CanCreate сообщает, что пользователь может заводить других пользователей.
// Это работа сотрудника: клиент регистрируется сам (POST /auth/register).
func (u *User) CanCreate() bool { return u.IsStaff() }

// CanRead сообщает, что пользователь может смотреть профиль target:
// свой профиль доступен всегда, чужой — сотрудникам.
func (u *User) CanRead(target *User) bool {
	if target == nil {
		return false
	}

	return u.IsStaff() || u.ID == target.ID
}

// CanUpdate сообщает, что пользователь может менять профиль target:
// свой профиль или любой профиль, если он администратор.
func (u *User) CanUpdate(target *User) bool {
	if target == nil {
		return false
	}

	return u.IsAdmin() || u.ID == target.ID
}

// CanDelete сообщает, что пользователь может удалять пользователей: удаление
// доступно только администратору. Профиль удаляемого для этой проверки не нужен.
func (u *User) CanDelete() bool {
	return u.IsAdmin()
}

// ValidateUUID проверяет, что строка — валидный UUID (любой версии).
//
// Экспортируется для слоёв, которые принимают идентификатор от пользователя:
// так невалидный id отклоняется до запроса в базу, где PostgreSQL ответил бы
// ошибкой приведения типа вместо понятного 400.
func ValidateUUID(value string) error { return validateUUID(value) }

// validateUUID проверяет, что идентификатор — валидный UUID (любой версии:
// proto-правило uuid не ограничивает версию, а создаём мы всегда UUIDv4).
// В PostgreSQL тот же формат проверяет тип колонки id (uuid).
func validateUUID(value string) error {
	if _, err := uuid.Parse(value); err != nil {
		return fmt.Errorf("%w: %q", ErrInvalidUUID, value)
	}

	return nil
}

// NormalizeEmail приводит адрес к виду, в котором он хранится: без пробелов по
// краям и в нижнем регистре.
//
// Адрес ищется точным равенством (уникальный индекс users_email_idx), а регистр
// в адресах на практике не значим: без приведения один и тот же адрес можно
// было бы зарегистрировать дважды и не войти, набрав его иначе. Экспортируется
// для входа: там адрес приходит от пользователя и должен совпасть с сохранённым.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// validateEmail повторяет proto-правило email: адрес разбирается net/mail,
// display name не допускается, длина адреса — до 254 байт,
// локальная часть — до 64 байт. Пустая строка невалидна.
func validateEmail(value string) error {
	addr, err := mail.ParseAddress(value)
	if err != nil {
		return fmt.Errorf("%w: %q", ErrInvalidEmail, value)
	}

	if addr.Name != "" || addr.Address != value {
		return fmt.Errorf("%w: %q", ErrInvalidEmail, value)
	}

	if len(addr.Address) > EmailMaxLen {
		return fmt.Errorf("%w: %q is longer than %d bytes", ErrInvalidEmail, value, EmailMaxLen)
	}

	parts := strings.SplitN(addr.Address, "@", 2)
	if len(parts) != 2 || len(parts[0]) == 0 || len(parts[0]) > EmailLocalPartMaxLen {
		return fmt.Errorf("%w: %q", ErrInvalidEmail, value)
	}

	return nil
}

// validateName проверяет имя или фамилию: 1..100 рун (как в proto-валидации,
// длина считается в рунах, поэтому кириллица не «дороже» латиницы).
func validateName(field, value string) error {
	length := utf8.RuneCountInString(value)
	if length == 0 || length > NameMaxRunes {
		return fmt.Errorf("%w: %s must be 1..%d runes, got %d", ErrNameRequired, field, NameMaxRunes, length)
	}

	return nil
}

// validateMiddleName проверяет отчество: необязательное поле до 100 рун.
func validateMiddleName(value string) error {
	if length := utf8.RuneCountInString(value); length > NameMaxRunes {
		return fmt.Errorf("%w: middle_name must be at most %d runes, got %d", ErrInvalidMiddleName, NameMaxRunes, length)
	}

	return nil
}

// validatePhone проверяет телефон в формате E.164.
//
// Пустое значение допустимо: в proto поле помечено ignore_empty, поэтому
// телефон необязателен при создании и его можно очистить при обновлении.
// Непустое значение проверяется правилом pattern ^\+[1-9]\d{1,14}$.
func validatePhone(value string) error {
	if value == "" {
		return nil
	}

	if !phonePattern.MatchString(value) {
		return fmt.Errorf("%w: %q is not E.164", ErrInvalidPhone, value)
	}

	return nil
}

// validateGender проверяет пол: пустое значение допускается и означает
// «не указан» (GENDER_UNSPECIFIED), неизвестное — ошибка.
func validateGender(value Gender) error {
	if value == "" || value.Valid() {
		return nil
	}

	return fmt.Errorf("%w: %q", ErrInvalidGender, value)
}

// validateBirthDate проверяет дату рождения: она должна быть задана, существовать
// в календаре и быть не в будущем. «Сегодня» считается допустимым.
//
// Сравниваются календарные даты: время суток в модели не хранится, поэтому
// перевести проверку в другой пояс нельзя — дата и есть дата.
func validateBirthDate(value *Date, now time.Time) error {
	if value == nil {
		return nil
	}

	if value.IsZero() {
		return fmt.Errorf("%w: date is not set", ErrInvalidBirthDate)
	}

	if !value.Valid() {
		return fmt.Errorf("%w: %s is not a valid date", ErrInvalidBirthDate, value)
	}

	if value.After(NewDate(now)) {
		return fmt.Errorf("%w: %s is in the future", ErrInvalidBirthDate, value)
	}

	return nil
}
