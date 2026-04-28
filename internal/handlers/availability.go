// internal/handlers/availability.go
package handlers

import (
	"net/http"

	"bookerbotapi/pkg/response"
)

func GetAvailableDates(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement logic
	response.JSON(w, http.StatusOK, []interface{}{})
}

func GetAvailableSlots(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement logic
	response.JSON(w, http.StatusOK, []interface{}{})
}
