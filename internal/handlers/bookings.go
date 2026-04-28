// internal/handlers/bookings.go
package handlers

import (
	"net/http"

	"encoding/json"
	"time"

	"bookerbotapi/internal/availability"
	"bookerbotapi/internal/models"
	"bookerbotapi/pkg/response"
)

func CreateBooking(w http.ResponseWriter, r *http.Request) {
	var req models.Booking
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", err.Error())
		return
	}

	// Validate request
	if req.ServiceID == "" || req.StaffID == "" || req.UserID == "" {
		response.Error(w, http.StatusBadRequest, "MISSING_FIELDS", "service_id, staff_id, and user_id are required", "")
		return
	}

	// Parse start time
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_DATE", "Invalid start_time format", err.Error())
		return
	}

	// Get service details (from database)
	service := &models.Service{
		ID:              req.ServiceID,
		TimeGranularity: 15,                    // This should come from database
		WorkingHours:    models.WorkingHours{}, // Load from database
	}

	// Validate granularity
	if err := availability.ValidateGranularity(service.TimeGranularity); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_CONFIGURATION", err.Error(), "")
		return
	}

	// Initialize availability manager (should be injected via dependency injection)
	availabilityManager := getAvailabilityManager() // Implement this

	// Check availability
	available, err := availabilityManager.CheckAvailability(r.Context(), service, startTime, req.DurationMinutes)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "AVAILABILITY_CHECK_FAILED", "Failed to check availability", err.Error())
		return
	}

	if !available {
		response.Error(w, http.StatusConflict, "SLOT_UNAVAILABLE", "The requested time slot is already booked", "")
		return
	}

	// Reserve the booking in Redis
	err = availabilityManager.ReserveBooking(r.Context(), service, startTime, req.DurationMinutes)
	if err != nil {
		response.Error(w, http.StatusConflict, "RESERVATION_FAILED", "Failed to reserve time slot", err.Error())
		return
	}

	// Create booking in database
	// ... save to database

	response.JSON(w, http.StatusCreated, map[string]interface{}{
		"id":               "bok_stub",
		"user_id":          req.UserID,
		"service_id":       req.ServiceID,
		"staff_id":         req.StaffID,
		"start_time":       req.StartTime,
		"duration_minutes": req.DurationMinutes,
		"status":           "confirmed",
	})
}

func ListBookings(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement logic
	response.JSON(w, http.StatusOK, map[string]interface{}{
		"total":    0,
		"bookings": []interface{}{},
	})
}

func GetBooking(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement logic
	response.JSON(w, http.StatusOK, map[string]interface{}{
		"id":               "bok_stub",
		"user_id":          "stub",
		"service_id":       "stub",
		"staff_id":         "stub",
		"start_time":       "2024-01-01T00:00:00Z",
		"duration_minutes": 30,
		"status":           "pending",
	})
}

func CancelBooking(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement logic
	response.JSON(w, http.StatusOK, map[string]interface{}{
		"id":               "bok_stub",
		"user_id":          "stub",
		"service_id":       "stub",
		"staff_id":         "stub",
		"start_time":       "2024-01-01T00:00:00Z",
		"duration_minutes": 30,
		"status":           "cancelled",
	})
}
