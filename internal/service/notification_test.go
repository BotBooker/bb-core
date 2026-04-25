package service

import (
	"testing"
)

// mockNotificationRepository is a mock implementation of NotificationRepository for testing.
type mockNotificationRepository struct{}

// mockNotificationUserService is a mock implementation of NotificationUserService for testing.
type mockNotificationUserService struct{}

// mockNotificationEventSubscriber is a mock implementation of NotificationEventSubscriber for testing.
type mockNotificationEventSubscriber struct{}

func (m *mockNotificationEventSubscriber) Subscribe(event string, handler func()) {}

func TestNewNotificationService(t *testing.T) {
	tests := []struct {
		name        string
		notifRepo   NotificationRepository
		userService NotificationUserService
		events      NotificationEventSubscriber
	}{
		{
			name:        "all valid dependencies",
			notifRepo:   &mockNotificationRepository{},
			userService: &mockNotificationUserService{},
			events:      &mockNotificationEventSubscriber{},
		},
		{
			name:        "nil notification repo",
			notifRepo:   nil,
			userService: &mockNotificationUserService{},
			events:      &mockNotificationEventSubscriber{},
		},
		{
			name:        "nil user service",
			notifRepo:   &mockNotificationRepository{},
			userService: nil,
			events:      &mockNotificationEventSubscriber{},
		},
		{
			name:        "nil events",
			notifRepo:   &mockNotificationRepository{},
			userService: &mockNotificationUserService{},
			events:      nil,
		},
		{
			name:        "all nil",
			notifRepo:   nil,
			userService: nil,
			events:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewNotificationService(tt.notifRepo, tt.userService, tt.events)
			if got == nil {
				t.Error("NewNotificationService() returned nil")
			}
		})
	}
}
