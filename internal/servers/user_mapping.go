package servers

import (
	"errors"
	"fmt"
	"time"

	date "google.golang.org/genproto/googleapis/type/date"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/botbooker/bb-core/internal/model"
	user_v1 "github.com/botbooker/bb-core/pkg/proto/user/v1"
)

// errNilRequest — адаптер не должен передавать nil-запросы: это ошибка
// транспорта, а не валидация пользовательских данных.
var errNilRequest = errors.New("request is nil")

// UserToProto конвертирует доменную модель пользователя в proto-сообщение User.
// Служебные поля модели (LastLoginAt, IsActive, IsDelete) в proto v1 не
// отдаются; роли у профиля нет — она принадлежит членству в тенанте.
// Глобальные флаги (IsSuperuser, IsGlobalOperator) — наоборот, отдаются: это
// системный уровень доступа, и клиенту нужно видеть, кто управляет тенантами.
func UserToProto(user *model.User) *user_v1.User {
	if user == nil {
		return nil
	}

	return &user_v1.User{
		Uuid:        user.ID,
		Email:       user.Email,
		FirstName:   user.FirstName,
		LastName:    user.LastName,
		MiddleName:  user.MiddleName,
		PhoneNumber: user.PhoneNumber,
		Gender:      genderToProto(user.Gender),
		Birthday:    dateToProto(user.BirthDate),
		CreatedAt:   timestamppb.New(user.CreatedAt.UTC()),
		UpdatedAt:   timestamppb.New(user.UpdatedAt.UTC()),
		// Глобальные флаги — часть профиля: по ним клиент и staff видят, кто
		// управляет всеми тенантами. Менять их через Update нельзя: для выдачи
		// прав глобального оператора есть отдельные RPC.
		IsSuperuser:      user.IsSuperuser,
		IsGlobalOperator: user.IsGlobalOperator,
	}
}

// CreateInputFromProto конвертирует proto-запрос на создание пользователя
// в доменный вход. Значения полей здесь не проверяются: формат проверяют
// proto-валидация (Validate) и model.CreateUserInput.Validate.
func CreateInputFromProto(req *user_v1.UserServiceCreateRequest) (model.CreateUserInput, error) {
	if req == nil {
		return model.CreateUserInput{}, errNilRequest
	}

	gender, err := genderFromProto(req.GetGender())
	if err != nil {
		return model.CreateUserInput{}, err
	}

	return model.CreateUserInput{
		Email:       req.GetEmail(),
		FirstName:   req.GetFirstName(),
		LastName:    req.GetLastName(),
		MiddleName:  req.GetMiddleName(),
		PhoneNumber: req.GetPhoneNumber(),
		Gender:      gender,
		BirthDate:   dateFromProto(req.GetBirthday()),
	}, nil
}

// UpdateInputFromProto конвертирует proto-запрос на частичное обновление.
// Незаполненные wrappers означают «не менять», поэтому остаются nil —
// так «поле не передано» отличается от «передано пустое значение».
func UpdateInputFromProto(req *user_v1.UserServiceUpdateRequest) (model.UpdateUserInput, error) {
	if req == nil {
		return model.UpdateUserInput{}, errNilRequest
	}

	var in model.UpdateUserInput

	if req.FirstName != nil {
		in.FirstName = strPtr(req.FirstName.GetValue())
	}

	if req.LastName != nil {
		in.LastName = strPtr(req.LastName.GetValue())
	}

	if req.MiddleName != nil {
		in.MiddleName = strPtr(req.MiddleName.GetValue())
	}

	if req.PhoneNumber != nil {
		in.PhoneNumber = strPtr(req.PhoneNumber.GetValue())
	}

	if req.Gender != nil {
		gender, err := genderFromProto(user_v1.Gender(req.Gender.GetValue()))
		if err != nil {
			return model.UpdateUserInput{}, err
		}

		in.Gender = &gender
	}

	if req.Birthday != nil {
		in.BirthDate = dateFromProto(req.Birthday)
	}

	return in, nil
}

// genderToProto конвертирует пол модели в proto-enum: неизвестные и пустые
// значения становятся GENDER_UNSPECIFIED, конвертация не падает.
func genderToProto(gender model.Gender) user_v1.Gender {
	switch gender {
	case model.GenderMale:
		return user_v1.Gender_GENDER_MALE
	case model.GenderFemale:
		return user_v1.Gender_GENDER_FEMALE
	default:
		return user_v1.Gender_GENDER_UNSPECIFIED
	}
}

// genderFromProto конвертирует proto-enum пола в значение модели.
// Неизвестное значение — ошибка валидации: правил для enum в proto нет,
// поэтому проверка остаётся на модели.
func genderFromProto(gender user_v1.Gender) (model.Gender, error) {
	switch gender {
	case user_v1.Gender_GENDER_UNSPECIFIED:
		return model.GenderUnspecified, nil
	case user_v1.Gender_GENDER_MALE:
		return model.GenderMale, nil
	case user_v1.Gender_GENDER_FEMALE:
		return model.GenderFemale, nil
	default:
		return "", fmt.Errorf("%w: unknown proto gender %d", model.ErrInvalidGender, int32(gender))
	}
}

// dateToProto конвертирует необязательную календарную дату в google.type.Date.
//
// В proto дата — это год, месяц и день без времени и часового пояса, как и
// model.Date: пояс не может «сдвинуть» день рождения на сутки.
func dateToProto(value *model.Date) *date.Date {
	if value == nil {
		return nil
	}

	return &date.Date{
		Year:  int32(value.Year),
		Month: int32(value.Month),
		Day:   int32(value.Day),
	}
}

// dateFromProto конвертирует google.type.Date в календарную дату модели.
//
// Пустой объект (все компоненты нулевые) означает «дата не задана» и даёт nil —
// так же, как нулевые поля самого google.type.Date. Частичная дата переносится
// как есть: у google.type.Date год 0 — законное значение для «месяц и день без
// года». Существование даты проверяет модель (model.Date.Valid), поэтому
// «31 февраля» получает понятную ошибку ErrInvalidBirthDate на создании и
// обновлении, а не превращается молча в другую дату.
func dateFromProto(value *date.Date) *model.Date {
	if value == nil {
		return nil
	}

	if value.GetYear() == 0 && value.GetMonth() == 0 && value.GetDay() == 0 {
		return nil
	}

	birthDate := model.Date{
		Year:  int(value.GetYear()),
		Month: time.Month(value.GetMonth()),
		Day:   int(value.GetDay()),
	}

	return &birthDate
}

// strPtr возвращает указатель на строку: значение wrapper-поля proto.
func strPtr(value string) *string { return &value }
