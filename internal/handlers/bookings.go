// internal/handlers/bookings.go
package handlers

import (
	"net/http"

	"bookerbotapi/pkg/response"
)

func CreateBooking(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement logic
	response.JSON(w, http.StatusCreated, map[string]interface{}{
		"id":               "bok_stub",
		"user_id":          "stub",
		"service_id":       "stub",
		"staff_id":         "stub",
		"start_time":       "2024-01-01T00:00:00Z",
		"duration_minutes": 30,
		"status":           "pending",
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
