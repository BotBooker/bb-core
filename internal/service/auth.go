package service

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
)

// AuthUserRepository — интерфейс репозитория пользователей для AuthService.
type AuthUserRepository any

// методы, которые AuthService использует из репозитория пользователей

// AuthSessionRepository — интерфейс репозитория сессий для AuthService.
type AuthSessionRepository any

// методы, которые AuthService использует из репозитория сессий

// AuthCache — интерфейс кэша для AuthService (хранит токены).
type AuthCache interface {
	Get(key string) (string, error)
	Set(key, value string) error
}

// AuthEventPublisher — интерфейс для публикации событий авторизации.
type AuthEventPublisher interface {
	Publish(event string)
}

// AuthService — интерфейс сервиса авторизации.
type AuthService interface {
	ValidateToken(token string) bool
}

// authService — конкретная реализация, скрыта от внешних пакетов.
type authService struct {
	userRepo    AuthUserRepository
	sessionRepo AuthSessionRepository
	cache       AuthCache
	events      AuthEventPublisher
}

// NewAuthService создаёт сервис авторизации.
func NewAuthService(
	userRepo AuthUserRepository,
	sessionRepo AuthSessionRepository,
	cache AuthCache,
	events AuthEventPublisher,
) AuthService {
	slog.Debug("сервис авторизации создан")
	return &authService{
		userRepo:    userRepo,
		sessionRepo: sessionRepo,
		cache:       cache,
		events:      events,
	}
}

// ValidateToken проверяет токен авторизации.
func (s *authService) ValidateToken(token string) bool {
	tokenHash := hashToken(token)
	slog.Debug("validating token", "token_hash", tokenHash)

	_, err := s.cache.Get(token)

	return err == nil
}

func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
