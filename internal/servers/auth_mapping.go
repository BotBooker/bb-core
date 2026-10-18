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
		Token:      session.AccessToken,
		ExpiresAt:  timestamppb.New(session.Token.ExpiresAt.UTC()),
		TenantUuid: sessionTenantID(session),
		TenantSlug: sessionTenantSlug(session),
		TenantName: sessionTenantName(session),
		Role:       session.Role.String(),
	}
}

// sessionTenantID возвращает UUID тенанта сессии или пустую строку.
func sessionTenantID(session *service.Session) string {
	if session.Tenant == nil {
		return ""
	}

	return session.Tenant.ID
}

// sessionTenantSlug возвращает slug тенанта сессии или пустую строку.
func sessionTenantSlug(session *service.Session) string {
	if session.Tenant == nil {
		return ""
	}

	return session.Tenant.Slug
}

// sessionTenantName возвращает название тенанта сессии или пустую строку.
func sessionTenantName(session *service.Session) string {
	if session.Tenant == nil {
		return ""
	}

	return session.Tenant.Name
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
		Password:   req.GetPassword(),
		TenantSlug: req.GetTenantSlug(),
	}, nil
}
