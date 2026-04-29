package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	mock_availability "bookerbotapi/internal/availability/mocks"
	"bookerbotapi/internal/models"
	mock_repo "bookerbotapi/internal/repository/mock"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
)

// TestGetAvailableDatesValidGranularity tests GetAvailableDates with bitmap
func TestGetAvailableDates(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc := &models.Service{ID: "svc", DurationMinutes: 60, TimeGranularity: 30}
	repo := mock_repo.NewMockBookingRepository(ctrl)
	mockMgr := mock_availability.NewMockAManager(ctrl)
	repo.EXPECT().GetServiceByID(gomock.Any(), "svc").Return(svc, nil)
	mockMgr.EXPECT().GetOrCreateBitmap(gomock.Any(), svc, gomock.Any()).Return([]byte{0xFF}, nil).AnyTimes()
	handler := NewAvailabilityHandler(mockMgr, repo)
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/availability?service_id=svc&days_ahead=7", nil)
	w := httptest.NewRecorder()
	handler.GetAvailableDates(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestGetAvailableDatesBadRequest tests missing service_id
func TestGetAvailableDatesBadRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock_repo.NewMockBookingRepository(ctrl)
	mockMgr := mock_availability.NewMockAManager(ctrl)
	handler := NewAvailabilityHandler(mockMgr, repo)
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/availability", nil)
	w := httptest.NewRecorder()
	handler.GetAvailableDates(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestGetAvailableDatesNotFound tests service not found
func TestGetAvailableDatesNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock_repo.NewMockBookingRepository(ctrl)
	mockMgr := mock_availability.NewMockAManager(ctrl)
	repo.EXPECT().GetServiceByID(gomock.Any(), gomock.Any()).Return(nil, nil)
	handler := NewAvailabilityHandler(mockMgr, repo)
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/availability?service_id=invalid", nil)
	w := httptest.NewRecorder()
	handler.GetAvailableDates(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestGetAvailableSlots tests GetAvailableSlots
func TestGetAvailableSlots(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc := &models.Service{ID: "svc", DurationMinutes: 60, TimeGranularity: 30}
	repo := mock_repo.NewMockBookingRepository(ctrl)
	mockMgr := mock_availability.NewMockAManager(ctrl)
	repo.EXPECT().GetServiceByID(gomock.Any(), "svc").Return(svc, nil)
	mockMgr.EXPECT().GetOrCreateBitmap(gomock.Any(), svc, gomock.Any()).Return([]byte{0xFF, 0xFF}, nil)
	handler := NewAvailabilityHandler(mockMgr, repo)
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/availability?date=2024-01-15&service_id=svc", nil)
	w := httptest.NewRecorder()
	handler.GetAvailableSlots(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestGetAvailableSlotsBadRequest tests missing required params
func TestGetAvailableSlotsBadRequest(t *testing.T) {
	tests := []struct {
		name  string
		url   string
		code  int
	}{
		{"no params", "/availability", http.StatusBadRequest},
		{"date only", "/availability?date=2024-01-15", http.StatusBadRequest},
		{"staff only", "/availability?staff_id=s1", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockMgr := mock_availability.NewMockAManager(ctrl)
			handler := NewAvailabilityHandler(mockMgr, nil)
			req, _ := http.NewRequestWithContext(context.Background(), "GET", tt.url, nil)
			w := httptest.NewRecorder()
			handler.GetAvailableSlots(w, req)
			assert.Equal(t, tt.code, w.Code)
		})
	}
}

// TestGetAvailableSlotsEmptyBitmap tests with all unavailable bitmap
func TestGetAvailableSlotsEmptyBitmap(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc := &models.Service{ID: "svc", DurationMinutes: 60, TimeGranularity: 30}
	repo := mock_repo.NewMockBookingRepository(ctrl)
	mockMgr := mock_availability.NewMockAManager(ctrl)
	repo.EXPECT().GetServiceByID(gomock.Any(), "svc").Return(svc, nil)
	mockMgr.EXPECT().GetOrCreateBitmap(gomock.Any(), svc, gomock.Any()).Return([]byte{0x00}, nil)
	handler := NewAvailabilityHandler(mockMgr, repo)
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/availability?date=2024-01-15&service_id=svc", nil)
	w := httptest.NewRecorder()
	handler.GetAvailableSlots(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestGetAvailableSlotsDifferentGranularity tests 10 minute granularity
func TestGetAvailableSlotsDifferentGranularity(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc := &models.Service{ID: "svc", DurationMinutes: 30, TimeGranularity: 10}
	repo := mock_repo.NewMockBookingRepository(ctrl)
	mockMgr := mock_availability.NewMockAManager(ctrl)
	repo.EXPECT().GetServiceByID(gomock.Any(), "svc").Return(svc, nil)
	mockMgr.EXPECT().GetOrCreateBitmap(gomock.Any(), svc, gomock.Any()).Return([]byte{0xFF}, nil)
	handler := NewAvailabilityHandler(mockMgr, repo)
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/availability?date=2024-01-15&service_id=svc", nil)
	w := httptest.NewRecorder()
	handler.GetAvailableSlots(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestGetAvailableSlotsJSONResponse tests JSON response structure
func TestGetAvailableSlotsJSONResponse(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc := &models.Service{ID: "svc", DurationMinutes: 60, TimeGranularity: 30}
	repo := mock_repo.NewMockBookingRepository(ctrl)
	mockMgr := mock_availability.NewMockAManager(ctrl)
	repo.EXPECT().GetServiceByID(gomock.Any(), "svc").Return(svc, nil)
	mockMgr.EXPECT().GetOrCreateBitmap(gomock.Any(), svc, gomock.Any()).Return([]byte{0x00}, nil)
	handler := NewAvailabilityHandler(mockMgr, repo)
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/availability?date=2024-01-15&service_id=svc", nil)
	w := httptest.NewRecorder()
	handler.GetAvailableSlots(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var slots []map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&slots)
	assert.NoError(t, err)
}

// TestCountAvailableSlots tests the countAvailableSlots function
func TestCountAvailableSlots(t *testing.T) {
	c1 := countAvailableSlots([]byte{0xFF}, 1440)
	c2 := countAvailableSlots([]byte{0xFF}, 30)
	assert.Equal(t, 1, c1)
	assert.Equal(t, 8, c2)
}

// TestExtractAvailableSlots tests the extractAvailableSlots function
func TestExtractAvailableSlots(t *testing.T) {
	slots1 := extractAvailableSlots([]byte{0x00}, time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), 30, 30)
	slots2 := extractAvailableSlots([]byte{0x01}, time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), 30, 30)
	assert.Equal(t, 0, len(slots1))
	assert.Equal(t, 1, len(slots2))
}

// TestGetAvailableSlotsInvalidDate tests invalid date format
func TestGetAvailableSlotsInvalidDate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockMgr := mock_availability.NewMockAManager(ctrl)
	handler := NewAvailabilityHandler(mockMgr, nil)
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/availability?date=invalid&service_id=svc", nil)
	w := httptest.NewRecorder()
	handler.GetAvailableSlots(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestGetAvailableSlotsMissingServiceID tests missing service_id
func TestGetAvailableSlotsMissingServiceID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockMgr := mock_availability.NewMockAManager(ctrl)
	handler := NewAvailabilityHandler(mockMgr, nil)
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/availability?date=2024-01-15", nil)
	w := httptest.NewRecorder()
	handler.GetAvailableSlots(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
