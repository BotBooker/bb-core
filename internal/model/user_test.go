package model

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// testNow — фиксированное «сейчас»: тесты валидации даты рождения
// не должны зависеть от реальных часов.
var testNow = time.Date(2026, time.October, 12, 10, 0, 0, 0, time.UTC)

// validCreateInput возвращает валидный вход создания: тест портит только то поле,
// которое проверяет.
func validCreateInput() CreateUserInput {
	return CreateUserInput{
		Email:       "ivan@example.com",
		FirstName:   "Иван",
		LastName:    "Иванов",
		MiddleName:  "Иванович",
		PhoneNumber: "+79991234567",
		Gender:      GenderMale,
		BirthDate:   datePtr(1990, time.May, 1),
	}
}

// datePtr возвращает указатель на календарную дату.
func datePtr(year int, month time.Month, day int) *Date {
	return &Date{Year: year, Month: month, Day: day}
}

// strPtr возвращает указатель на строку: вход частичного обновления полей.
func strPtr(value string) *string { return &value }

// genderPtr возвращает указатель на пол.
func genderPtr(value Gender) *Gender { return &value }

func TestNewUser(t *testing.T) {
	t.Run("заполняет поля и служебные значения", func(t *testing.T) {
		in := validCreateInput()

		user, err := NewUser(in, testNow)
		if err != nil {
			t.Fatalf("NewUser() unexpected error = %v", err)
		}

		if user.Email != in.Email {
			t.Errorf("Email = %q, want %q", user.Email, in.Email)
		}
		if user.FirstName != in.FirstName || user.LastName != in.LastName || user.MiddleName != in.MiddleName {
			t.Errorf("ФИО = %q %q %q, want %q %q %q",
				user.FirstName, user.LastName, user.MiddleName, in.FirstName, in.LastName, in.MiddleName)
		}
		if user.PhoneNumber != in.PhoneNumber {
			t.Errorf("PhoneNumber = %q, want %q", user.PhoneNumber, in.PhoneNumber)
		}
		if user.Gender != GenderMale {
			t.Errorf("Gender = %q, want %q", user.Gender, GenderMale)
		}
		if user.Role != RoleClient {
			t.Errorf("Role = %q, want %q", user.Role, RoleClient)
		}
		if user.BirthDate == nil || *user.BirthDate != *in.BirthDate {
			t.Errorf("BirthDate = %v, want %v", user.BirthDate, in.BirthDate)
		}
		if !user.CreatedAt.Equal(testNow) || !user.UpdatedAt.Equal(testNow) {
			t.Errorf("CreatedAt/UpdatedAt = %v/%v, want %v", user.CreatedAt, user.UpdatedAt, testNow)
		}
		if user.LastLoginAt != nil {
			t.Errorf("LastLoginAt = %v, want nil", user.LastLoginAt)
		}
		if !user.IsActive {
			t.Error("IsActive = false, want true: NewUser создаёт активного пользователя")
		}
		if err := user.Validate(); err != nil {
			t.Errorf("Validate() unexpected error = %v", err)
		}
	})

	t.Run("генерирует UUIDv4", func(t *testing.T) {
		user, err := NewUser(validCreateInput(), testNow)
		if err != nil {
			t.Fatalf("NewUser() unexpected error = %v", err)
		}

		parsed, err := uuid.Parse(user.ID)
		if err != nil {
			t.Fatalf("uuid.Parse(%q) unexpected error = %v", user.ID, err)
		}
		if parsed.Version() != 4 {
			t.Errorf("версия uuid = %d, want 4", parsed.Version())
		}
	})

	t.Run("разные пользователи получают разные UUID", func(t *testing.T) {
		first, err := NewUser(validCreateInput(), testNow)
		if err != nil {
			t.Fatalf("NewUser() unexpected error = %v", err)
		}

		second, err := NewUser(validCreateInput(), testNow)
		if err != nil {
			t.Fatalf("NewUser() unexpected error = %v", err)
		}

		if first.ID == second.ID {
			t.Errorf("UUID совпадают: %q", first.ID)
		}
	})

	t.Run("пустой пол превращается в unspecified", func(t *testing.T) {
		in := validCreateInput()
		in.Gender = ""

		user, err := NewUser(in, testNow)
		if err != nil {
			t.Fatalf("NewUser() unexpected error = %v", err)
		}

		if user.Gender != GenderUnspecified {
			t.Errorf("Gender = %q, want %q", user.Gender, GenderUnspecified)
		}
	})

	t.Run("без отчества и даты рождения", func(t *testing.T) {
		in := validCreateInput()
		in.MiddleName = ""
		in.BirthDate = nil

		user, err := NewUser(in, testNow)
		if err != nil {
			t.Fatalf("NewUser() unexpected error = %v", err)
		}

		if user.BirthDate != nil {
			t.Errorf("BirthDate = %v, want nil", user.BirthDate)
		}
		if user.FullName() != "Иванов Иван" {
			t.Errorf("FullName() = %q, want %q", user.FullName(), "Иванов Иван")
		}
	})

	t.Run("телефон необязателен", func(t *testing.T) {
		in := validCreateInput()
		in.PhoneNumber = ""

		user, err := NewUser(in, testNow)
		if err != nil {
			t.Fatalf("NewUser() unexpected error = %v", err)
		}

		if user.PhoneNumber != "" {
			t.Errorf("PhoneNumber = %q, want пустую строку", user.PhoneNumber)
		}
		if err := user.Validate(); err != nil {
			t.Errorf("Validate() unexpected error = %v", err)
		}
	})

	t.Run("дата рождения сегодня допустима", func(t *testing.T) {
		in := validCreateInput()
		in.BirthDate = datePtr(testNow.Year(), testNow.Month(), testNow.Day())

		if _, err := NewUser(in, testNow); err != nil {
			t.Fatalf("NewUser() unexpected error = %v", err)
		}
	})

	t.Run("дата рождения в будущем отклоняется", func(t *testing.T) {
		in := validCreateInput()
		in.BirthDate = datePtr(testNow.Year()+1, testNow.Month(), testNow.Day())

		_, err := NewUser(in, testNow)
		if !errors.Is(err, ErrInvalidBirthDate) {
			t.Fatalf("NewUser() error = %v, want %v", err, ErrInvalidBirthDate)
		}
	})

	t.Run("несуществующая дата отклоняется", func(t *testing.T) {
		in := validCreateInput()
		in.BirthDate = datePtr(2000, time.February, 31)

		_, err := NewUser(in, testNow)
		if !errors.Is(err, ErrInvalidBirthDate) {
			t.Fatalf("NewUser() error = %v, want %v", err, ErrInvalidBirthDate)
		}

		// В тексте ошибки должна быть отправленная дата, а не нормализованная
		// (иначе клиент увидел бы «2000-03-02» и не понял, что не так).
		if !strings.Contains(err.Error(), "2000-02-31") {
			t.Errorf("текст ошибки = %q, want упоминание %q", err, "2000-02-31")
		}
	})

	t.Run("незаданная дата отклоняется понятным текстом", func(t *testing.T) {
		in := validCreateInput()
		in.BirthDate = datePtr(0, 0, 0)

		_, err := NewUser(in, testNow)
		if !errors.Is(err, ErrInvalidBirthDate) {
			t.Fatalf("NewUser() error = %v, want %v", err, ErrInvalidBirthDate)
		}

		if !strings.Contains(err.Error(), "date is not set") {
			t.Errorf("текст ошибки = %q, want упоминание, что дата не задана", err)
		}
	})

	t.Run("дата не разделяется с входом", func(t *testing.T) {
		in := validCreateInput()

		user, err := NewUser(in, testNow)
		if err != nil {
			t.Fatalf("NewUser() unexpected error = %v", err)
		}

		// Правка входных данных не должна менять созданного пользователя.
		*in.BirthDate = Date{Year: 1900, Month: time.January, Day: 1}

		if user.BirthDate.String() != "1990-05-01" {
			t.Errorf("BirthDate = %q, want %q", user.BirthDate, "1990-05-01")
		}
	})

	t.Run("невалидный вход не создаёт пользователя", func(t *testing.T) {
		in := validCreateInput()
		in.Email = "not-an-email"

		user, err := NewUser(in, testNow)
		if !errors.Is(err, ErrInvalidEmail) {
			t.Fatalf("NewUser() error = %v, want %v", err, ErrInvalidEmail)
		}
		if user != nil {
			t.Errorf("NewUser() = %v, want nil", user)
		}
	})

	t.Run("время переводится в UTC", func(t *testing.T) {
		zone := time.FixedZone("MSK", 3*60*60)
		now := testNow.In(zone)

		user, err := NewUser(validCreateInput(), now)
		if err != nil {
			t.Fatalf("NewUser() unexpected error = %v", err)
		}

		if user.CreatedAt.Location() != time.UTC {
			t.Errorf("CreatedAt location = %v, want UTC", user.CreatedAt.Location())
		}
		if !user.CreatedAt.Equal(now) {
			t.Errorf("CreatedAt = %v, want %v", user.CreatedAt, now)
		}
	})
}

// newValidUser создаёт валидного пользователя для тестов инвариантов записи.
func newValidUser(t *testing.T) *User {
	t.Helper()

	user, err := NewUser(validCreateInput(), testNow)
	if err != nil {
		t.Fatalf("NewUser() unexpected error = %v", err)
	}

	return user
}

func TestCreateUserInput_Validate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(in *CreateUserInput)
		wantErr error
	}{
		{
			name: "валидный вход",
		},
		{
			name: "без отчества и даты рождения",
			mutate: func(in *CreateUserInput) {
				in.MiddleName = ""
				in.BirthDate = nil
			},
		},
		{
			name: "пол не указан",
			mutate: func(in *CreateUserInput) {
				in.Gender = ""
			},
		},
		{
			name: "имя ровно 100 рун",
			mutate: func(in *CreateUserInput) {
				in.FirstName = strings.Repeat("и", NameMaxRunes)
			},
		},
		{
			name: "пустой email",
			mutate: func(in *CreateUserInput) {
				in.Email = ""
			},
			wantErr: ErrInvalidEmail,
		},
		{
			name: "email без собаки",
			mutate: func(in *CreateUserInput) {
				in.Email = "ivan.example.com"
			},
			wantErr: ErrInvalidEmail,
		},
		{
			name: "email с display name",
			mutate: func(in *CreateUserInput) {
				in.Email = "Иван <ivan@example.com>"
			},
			wantErr: ErrInvalidEmail,
		},
		{
			name: "email длиннее 254 байт",
			mutate: func(in *CreateUserInput) {
				in.Email = strings.Repeat("a", 60) + "@" + strings.Repeat("b", 190) + ".com"
			},
			wantErr: ErrInvalidEmail,
		},
		{
			name: "локальная часть email 65 байт",
			mutate: func(in *CreateUserInput) {
				in.Email = strings.Repeat("a", 65) + "@example.com"
			},
			wantErr: ErrInvalidEmail,
		},
		{
			name: "пустое имя",
			mutate: func(in *CreateUserInput) {
				in.FirstName = ""
			},
			wantErr: ErrNameRequired,
		},
		{
			name: "имя 101 руна",
			mutate: func(in *CreateUserInput) {
				in.FirstName = strings.Repeat("и", NameMaxRunes+1)
			},
			wantErr: ErrNameRequired,
		},
		{
			name: "пустая фамилия",
			mutate: func(in *CreateUserInput) {
				in.LastName = ""
			},
			wantErr: ErrNameRequired,
		},
		{
			name: "отчество 101 руна",
			mutate: func(in *CreateUserInput) {
				in.MiddleName = strings.Repeat("и", NameMaxRunes+1)
			},
			wantErr: ErrInvalidMiddleName,
		},
		{
			name: "пустой телефон допустим",
			mutate: func(in *CreateUserInput) {
				in.PhoneNumber = ""
			},
		},
		{
			name: "телефон без плюса",
			mutate: func(in *CreateUserInput) {
				in.PhoneNumber = "89991234567"
			},
			wantErr: ErrInvalidPhone,
		},
		{
			name: "телефон с форматированием",
			mutate: func(in *CreateUserInput) {
				in.PhoneNumber = "+7 (999) 123-45-67"
			},
			wantErr: ErrInvalidPhone,
		},
		{
			name: "телефон длиннее 15 цифр",
			mutate: func(in *CreateUserInput) {
				in.PhoneNumber = "+1234567890123456"
			},
			wantErr: ErrInvalidPhone,
		},
		{
			name: "неизвестный пол",
			mutate: func(in *CreateUserInput) {
				in.Gender = Gender("wat")
			},
			wantErr: ErrInvalidGender,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validCreateInput()
			if tt.mutate != nil {
				tt.mutate(&in)
			}

			err := in.Validate()

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

// TestCreateUserInput_ValidateJoinsErrors проверяет, что валидация возвращает
// все найденные ошибки сразу, а не первую попавшуюся.
func TestCreateUserInput_ValidateJoinsErrors(t *testing.T) {
	in := validCreateInput()
	in.Email = ""
	in.FirstName = ""
	in.PhoneNumber = "89991234567"

	err := in.Validate()

	for _, wantErr := range []error{ErrInvalidEmail, ErrNameRequired, ErrInvalidPhone} {
		if !errors.Is(err, wantErr) {
			t.Errorf("errors.Is(%v, %v) = false, want true", err, wantErr)
		}
	}
}

func TestUser_Validate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(user *User)
		wantErr error
	}{
		{name: "валидный пользователь"},
		{name: "пустой телефон допустим", mutate: func(u *User) { u.PhoneNumber = "" }},
		{name: "без даты рождения", mutate: func(u *User) { u.BirthDate = nil }},
		{name: "пустой uuid", mutate: func(u *User) { u.ID = "" }, wantErr: ErrInvalidUUID},
		{name: "uuid не парсится", mutate: func(u *User) { u.ID = "not-a-uuid" }, wantErr: ErrInvalidUUID},
		{name: "пустой email", mutate: func(u *User) { u.Email = "" }, wantErr: ErrInvalidEmail},
		{name: "пустое имя", mutate: func(u *User) { u.FirstName = "" }, wantErr: ErrNameRequired},
		{
			name:    "имя 101 руна",
			mutate:  func(u *User) { u.FirstName = strings.Repeat("и", NameMaxRunes+1) },
			wantErr: ErrNameRequired,
		},
		{
			name:    "отчество 101 руна",
			mutate:  func(u *User) { u.MiddleName = strings.Repeat("и", NameMaxRunes+1) },
			wantErr: ErrInvalidMiddleName,
		},
		{name: "телефон не E.164", mutate: func(u *User) { u.PhoneNumber = "89991234567" }, wantErr: ErrInvalidPhone},
		{name: "пол не задан", mutate: func(u *User) { u.Gender = "" }, wantErr: ErrInvalidGender},
		{name: "неизвестный пол", mutate: func(u *User) { u.Gender = Gender("wat") }, wantErr: ErrInvalidGender},
		{name: "роль не задана", mutate: func(u *User) { u.Role = "" }, wantErr: ErrInvalidRole},
		{name: "неизвестная роль", mutate: func(u *User) { u.Role = Role("root") }, wantErr: ErrInvalidRole},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := newValidUser(t)
			if tt.mutate != nil {
				tt.mutate(user)
			}

			err := user.Validate()

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

func TestUpdateUserInput_Validate(t *testing.T) {
	tests := []struct {
		name    string
		in      UpdateUserInput
		wantErr error
	}{
		{name: "пустое обновление"},
		{
			name: "все поля заданы",
			in: UpdateUserInput{
				FirstName:   strPtr("Пётр"),
				LastName:    strPtr("Петров"),
				MiddleName:  strPtr(""),
				PhoneNumber: strPtr("+79990000000"),
				Gender:      genderPtr(GenderFemale),
				BirthDate:   datePtr(2000, time.January, 2),
			},
		},
		{name: "пустое имя", in: UpdateUserInput{FirstName: strPtr("")}, wantErr: ErrNameRequired},
		{
			name:    "имя 101 руна",
			in:      UpdateUserInput{FirstName: strPtr(strings.Repeat("и", NameMaxRunes+1))},
			wantErr: ErrNameRequired,
		},
		{
			name:    "отчество 101 руна",
			in:      UpdateUserInput{MiddleName: strPtr(strings.Repeat("и", NameMaxRunes+1))},
			wantErr: ErrInvalidMiddleName,
		},
		{name: "телефон очищается пустой строкой", in: UpdateUserInput{PhoneNumber: strPtr("")}},
		{
			name:    "телефон с форматированием",
			in:      UpdateUserInput{PhoneNumber: strPtr("+7 999 123-45-67")},
			wantErr: ErrInvalidPhone,
		},
		{name: "неизвестный пол", in: UpdateUserInput{Gender: genderPtr(Gender("wat"))}, wantErr: ErrInvalidGender},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.in.Validate()

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

func TestApplyUpdate(t *testing.T) {
	t.Run("пустое обновление не меняет поля", func(t *testing.T) {
		user := newValidUser(t)
		before := *user
		later := testNow.Add(time.Hour)

		if err := user.ApplyUpdate(UpdateUserInput{}, later); err != nil {
			t.Fatalf("ApplyUpdate() unexpected error = %v", err)
		}

		if user.FirstName != before.FirstName || user.LastName != before.LastName || user.MiddleName != before.MiddleName {
			t.Errorf("ФИО изменилось: %q %q %q", user.FirstName, user.LastName, user.MiddleName)
		}
		if user.PhoneNumber != before.PhoneNumber || user.Gender != before.Gender {
			t.Errorf("поля изменились: %q %q", user.PhoneNumber, user.Gender)
		}
		if user.BirthDate == nil || *user.BirthDate != *before.BirthDate {
			t.Errorf("BirthDate = %v, want %v", user.BirthDate, before.BirthDate)
		}
		if !user.CreatedAt.Equal(before.CreatedAt) || user.Role != before.Role || user.Email != before.Email {
			t.Errorf("служебные поля изменились: %v %q %q", user.CreatedAt, user.Role, user.Email)
		}
		if !user.UpdatedAt.Equal(later) {
			t.Errorf("UpdatedAt = %v, want %v", user.UpdatedAt, later)
		}
	})

	t.Run("меняет переданные поля", func(t *testing.T) {
		user := newValidUser(t)
		later := testNow.Add(2 * time.Hour)

		in := UpdateUserInput{
			FirstName:   strPtr("Пётр"),
			LastName:    strPtr("Петров"),
			MiddleName:  strPtr(""),
			PhoneNumber: strPtr("+79990000000"),
			Gender:      genderPtr(GenderFemale),
			BirthDate:   datePtr(2000, time.January, 2),
		}

		if err := user.ApplyUpdate(in, later); err != nil {
			t.Fatalf("ApplyUpdate() unexpected error = %v", err)
		}

		if user.FirstName != "Пётр" || user.LastName != "Петров" || user.MiddleName != "" {
			t.Errorf("ФИО = %q %q %q, want Пётр Петров \"\"", user.FirstName, user.LastName, user.MiddleName)
		}
		if user.PhoneNumber != "+79990000000" {
			t.Errorf("PhoneNumber = %q, want %q", user.PhoneNumber, "+79990000000")
		}
		if user.Gender != GenderFemale {
			t.Errorf("Gender = %q, want %q", user.Gender, GenderFemale)
		}
		if user.BirthDate == nil || *user.BirthDate != *in.BirthDate {
			t.Errorf("BirthDate = %v, want %v", user.BirthDate, in.BirthDate)
		}
		if !user.UpdatedAt.Equal(later) {
			t.Errorf("UpdatedAt = %v, want %v", user.UpdatedAt, later)
		}
		if !user.CreatedAt.Equal(testNow) {
			t.Errorf("CreatedAt = %v, want %v", user.CreatedAt, testNow)
		}
		if user.Role != RoleClient || user.Email != "ivan@example.com" {
			t.Errorf("служебные поля изменились: %q %q", user.Role, user.Email)
		}
	})

	t.Run("ошибка не меняет пользователя", func(t *testing.T) {
		user := newValidUser(t)
		before := *user

		err := user.ApplyUpdate(UpdateUserInput{FirstName: strPtr("")}, testNow.Add(time.Hour))
		if !errors.Is(err, ErrNameRequired) {
			t.Fatalf("ApplyUpdate() error = %v, want %v", err, ErrNameRequired)
		}

		if user.FirstName != before.FirstName {
			t.Errorf("FirstName = %q, want %q", user.FirstName, before.FirstName)
		}
		if !user.UpdatedAt.Equal(before.UpdatedAt) {
			t.Errorf("UpdatedAt = %v, want %v", user.UpdatedAt, before.UpdatedAt)
		}
	})

	t.Run("дата рождения в будущем отклоняется", func(t *testing.T) {
		user := newValidUser(t)
		before := *user

		in := UpdateUserInput{BirthDate: datePtr(testNow.Year()+1, testNow.Month(), testNow.Day())}

		err := user.ApplyUpdate(in, testNow)
		if !errors.Is(err, ErrInvalidBirthDate) {
			t.Fatalf("ApplyUpdate() error = %v, want %v", err, ErrInvalidBirthDate)
		}
		if user.BirthDate == nil || *user.BirthDate != *before.BirthDate {
			t.Errorf("BirthDate = %v, want %v", user.BirthDate, before.BirthDate)
		}
	})

	t.Run("несуществующая дата отклоняется", func(t *testing.T) {
		user := newValidUser(t)
		before := *user

		in := UpdateUserInput{BirthDate: datePtr(2000, time.February, 31)}

		err := user.ApplyUpdate(in, testNow)
		if !errors.Is(err, ErrInvalidBirthDate) {
			t.Fatalf("ApplyUpdate() error = %v, want %v", err, ErrInvalidBirthDate)
		}
		if user.BirthDate == nil || *user.BirthDate != *before.BirthDate {
			t.Errorf("BirthDate = %v, want %v", user.BirthDate, before.BirthDate)
		}
	})

	t.Run("дата рождения не хранит время суток", func(t *testing.T) {
		user := newValidUser(t)
		// 2000-01-02T00:30+03:00 — это 2000-01-01 по UTC.
		date := NewDate(time.Date(2000, time.January, 2, 0, 30, 0, 0, time.FixedZone("MSK", 3*60*60)))

		if err := user.ApplyUpdate(UpdateUserInput{BirthDate: &date}, testNow); err != nil {
			t.Fatalf("ApplyUpdate() unexpected error = %v", err)
		}

		if user.BirthDate == nil || *user.BirthDate != date {
			t.Fatalf("BirthDate = %v, want %v", user.BirthDate, date)
		}
		if user.BirthDate.String() != "2000-01-01" {
			t.Errorf("BirthDate = %q, want %q", user.BirthDate, "2000-01-01")
		}
		if user.BirthDate.Time().Location() != time.UTC {
			t.Errorf("BirthDate.Time() location = %v, want UTC", user.BirthDate.Time().Location())
		}
	})
}

func TestFullName(t *testing.T) {
	tests := []struct {
		name       string
		lastName   string
		firstName  string
		middleName string
		want       string
	}{
		{
			name:       "с отчеством",
			lastName:   "Иванов",
			firstName:  "Иван",
			middleName: "Иванович",
			want:       "Иванов Иван Иванович",
		},
		{name: "без отчества", lastName: "Иванов", firstName: "Иван", want: "Иванов Иван"},
		{name: "только имя", firstName: "Иван", want: "Иван"},
		{name: "пустые поля", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &User{LastName: tt.lastName, FirstName: tt.firstName, MiddleName: tt.middleName}

			if got := user.FullName(); got != tt.want {
				t.Errorf("FullName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSetLastLogin(t *testing.T) {
	user := newValidUser(t)
	if user.LastLoginAt != nil {
		t.Fatalf("LastLoginAt = %v, want nil", user.LastLoginAt)
	}

	zone := time.FixedZone("MSK", 3*60*60)
	at := testNow.In(zone)
	user.SetLastLogin(at)

	if user.LastLoginAt == nil {
		t.Fatal("LastLoginAt = nil, want время входа")
	}
	if !user.LastLoginAt.Equal(at) {
		t.Errorf("LastLoginAt = %v, want %v", user.LastLoginAt, at)
	}
	if user.LastLoginAt.Location() != time.UTC {
		t.Errorf("LastLoginAt location = %v, want UTC", user.LastLoginAt.Location())
	}
}

func TestParseGender(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    Gender
		wantErr error
	}{
		{name: "male", value: "male", want: GenderMale},
		{name: "female", value: "female", want: GenderFemale},
		{name: "unspecified", value: "unspecified", want: GenderUnspecified},
		{name: "значение с пробелами", value: " female ", want: GenderFemale},
		{name: "пустая строка", value: "", want: GenderUnspecified},
		{name: "только пробелы", value: "   ", want: GenderUnspecified},
		{name: "неизвестное значение", value: "wat", wantErr: ErrInvalidGender},
		{name: "другой регистр", value: "MALE", wantErr: ErrInvalidGender},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseGender(tt.value)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ParseGender(%q) error = %v, want %v", tt.value, err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("ParseGender(%q) unexpected error = %v", tt.value, err)
			}
			if got != tt.want {
				t.Errorf("ParseGender(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestParseRole(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    Role
		wantErr error
	}{
		{name: "client", value: "client", want: RoleClient},
		{name: "operator", value: "operator", want: RoleOperator},
		{name: "admin", value: "admin", want: RoleAdmin},
		{name: "значение с пробелами", value: " admin ", want: RoleAdmin},
		{name: "пустая строка", value: "", want: RoleClient},
		{name: "неизвестная роль", value: "root", wantErr: ErrInvalidRole},
		{name: "другой регистр", value: "CLIENT", wantErr: ErrInvalidRole},
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

func TestGenderValid(t *testing.T) {
	tests := []struct {
		name  string
		value Gender
		want  bool
	}{
		{name: "unspecified", value: GenderUnspecified, want: true},
		{name: "male", value: GenderMale, want: true},
		{name: "female", value: GenderFemale, want: true},
		{name: "пустое значение", value: "", want: false},
		{name: "неизвестное значение", value: Gender("wat"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.value.Valid(); got != tt.want {
				t.Errorf("Gender(%q).Valid() = %v, want %v", tt.value, got, tt.want)
			}
			if got := tt.value.String(); got != string(tt.value) {
				t.Errorf("Gender(%q).String() = %q, want %q", tt.value, got, tt.value)
			}
		})
	}
}

func TestRoleValid(t *testing.T) {
	tests := []struct {
		name  string
		value Role
		want  bool
	}{
		{name: "client", value: RoleClient, want: true},
		{name: "operator", value: RoleOperator, want: true},
		{name: "admin", value: RoleAdmin, want: true},
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
