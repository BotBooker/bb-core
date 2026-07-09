// internal/handlers/availability_test.go
package handlers

import (
	"context"
	"net/http"
	"testing"

	mock_availability "bookerbotapi/internal/availability/mocks"
	"bookerbotapi/internal/models"
	mock_repo "bookerbotapi/internal/repository/mock"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
)

// TestGetAvailableDatestest ValidGranularity tests GetAvailableDates with bitmap
func TestGetAvailableDatestestValidGranularity(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc := &models.Service{ID: "svc", DurationMinutes: 60, TimeGranularity: 30}
	repo := mock_repo.NewMockRepository(ctrl)
	mockMgr := mock_availability.NewMockAManager(ctrl)
	repo.EXPECT().GetServiceByID(gomock.Any(), "svc").Return(svc, nil)
	mockMgr.EXPECT().GetOrCreateBitmap(gomock.Any(), svc, gomock.Any()).Return([]byte{0xFF}, nil).AnyTimes()
	handler := NewAvailabilityHandler(mockMgr, repo)
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/availability?service_id=svc&days_ahead=7", nil)
	w := httptest.NewRecorder()
	handler.GetAvailableDates(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestGetAvailableDatestestBadRequest tests missing service_id
func TestGetAvailableDatestestBadRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock_repo.NewMockRepository(ctrl)
	mockMgr := mock_availability.NewMockAManager(ctrl)
	handler := NewAvailabilityHandler(mockMgr, repo)
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/availability", nil)
	w := httptest.NewRecorder()
	handler.GetAvailableDates(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestGetAvailableDatestestNotFound tests service not found
func TestGetAvailableDatestestNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	repo := mock_repo.NewMockRepository(ctrl)
	mockMgr := mock_availability.NewMockAManager(ctrl)
	repo.EXPECT().GetServiceByID(gomock.Any(), gomock.Any()).Return(nil, nil)
	handler := NewAvailabilityHandler(mockMgr, repo)
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/availability?service_id=invalid", nil)
	w := httptest.NewRecorder()
	handler.GetAvailableDates(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestGetAvailableSlotstest tests GetAvailableSlots without staff filter
func TestGetAvailableSlotstest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	svc := &models.Service{ID: "svc", DurationMinutes: 60, TimeGranularity: 30}
	repo := mock_repo.NewMockRepository(ctrl)
	mockMgr := mock_availability.NewMockAManager(ctrl)
	repo.EXPECT().GetServiceByID(gomock.Any(), "svc").Return(svc, nil)
	mockMgr.EXPECT().GetOrCreateBitmap(gomock.Any(), svc, gomock.Any()).Return([]byte{0xFF, 0xFF}, nil)
	handler := NewAvailabilityHandler(mockMgr, repo)
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/availability?date=2024-01-15&service_id=svc", nil)
	w := httptest.NewRecorder()
	handler.GetAvailableSlots(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestGetAvailableSlotstestBadRequest tests missing required params
func TestGetAvailableSlotstestBadRequest(t *testing.T) {
	tests := []struct {
		name       string
		serviceID  string
		date       string
		wantCode   int
	}{
		{
			name:      "missing service_id and date",
			serviceID: "",
			date:      "",
			wantCode:  http.StatusBadRequest,
		},
		{
			name:      "missing date",
			serviceID: "svc",
			date:      "",
			wantCode:  http.StatusBadRequest,
		},
		{
			name:      "missing service_id",
			serviceID: "",
			date:      "2024-01-15",
			wantCode:  http.StatusBadRequest,
		},
		{
			name:      "invalid date format",
			serviceID: "svc",
			date:      "not-a-date",
			wantCode:  http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			repo := mock_repo.NewMockRepository(ctrl)
			mockMgr := mock_availability.NewMockAManager(ctrl)
			handler := NewAvailabilityHandler(mockMgr, repo)
			url := "/availability/slots?"
			if tt.serviceID != "" {
				url += "service_id=" + tt.serviceID + "&"
			}
			if tt.date != "" {
				url += "date=" + tt.date
			}
			req, _ := http.NewRequestWithContext(context.Background(), "GET", url, nil)
			w := httptest.NewRecorder()
			handler.GetAvailableSlots(w, req)
			assert.Equal(t, tt.wantCode, w.Code)
		})
	}
}
