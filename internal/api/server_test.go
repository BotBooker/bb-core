package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockUserService is a mock implementation of UserService for testing.
type mockUserService struct {
	getProfileFunc func(token string) string
}

func (m *mockUserService) GetProfile(token string) string {
	if m.getProfileFunc != nil {
		return m.getProfileFunc(token)
	}
	return ""
}

// mockAuthService is a mock implementation of AuthService for testing.
type mockAuthService struct{}

func (m *mockAuthService) ValidateToken(token string) bool {
	return token == "valid-token"
}

// mockNotificationService is a mock implementation of NotificationService for testing.
type mockNotificationService struct{}

func TestNewHandler(t *testing.T) {
	tests := []struct {
		name         string
		userService  UserService
		authService  AuthService
		notifService NotificationService
		wantNotNil   bool
	}{
		{
			name:         "all valid services",
			userService:  &mockUserService{},
			authService:  &mockAuthService{},
			notifService: &mockNotificationService{},
			wantNotNil:   true,
		},
		{
			name:         "nil user service",
			userService:  nil,
			authService:  &mockAuthService{},
			notifService: &mockNotificationService{},
			wantNotNil:   true,
		},
		{
			name:         "nil auth service",
			userService:  &mockUserService{},
			authService:  nil,
			notifService: &mockNotificationService{},
			wantNotNil:   true,
		},
		{
			name:         "nil notification service",
			userService:  &mockUserService{},
			authService:  &mockAuthService{},
			notifService: nil,
			wantNotNil:   true,
		},
		{
			name:         "all nil services",
			userService:  nil,
			authService:  nil,
			notifService: nil,
			wantNotNil:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewHandler(tt.userService, tt.authService, tt.notifService)
			if tt.wantNotNil && got == nil {
				t.Error("NewHandler() returned nil, expected non-nil Handler")
			}
		})
	}
}

func TestHandler_Routes(t *testing.T) {
	h := NewHandler(&mockUserService{}, &mockAuthService{}, &mockNotificationService{})
	routes := h.Routes()
	if routes == nil {
		t.Fatal("Routes() returned nil http.Handler")
	}
}

func TestHandler_healthHandler(t *testing.T) {
	h := NewHandler(&mockUserService{}, &mockAuthService{}, &mockNotificationService{})
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	h.Routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("healthHandler returned wrong status code: got %v want %v", w.Code, http.StatusOK)
	}

	expectedBody := "ok\n"
	if w.Body.String() != expectedBody {
		t.Errorf("healthHandler returned unexpected body: got %q want %q", w.Body.String(), expectedBody)
	}
}

func TestHandler_getUserProfile(t *testing.T) {
	tests := []struct {
		name        string
		authHeader  string
		mockProfile string
		wantStatus  int
		wantProfile string
	}{
		{
			name:        "valid token",
			authHeader:  "valid-token",
			mockProfile: "user profile data",
			wantStatus:  http.StatusOK,
			wantProfile: "user profile data",
		},
		{
			name:        "invalid token",
			authHeader:  "invalid-token",
			mockProfile: "unauthorized",
			wantStatus:  http.StatusOK,
			wantProfile: "unauthorized",
		},
		{
			name:        "empty token",
			authHeader:  "",
			mockProfile: "unauthorized",
			wantStatus:  http.StatusOK,
			wantProfile: "unauthorized",
		},
		{
			name:        "no authorization header",
			authHeader:  "",
			mockProfile: "unauthorized",
			wantStatus:  http.StatusOK,
			wantProfile: "unauthorized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockUserSvc := &mockUserService{
				getProfileFunc: func(token string) string {
					return tt.mockProfile
				},
			}
			h := NewHandler(mockUserSvc, &mockAuthService{}, &mockNotificationService{})

			req := httptest.NewRequest("GET", "/users/me", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			w := httptest.NewRecorder()

			h.Routes().ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("getUserProfile returned wrong status code: got %v want %v", w.Code, tt.wantStatus)
			}

			contentType := w.Header().Get("Content-Type")
			expectedContentType := "application/json; charset=utf-8"
			if contentType != expectedContentType {
				t.Errorf("getUserProfile returned wrong content type: got %q want %q", contentType, expectedContentType)
			}
		})
	}
}
