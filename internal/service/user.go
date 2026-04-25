package service

import "log/slog"

// UserRepository — интерфейс репозитория пользователей для UserService.
type UserRepository any

// методы, которые UserService использует из репозитория пользователей

// UserAuthService — интерфейс сервиса авторизации для UserService.
type UserAuthService interface {
	ValidateToken(token string) bool
}

// UserEventPublisher — интерфейс для публикации событий пользователей.
type UserEventPublisher interface {
	Publish(event string)
}

// UserService — интерфейс сервиса пользователей.
type UserService interface {
	GetProfile(token string) string
}

// userService — конкретная реализация, скрыта от внешних пакетов.
type userService struct {
	userRepo    UserRepository
	authService UserAuthService
	events      UserEventPublisher
}

// NewUserService создаёт сервис пользователей.
func NewUserService(userRepo UserRepository, authService UserAuthService, events UserEventPublisher) UserService {
	slog.Debug("сервис пользователей создан")
	return &userService{
		userRepo:    userRepo,
		authService: authService,
		events:      events,
	}
}

// GetProfile возвращает профиль текущего пользователя.
func (s *userService) GetProfile(token string) string {
	if !s.authService.ValidateToken(token) {
		return "unauthorized"
	}

	return "user profile data"
}
