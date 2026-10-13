package servers

import (
	"errors"
	"testing"
	"time"

	date "google.golang.org/genproto/googleapis/type/date"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/botbooker/bb-core/internal/model"
	user_v1 "github.com/botbooker/bb-core/pkg/proto/user/v1"
)

// testTime — фиксированный момент времени, чтобы тесты конвертации были
// детерминированными.
var testTime = time.Date(2026, time.October, 12, 10, 30, 0, 0, time.UTC)

// testUser возвращает валидного пользователя доменной модели.
func testUser(t *testing.T) *model.User {
	t.Helper()

	birth := model.Date{Year: 1990, Month: time.May, Day: 1}

	user, err := model.NewUser(model.CreateUserInput{
		Email:       "ivan@example.com",
		FirstName:   "Иван",
		LastName:    "Иванов",
		MiddleName:  "Иванович",
		PhoneNumber: "+79991234567",
		Gender:      model.GenderMale,
		BirthDate:   &birth,
	}, testTime)
	if err != nil {
		t.Fatalf("model.NewUser() unexpected error = %v", err)
	}

	user.SetLastLogin(testTime)

	return user
}

func TestUserToProto(t *testing.T) {
	t.Run("nil модель", func(t *testing.T) {
		if got := UserToProto(nil); got != nil {
			t.Errorf("UserToProto(nil) = %v, want nil", got)
		}
	})

	t.Run("конвертирует все поля", func(t *testing.T) {
		user := testUser(t)

		got := UserToProto(user)

		if got.GetUuid() != user.ID {
			t.Errorf("Uuid = %q, want %q", got.GetUuid(), user.ID)
		}
		if got.GetEmail() != user.Email {
			t.Errorf("Email = %q, want %q", got.GetEmail(), user.Email)
		}
		if got.GetFirstName() != user.FirstName || got.GetLastName() != user.LastName || got.GetMiddleName() != user.MiddleName {
			t.Errorf("ФИО = %q %q %q, want %q %q %q",
				got.GetFirstName(), got.GetLastName(), got.GetMiddleName(),
				user.FirstName, user.LastName, user.MiddleName)
		}
		if got.GetPhoneNumber() != user.PhoneNumber {
			t.Errorf("PhoneNumber = %q, want %q", got.GetPhoneNumber(), user.PhoneNumber)
		}
		if got.GetGender() != user_v1.Gender_GENDER_MALE {
			t.Errorf("Gender = %v, want %v", got.GetGender(), user_v1.Gender_GENDER_MALE)
		}
		birth := got.GetBirthday()
		if birth.GetYear() != int32(user.BirthDate.Year) ||
			birth.GetMonth() != int32(user.BirthDate.Month) ||
			birth.GetDay() != int32(user.BirthDate.Day) {
			t.Errorf("BirthDate = %v, want %v", birth, user.BirthDate)
		}
		if !got.GetCreatedAt().AsTime().Equal(user.CreatedAt) {
			t.Errorf("CreatedAt = %v, want %v", got.GetCreatedAt().AsTime(), user.CreatedAt)
		}
		if !got.GetUpdatedAt().AsTime().Equal(user.UpdatedAt) {
			t.Errorf("UpdatedAt = %v, want %v", got.GetUpdatedAt().AsTime(), user.UpdatedAt)
		}
	})

	t.Run("пол", func(t *testing.T) {
		tests := []struct {
			name  string
			model model.Gender
			want  user_v1.Gender
		}{
			{name: "male", model: model.GenderMale, want: user_v1.Gender_GENDER_MALE},
			{name: "female", model: model.GenderFemale, want: user_v1.Gender_GENDER_FEMALE},
			{name: "unspecified", model: model.GenderUnspecified, want: user_v1.Gender_GENDER_UNSPECIFIED},
			{name: "пустое значение", model: "", want: user_v1.Gender_GENDER_UNSPECIFIED},
			{name: "неизвестное значение", model: model.Gender("wat"), want: user_v1.Gender_GENDER_UNSPECIFIED},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				user := testUser(t)
				user.Gender = tt.model

				if got := UserToProto(user).GetGender(); got != tt.want {
					t.Errorf("Gender = %v, want %v", got, tt.want)
				}
			})
		}
	})

	t.Run("пустая дата рождения остаётся nil", func(t *testing.T) {
		user := testUser(t)
		user.BirthDate = nil

		if got := UserToProto(user).GetBirthday(); got != nil {
			t.Errorf("BirthDate = %v, want nil", got)
		}
	})

	t.Run("время приходит в UTC", func(t *testing.T) {
		zone := time.FixedZone("MSK", 3*60*60)
		user := testUser(t)
		user.CreatedAt = testTime.In(zone)
		user.UpdatedAt = testTime.In(zone)

		got := UserToProto(user)

		if !got.GetCreatedAt().AsTime().Equal(testTime) {
			t.Errorf("CreatedAt = %v, want %v", got.GetCreatedAt().AsTime(), testTime)
		}
		if got.GetCreatedAt().AsTime().Location() != time.UTC {
			t.Errorf("CreatedAt location = %v, want UTC", got.GetCreatedAt().AsTime().Location())
		}
	})
}

func TestCreateInputFromProto(t *testing.T) {
	// В proto дата рождения — google.type.Date: год, месяц и день без времени.
	birthDate := &date.Date{Year: 1990, Month: 5, Day: 1}
	wantBirth := model.Date{Year: 1990, Month: time.May, Day: 1}

	t.Run("nil запрос", func(t *testing.T) {
		if _, err := CreateInputFromProto(nil); !errors.Is(err, errNilRequest) {
			t.Fatalf("CreateInputFromProto(nil) error = %v, want %v", err, errNilRequest)
		}
	})

	t.Run("конвертирует все поля", func(t *testing.T) {
		req := &user_v1.UserServiceCreateRequest{
			Email:       "ivan@example.com",
			FirstName:   "Иван",
			LastName:    "Иванов",
			MiddleName:  "Иванович",
			PhoneNumber: "+79991234567",
			Gender:      user_v1.Gender_GENDER_FEMALE,
			Birthday:    birthDate,
		}

		in, err := CreateInputFromProto(req)
		if err != nil {
			t.Fatalf("CreateInputFromProto() unexpected error = %v", err)
		}

		if in.Email != req.GetEmail() || in.FirstName != req.GetFirstName() ||
			in.LastName != req.GetLastName() || in.MiddleName != req.GetMiddleName() {
			t.Errorf("поля сконвертированы неверно: %+v", in)
		}
		if in.PhoneNumber != req.GetPhoneNumber() {
			t.Errorf("PhoneNumber = %q, want %q", in.PhoneNumber, req.GetPhoneNumber())
		}
		if in.Gender != model.GenderFemale {
			t.Errorf("Gender = %q, want %q", in.Gender, model.GenderFemale)
		}
		if in.BirthDate == nil {
			t.Fatal("BirthDate = nil, want дату рождения")
		}
		if *in.BirthDate != wantBirth {
			t.Errorf("BirthDate = %v, want %v", in.BirthDate, wantBirth)
		}
		if in.BirthDate.String() != "1990-05-01" {
			t.Errorf("BirthDate = %q, want %q", in.BirthDate, "1990-05-01")
		}
	})

	t.Run("несуществующую дату отвергает валидация модели", func(t *testing.T) {
		// google.type.Date хранит только год, месяц и день: конвертер переносит
		// значения как есть, а 31 февраля отсекает валидация модели.
		req := &user_v1.UserServiceCreateRequest{
			Email:     "ivan@example.com",
			FirstName: "Иван",
			LastName:  "Иванов",
			Birthday:  &date.Date{Year: 1990, Month: 2, Day: 31},
		}

		in, err := CreateInputFromProto(req)
		if err != nil {
			t.Fatalf("CreateInputFromProto() unexpected error = %v", err)
		}

		if _, err := model.NewUser(in, testTime); !errors.Is(err, model.ErrInvalidBirthDate) {
			t.Errorf("model.NewUser() error = %v, want %v", err, model.ErrInvalidBirthDate)
		}
	})

	t.Run("дата без месяца и дня отвергается", func(t *testing.T) {
		// В google.type.Date поля — обычные int32, поэтому возможен частично
		// заполненный объект: такой дате тоже нужна понятная ошибка.
		req := &user_v1.UserServiceCreateRequest{
			Email:     "ivan@example.com",
			FirstName: "Иван",
			LastName:  "Иванов",
			Birthday:  &date.Date{Year: 1990},
		}

		in, err := CreateInputFromProto(req)
		if err != nil {
			t.Fatalf("CreateInputFromProto() unexpected error = %v", err)
		}

		if in.BirthDate == nil || in.BirthDate.Year != 1990 || in.BirthDate.Month != 0 || in.BirthDate.Day != 0 {
			t.Fatalf("BirthDate = %v, want значения из proto без изменений", in.BirthDate)
		}

		if _, err := model.NewUser(in, testTime); !errors.Is(err, model.ErrInvalidBirthDate) {
			t.Errorf("model.NewUser() error = %v, want %v", err, model.ErrInvalidBirthDate)
		}
	})

	t.Run("пустой объект значит «дата не задана»", func(t *testing.T) {
		req := &user_v1.UserServiceCreateRequest{
			Email:     "ivan@example.com",
			FirstName: "Иван",
			LastName:  "Иванов",
			Birthday:  &date.Date{},
		}

		in, err := CreateInputFromProto(req)
		if err != nil {
			t.Fatalf("CreateInputFromProto() unexpected error = %v", err)
		}

		if in.BirthDate != nil {
			t.Errorf("BirthDate = %v, want nil", in.BirthDate)
		}
	})

	t.Run("частичная дата без года принимается", func(t *testing.T) {
		// google.type.Date допускает дату без года: в модели остаются месяц и день,
		// а год — 0, как и запросил клиент, скрывший год рождения.
		req := &user_v1.UserServiceCreateRequest{
			Email:     "ivan@example.com",
			FirstName: "Иван",
			LastName:  "Иванов",
			Birthday:  &date.Date{Month: 5, Day: 17},
		}

		in, err := CreateInputFromProto(req)
		if err != nil {
			t.Fatalf("CreateInputFromProto() unexpected error = %v", err)
		}

		if in.BirthDate == nil || in.BirthDate.Year != 0 || in.BirthDate.Month != time.May || in.BirthDate.Day != 17 {
			t.Fatalf("BirthDate = %v, want 0000-05-17", in.BirthDate)
		}

		if _, err := model.NewUser(in, testTime); err != nil {
			t.Errorf("model.NewUser() unexpected error = %v", err)
		}
	})

	t.Run("пол и дата рождения не заданы", func(t *testing.T) {
		req := &user_v1.UserServiceCreateRequest{
			Email:     "ivan@example.com",
			FirstName: "Иван",
			LastName:  "Иванов",
		}

		in, err := CreateInputFromProto(req)
		if err != nil {
			t.Fatalf("CreateInputFromProto() unexpected error = %v", err)
		}

		if in.Gender != model.GenderUnspecified {
			t.Errorf("Gender = %q, want %q", in.Gender, model.GenderUnspecified)
		}
		if in.BirthDate != nil {
			t.Errorf("BirthDate = %v, want nil", in.BirthDate)
		}
	})

	t.Run("неизвестный пол", func(t *testing.T) {
		req := &user_v1.UserServiceCreateRequest{Gender: user_v1.Gender(42)}

		if _, err := CreateInputFromProto(req); !errors.Is(err, model.ErrInvalidGender) {
			t.Fatalf("CreateInputFromProto() error = %v, want %v", err, model.ErrInvalidGender)
		}
	})

	t.Run("запрос превращается в валидного пользователя", func(t *testing.T) {
		req := &user_v1.UserServiceCreateRequest{
			Email:       "ivan@example.com",
			FirstName:   "Иван",
			LastName:    "Иванов",
			MiddleName:  "Иванович",
			PhoneNumber: "+79991234567",
			Gender:      user_v1.Gender_GENDER_MALE,
			Birthday:    birthDate,
		}

		in, err := CreateInputFromProto(req)
		if err != nil {
			t.Fatalf("CreateInputFromProto() unexpected error = %v", err)
		}

		user, err := model.NewUser(in, testTime)
		if err != nil {
			t.Fatalf("model.NewUser() unexpected error = %v", err)
		}

		got := UserToProto(user)

		if got.GetEmail() != req.GetEmail() || got.GetFirstName() != req.GetFirstName() {
			t.Errorf("после конвертации Email/FirstName = %q/%q, want %q/%q",
				got.GetEmail(), got.GetFirstName(), req.GetEmail(), req.GetFirstName())
		}
		if got.GetGender() != req.GetGender() {
			t.Errorf("после конвертации Gender = %v, want %v", got.GetGender(), req.GetGender())
		}
		birth := got.GetBirthday()
		if birth.GetYear() != int32(wantBirth.Year) ||
			birth.GetMonth() != int32(wantBirth.Month) ||
			birth.GetDay() != int32(wantBirth.Day) {
			t.Errorf("после конвертации BirthDate = %v, want %v", birth, wantBirth)
		}
		if got.GetUuid() == "" {
			t.Error("после конвертации Uuid пуст, want UUID созданного пользователя")
		}
	})
}

func TestUpdateInputFromProto(t *testing.T) {
	// В proto дата рождения — google.type.Date: год, месяц и день без времени.
	birthDate := &date.Date{Year: 1990, Month: 5, Day: 1}
	wantBirth := model.Date{Year: 1990, Month: time.May, Day: 1}

	t.Run("nil запрос", func(t *testing.T) {
		if _, err := UpdateInputFromProto(nil); !errors.Is(err, errNilRequest) {
			t.Fatalf("UpdateInputFromProto(nil) error = %v, want %v", err, errNilRequest)
		}
	})

	t.Run("пустой запрос ничего не меняет", func(t *testing.T) {
		in, err := UpdateInputFromProto(&user_v1.UserServiceUpdateRequest{})
		if err != nil {
			t.Fatalf("UpdateInputFromProto() unexpected error = %v", err)
		}

		if in.FirstName != nil || in.LastName != nil || in.MiddleName != nil ||
			in.PhoneNumber != nil || in.Gender != nil || in.BirthDate != nil {
			t.Errorf("UpdateInputFromProto() = %+v, want все поля nil", in)
		}
	})

	t.Run("пустой объект даты ничего не меняет", func(t *testing.T) {
		req := &user_v1.UserServiceUpdateRequest{Birthday: &date.Date{}}

		in, err := UpdateInputFromProto(req)
		if err != nil {
			t.Fatalf("UpdateInputFromProto() unexpected error = %v", err)
		}

		if in.BirthDate != nil {
			t.Errorf("BirthDate = %v, want nil", in.BirthDate)
		}
	})

	t.Run("конвертирует все поля", func(t *testing.T) {
		req := &user_v1.UserServiceUpdateRequest{
			FirstName:   wrapperspb.String("Пётр"),
			LastName:    wrapperspb.String("Петров"),
			MiddleName:  wrapperspb.String(""),
			PhoneNumber: wrapperspb.String("+79990000000"),
			Gender:      wrapperspb.Int32(int32(user_v1.Gender_GENDER_FEMALE)),
			Birthday:    birthDate,
		}

		in, err := UpdateInputFromProto(req)
		if err != nil {
			t.Fatalf("UpdateInputFromProto() unexpected error = %v", err)
		}

		if in.FirstName == nil || *in.FirstName != "Пётр" {
			t.Errorf("FirstName = %v, want %q", in.FirstName, "Пётр")
		}
		if in.LastName == nil || *in.LastName != "Петров" {
			t.Errorf("LastName = %v, want %q", in.LastName, "Петров")
		}
		if in.MiddleName == nil || *in.MiddleName != "" {
			t.Errorf("MiddleName = %v, want указатель на пустую строку", in.MiddleName)
		}
		if in.PhoneNumber == nil || *in.PhoneNumber != "+79990000000" {
			t.Errorf("PhoneNumber = %v, want %q", in.PhoneNumber, "+79990000000")
		}
		if in.Gender == nil || *in.Gender != model.GenderFemale {
			t.Errorf("Gender = %v, want %q", in.Gender, model.GenderFemale)
		}
		if in.BirthDate == nil || *in.BirthDate != wantBirth {
			t.Errorf("BirthDate = %v, want %v", in.BirthDate, wantBirth)
		}
	})

	t.Run("частичный запрос меняет только переданные поля", func(t *testing.T) {
		user := testUser(t)

		req := &user_v1.UserServiceUpdateRequest{
			FirstName: wrapperspb.String("Пётр"),
			Gender:    wrapperspb.Int32(int32(user_v1.Gender_GENDER_FEMALE)),
		}

		in, err := UpdateInputFromProto(req)
		if err != nil {
			t.Fatalf("UpdateInputFromProto() unexpected error = %v", err)
		}

		if err := user.ApplyUpdate(in, testTime.Add(time.Hour)); err != nil {
			t.Fatalf("ApplyUpdate() unexpected error = %v", err)
		}

		if user.FirstName != "Пётр" || user.Gender != model.GenderFemale {
			t.Errorf("FirstName/Gender = %q/%q, want Пётр/%q", user.FirstName, user.Gender, model.GenderFemale)
		}
		if user.LastName != "Иванов" || user.PhoneNumber != "+79991234567" {
			t.Errorf("LastName/PhoneNumber = %q/%q, want Иванов/+79991234567", user.LastName, user.PhoneNumber)
		}
	})

	t.Run("неизвестный пол", func(t *testing.T) {
		req := &user_v1.UserServiceUpdateRequest{Gender: wrapperspb.Int32(42)}

		if _, err := UpdateInputFromProto(req); !errors.Is(err, model.ErrInvalidGender) {
			t.Fatalf("UpdateInputFromProto() error = %v, want %v", err, model.ErrInvalidGender)
		}
	})
}
