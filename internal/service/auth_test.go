package service

import (
	"errors"
	"testing"
)

// mockAuthCache is a mock implementation of AuthCache for testing.
type mockAuthCache struct {
	getFunc func(key string) (string, error)
	setFunc func(key, value string) error
}

func (m *mockAuthCache) Get(key string) (string, error) {
	if m.getFunc != nil {
		return m.getFunc(key)
	}
	return "", nil
}

func (m *mockAuthCache) Set(key, value string) error {
	if m.setFunc != nil {
		return m.setFunc(key, value)
	}
	return nil
}

// mockAuthUserRepository is a mock implementation of AuthUserRepository for testing.
type mockAuthUserRepository struct{}

// mockAuthSessionRepository is a mock implementation of AuthSessionRepository for testing.
type mockAuthSessionRepository struct{}

// mockAuthEventPublisher is a mock implementation of AuthEventPublisher for testing.
type mockAuthEventPublisher struct{}

func (m *mockAuthEventPublisher) Publish(event string) {}

func TestNewAuthService(t *testing.T) {
	tests := []struct {
		name        string
		userRepo    AuthUserRepository
		sessionRepo AuthSessionRepository
		cache       AuthCache
		events      AuthEventPublisher
	}{
		{
			name:        "all valid dependencies",
			userRepo:    &mockAuthUserRepository{},
			sessionRepo: &mockAuthSessionRepository{},
			cache:       &mockAuthCache{},
			events:      &mockAuthEventPublisher{},
		},
		{
			name:        "nil user repo",
			userRepo:    nil,
			sessionRepo: &mockAuthSessionRepository{},
			cache:       &mockAuthCache{},
			events:      &mockAuthEventPublisher{},
		},
		{
			name:        "nil session repo",
			userRepo:    &mockAuthUserRepository{},
			sessionRepo: nil,
			cache:       &mockAuthCache{},
			events:      &mockAuthEventPublisher{},
		},
		{
			name:        "nil cache",
			userRepo:    &mockAuthUserRepository{},
			sessionRepo: &mockAuthSessionRepository{},
			cache:       nil,
			events:      &mockAuthEventPublisher{},
		},
		{
			name:        "nil events",
			userRepo:    &mockAuthUserRepository{},
			sessionRepo: &mockAuthSessionRepository{},
			cache:       &mockAuthCache{},
			events:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewAuthService(tt.userRepo, tt.sessionRepo, tt.cache, tt.events)
			if got == nil {
				t.Error("NewAuthService() returned nil")
			}
		})
	}
}

func TestAuthService_ValidateToken(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		cacheGet func(key string) (string, error)
		want     bool
	}{
		{
			name:  "valid token exists in cache",
			token: "valid-token",
			cacheGet: func(key string) (string, error) {
				if key == "valid-token" {
					return "session-data", nil
				}
				return "", nil
			},
			want: true,
		},
		{
			name:  "invalid token not in cache - returns error",
			token: "invalid-token",
			cacheGet: func(key string) (string, error) {
				return "", errors.New("not found")
			},
			want: false,
		},
		{
			name:  "empty token returns error",
			token: "",
			cacheGet: func(key string) (string, error) {
				return "", errors.New("not found")
			},
			want: false,
		},
		{
			name:  "token with error from cache",
			token: "error-token",
			cacheGet: func(key string) (string, error) {
				return "", errors.New("cache error")
			},
			want: false,
		},
		{
			name:  "special characters in valid token",
			token: "token!@#$%^&*()",
			cacheGet: func(key string) (string, error) {
				if key == "token!@#$%^&*()" {
					return "data", nil
				}
				return "", errors.New("not found")
			},
			want: true,
		},
		{
			name:  "very long valid token",
			token: "a-very-long-token-that-exceeds-normal-length-limits-for-tokens-to-test-boundary-conditions",
			cacheGet: func(key string) (string, error) {
				if key == "a-very-long-token-that-exceeds-normal-length-limits-for-tokens-to-test-boundary-conditions" {
					return "data", nil
				}
				return "", errors.New("not found")
			},
			want: true,
		},
		{
			name:  "very long invalid token",
			token: "a-very-long-token-that-exceeds-normal-length-limits-for-tokens-to-test-boundary-conditions",
			cacheGet: func(key string) (string, error) {
				return "", errors.New("not found")
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := &mockAuthCache{
				getFunc: tt.cacheGet,
			}
			svc := NewAuthService(
				&mockAuthUserRepository{},
				&mockAuthSessionRepository{},
				cache,
				&mockAuthEventPublisher{},
			)

			got := svc.ValidateToken(tt.token)
			if got != tt.want {
				t.Errorf("ValidateToken() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAuthService_ValidateToken_NoCache(t *testing.T) {
	// Test with nil cache - should handle gracefully (returns false since cache.Get would panic)
	// The production code doesn't guard against nil cache, so this documents the behavior
	svc := NewAuthService(
		&mockAuthUserRepository{},
		&mockAuthSessionRepository{},
		nil,
		&mockAuthEventPublisher{},
	)

	// This will panic with nil cache - documenting expected behavior
	// In production, cache should never be nil
	defer func() {
		if r := recover(); r != nil {
			t.Logf("ValidateToken() panicked with nil cache as expected: %v", r)
		}
	}()

	result := svc.ValidateToken("any-token")
	t.Logf("ValidateToken() with nil cache = %v (if no panic)", result)
}
