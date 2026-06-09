// internal/handlers/bookings.go
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"bookerbotapi/internal/availability"
	"bookerbotapi/internal/models"
	"bookerbotapi/internal/repository"
	"bookerbotapi/pkg/response"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type BookingHandler struct {
	repo                repository.AdminRepository
	availabilityManager *availability.AvailabilityManager
}

func NewBookingHandler(repo repository.AdminRepository, am *availability.AvailabilityManager) *BookingHandler {
	return &BookingHandler{
		repo:                repo,
		availabilityManager: am,
	}
}

func (h *BookingHandler) CreateBooking(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID          string  `json:"user_id"`
		ServiceID       string  `json:"service_id"`
		StaffID         string  `json:"staff_id"`
		StartTime       string  `json:"start_time"`
		DurationMinutes int     `json:"duration_minutes"`
		Price           float64 `json:"price,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", err.Error())
		return
	}

	// Validate required fields
	if req.UserID == "" || req.ServiceID == "" || req.StaffID == "" {
		response.Error(w, http.StatusBadRequest, "MISSING_FIELDS", "user_id, service_id, and staff_id are required", "")
		return
	}

	// Parse start time
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_DATE", "Invalid start_time format", err.Error())
		return
	}

	// Get service details
	service, err := h.repo.GetServiceByID(r.Context(), req.ServiceID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "SERVICE_ERROR", "Failed to get service", err.Error())
		return
	}
	if service == nil {
		response.Error(w, http.StatusNotFound, "SERVICE_NOT_FOUND", "Service not found", "")
		return
	}

	// Use service duration if not provided in request
	durationMinutes := req.DurationMinutes
	if durationMinutes == 0 {
		durationMinutes = service.DurationMinutes
	}

	// Validate granularity
	if err := availability.ValidateGranularity(service.TimeGranularity); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_CONFIGURATION", err.Error(), "")
		return
	}

	// Check availability
	available, err := h.availabilityManager.CheckAvailability(r.Context(), service, startTime, durationMinutes)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "AVAILABILITY_CHECK_FAILED", "Failed to check availability", err.Error())
		return
	}

	if !available {
		response.Error(w, http.StatusConflict, "SLOT_UNAVAILABLE", "The requested time slot is already booked", "")
		return
	}

	// Reserve the booking in Redis
	err = h.availabilityManager.ReserveBooking(r.Context(), service, startTime, durationMinutes)
	if err != nil {
		response.Error(w, http.StatusConflict, "RESERVATION_FAILED", "Failed to reserve time slot", err.Error())
		return
	}

	// Create booking in database
	now := time.Now()
	booking := &models.Booking{
		ID:              uuid.New().String(),
		UserID:          req.UserID,
		ServiceID:       req.ServiceID,
		StaffID:         req.StaffID,
		StartTime:       req.StartTime,
		DurationMinutes: durationMinutes,
		Price:           req.Price,
		Paid:            false,
		Status:          "confirmed",
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := h.repo.CreateBooking(r.Context(), booking); err != nil {
		// Note: In production, implement Redis rollback mechanism
		response.Error(w, http.StatusInternalServerError, "BOOKING_CREATION_FAILED", "Failed to create booking", err.Error())
		return
	}

	response.JSON(w, http.StatusCreated, booking)
}

func (h *BookingHandler) ListBookings(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	status := r.URL.Query().Get("status")
	limit := 20
	offset := 0

	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := parseInt(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := parseInt(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	if userID == "" {
		response.Error(w, http.StatusBadRequest, "MISSING_USER", "user_id is required", "")
		return
	}

	bookings, total, err := h.repo.GetBookingsByUserID(r.Context(), userID, status, limit, offset)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "FETCH_FAILED", "Failed to fetch bookings", err.Error())
		return
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"total":    total,
		"limit":    limit,
		"offset":   offset,
		"bookings": bookings,
	})
}

func (h *BookingHandler) GetBooking(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	booking, err := h.repo.GetBookingByID(r.Context(), id)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "FETCH_FAILED", "Failed to fetch booking", err.Error())
		return
	}

	if booking == nil {
		response.Error(w, http.StatusNotFound, "NOT_FOUND", "Booking not found", "")
		return
	}

	response.JSON(w, http.StatusOK, booking)
}

func (h *BookingHandler) CancelBooking(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Check if booking exists
	booking, err := h.repo.GetBookingByID(r.Context(), id)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "FETCH_FAILED", "Failed to fetch booking", err.Error())
		return
	}

	if booking == nil {
		response.Error(w, http.StatusNotFound, "NOT_FOUND", "Booking not found", "")
		return
	}

	// Update status
	err = h.repo.UpdateBookingStatus(r.Context(), id, "cancelled")
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "CANCEL_FAILED", "Failed to cancel booking", err.Error())
		return
	}

	// Note: In production, update Redis bitmap to free up the cancelled slots
	// Get service and free up the slots in Redis

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"id":     id,
		"status": "cancelled",
	})
}

func parseInt(s string) (int, error) {
	var result int
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}
