// internal/handlers/availability.go
package handlers

import (
	"net/http"
	"time"

	"bookerbotapi/internal/availability"
	"bookerbotapi/internal/models"
	"bookerbotapi/internal/repository"
	"bookerbotapi/pkg/response"
)

// AvailabilityHandler handles availability endpoints.
// It requires service, staff, and booking repository operations.
type AvailabilityHandler struct {
	availabilityManager availability.AManager
	repo                repository.Repository
}

func NewAvailabilityHandler(am availability.AManager, repo repository.Repository) *AvailabilityHandler {
	return &AvailabilityHandler{
		availabilityManager: am,
		repo:                repo,
	}
}

func (h *AvailabilityHandler) GetAvailableDates(w http.ResponseWriter, r *http.Request) {
	serviceID := r.URL.Query().Get("service_id")

	if serviceID == "" {
		response.Error(w, http.StatusBadRequest, "MISSING_SERVICE", "service_id is required", "")
		return
	}

	// Get service details
	service, err := h.repo.GetServiceByID(r.Context(), serviceID)
	if err != nil {
		response.InternalError(w, "SERVICE_ERROR", "Failed to get service", err)
		return
	}
	if service == nil {
		response.Error(w, http.StatusNotFound, "SERVICE_NOT_FOUND", "Service not found", "")
		return
	}

	// Calculate available dates
	availableDates := []map[string]interface{}{}
	now := time.Now()
	daysAhead := 14 // Check next 2 weeks

	for i := 0; i < daysAhead; i++ {
		date := now.AddDate(0, 0, i)
		dateKey := date.Format("2006-01-02")

		// Get or create bitmap for this date
		bitmap, err := h.availabilityManager.GetOrCreateBitmap(r.Context(), service, date)
		if err != nil {
			continue
		}

		// Count available slots in bitmap
		availableSlots := countAvailableSlots(bitmap, service.TimeGranularity)

		if availableSlots > 0 {
			availableDates = append(availableDates, map[string]interface{}{
				"date":            dateKey,
				"slots_available": availableSlots,
			})
		}
	}

	response.JSON(w, http.StatusOK, availableDates)
}

func (h *AvailabilityHandler) GetAvailableSlots(w http.ResponseWriter, r *http.Request) {
	dateStr := r.URL.Query().Get("date")
	staffID := r.URL.Query().Get("staff_id")
	serviceID := r.URL.Query().Get("service_id")

	if serviceID == "" || dateStr == "" {
		response.Error(w, http.StatusBadRequest, "MISSING_PARAMS", "service_id and date are required", "")
		return
	}

	// Parse date
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_DATE", "Invalid date format. Use YYYY-MM-DD", "")
		return
	}

	// Get service details
	service, err := h.repo.GetServiceByID(r.Context(), serviceID)
	if err != nil {
		response.InternalError(w, "SERVICE_ERROR", "Failed to get service", err)
		return
	}
	if service == nil {
		response.Error(w, http.StatusNotFound, "SERVICE_NOT_FOUND", "Service not found", "")
		return
	}

	// Extract available slots from bitmap
	bitmap, err := h.availabilityManager.GetOrCreateBitmap(r.Context(), service, date)
	if err != nil {
		response.InternalError(w, "BITMAP_ERROR", "Failed to get availability", err)
		return
	}

	slots := extractAvailableSlots(bitmap, date, service.TimeGranularity, service.DurationMinutes)

	// Filter by staff if staff_id is provided
	if staffID != "" {
		staff, err := h.repo.GetStaffByID(r.Context(), staffID)
		if err != nil {
			response.InternalError(w, "STAFF_ERROR", "Failed to get staff", err)
			return
		}
		if staff == nil {
			response.Error(w, http.StatusNotFound, "STAFF_NOT_FOUND", "Staff not found", "")
			return
		}

		// Verify staff is associated with the requested service
		staffCanProvideService := false
		for _, svcID := range staff.Services {
			if svcID == serviceID {
				staffCanProvideService = true
				break
			}
		}
		if !staffCanProvideService {
			response.Error(w, http.StatusUnprocessableEntity, "STAFF_SERVICE_MISMATCH",
				"Staff cannot provide this service", "")
			return
		}

		// Filter slots by staff availability - verify bookings don't conflict
		existingBookings, err := h.repo.GetBookingsByServiceAndDate(r.Context(), serviceID, date)
		if err != nil {
			response.InternalError(w, "BOOKING_ERROR", "Failed to get existing bookings", err)
			return
		}

		// Remove slots that are already booked by this staff member
		slots = removeBookedSlots(slots, existingBookings, staffID)
	}

	response.JSON(w, http.StatusOK, slots)
}

// Helper functions
func countAvailableSlots(bitmap []byte, granularity int) int {
	bitmapSize := availability.CalculateBitmapSize(granularity)
	count := 0
	for i := 0; i < bitmapSize; i++ {
		if availability.GetBit(bitmap, i) == 1 {
			count++
		}
	}
	return count
}

func extractAvailableSlots(bitmap []byte, date time.Time, granularity, durationMinutes int) []map[string]interface{} {
	slots := []map[string]interface{}{}
	bitmapSize := availability.CalculateBitmapSize(granularity)

	for i := 0; i < bitmapSize; i++ {
		if availability.GetBit(bitmap, i) == 1 {
			startTime := availability.BitIndexToTime(date, i, granularity)

			// Check if the entire duration fits
			slotsNeeded := durationMinutes / granularity
			available := true

			for j := 1; j < slotsNeeded; j++ {
				if i+j >= bitmapSize || availability.GetBit(bitmap, i+j) == 0 {
					available = false
					break
				}
			}

			if available {
				slots = append(slots, map[string]interface{}{
					"start_time": startTime.Format(time.RFC3339),
					"end_time":   startTime.Add(time.Duration(durationMinutes)*time.Minute).Format(time.RFC3339),
				})
			}
		}
	}

	return slots
}

// removeBookedSlots removes time slots that are already booked by the specified staff.
func removeBookedSlots(slots []map[string]interface{}, bookings []models.Booking, staffID string) []map[string]interface{} {
	// Build a set of occupied start times for this staff member
	occupied := map[string]bool{}
	for _, b := range bookings {
		if b.StaffID == staffID && b.Status != "cancelled" {
			occupied[b.StartTime] = true
		}
	}

	filtered := make([]map[string]interface{}, 0, len(slots))
	for _, s := range slots {
		startTime, ok := s["start_time"].(string)
		if !ok || occupied[startTime] {
			continue
		}
		filtered = append(filtered, s)
	}
	return filtered
}
