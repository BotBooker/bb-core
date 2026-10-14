package servers

import (
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/botbooker/bb-core/internal/model"
	"github.com/botbooker/bb-core/internal/service"
	auth_v1 "github.com/botbooker/bb-core/pkg/proto/auth/v1"
)

// SessionToProto конвертирует выданную сессию в proto-сообщение Session.
//
// Время отдаётся в UTC — так же, как google.protobuf.Timestamp в ответах
// grpc-gateway: клиенту не важен пояс сервера, важен момент времени.
func SessionToProto(session *service.Session) *auth_v1.Session {
	if session == nil || session.Token == nil {
		return nil
	}

	return &auth_v1.Session{
		Token:     session.AccessToken,
		ExpiresAt: timestamppb.New(session.Token.ExpiresAt.UTC()),
	}
}

// RegisterInputFromProto конвертирует proto-запрос регистрации в доменный вход.
// Значения полей здесь не проверяются: формат проверяют proto-валидация
// (Validate) и model.RegisterInput.Validate.
func RegisterInputFromProto(req *auth_v1.AuthServiceRegisterRequest) (model.RegisterInput, error) {
	if req == nil {
		return model.RegisterInput{}, errNilRequest
	}

	// genderFromProto объявлена в user_mapping.go: пол приходит тем же
	// user.v1.Gender, что и в proto-сервисе пользователей.
	gender, err := genderFromProto(req.GetGender())
	if err != nil {
		return model.RegisterInput{}, err
	}

	return model.RegisterInput{
		CreateUserInput: model.CreateUserInput{
			Email:       req.GetEmail(),
			FirstName:   req.GetFirstName(),
			LastName:    req.GetLastName(),
			MiddleName:  req.GetMiddleName(),
			PhoneNumber: req.GetPhoneNumber(),
			Gender:      gender,
		},
		Password: req.GetPassword(),
	}, nil
}
