// internal/handlers/availability.go
package handlers

import (
	"net/http"
	"strconv"
	"time"

	"bookerbotapi/internal/availability"
	"bookerbotapi/internal/repository"
	"bookerbotapi/pkg/response"
)

type AvailabilityHandler struct {
	availabilityManager *availability.AvailabilityManager
	repo                repository.BookingRepository
}

func NewAvailabilityHandler(am *availability.AvailabilityManager, repo repository.BookingRepository) *AvailabilityHandler {
	return &AvailabilityHandler{
		availabilityManager: am,
		repo:                repo,
	}
}

func (h *AvailabilityHandler) GetAvailableDates(w http.ResponseWriter, r *http.Request) {
	staffID := r.URL.Query().Get("staff_id")
	_ = staffID // fixme
	serviceID := r.URL.Query().Get("service_id")
	daysAhead := 7 // default

	if days := r.URL.Query().Get("days_ahead"); days != "" {
		if d, err := strconv.Atoi(days); err == nil && d > 0 && d <= 30 {
			daysAhead = d
		}
	}

	if serviceID == "" {
		response.Error(w, http.StatusBadRequest, "MISSING_SERVICE", "service_id is required", "")
		return
	}

	// Get service details
	service, err := h.repo.GetServiceByID(r.Context(), serviceID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "SERVICE_ERROR", "Failed to get service", err.Error())
		return
	}
	if service == nil {
		response.Error(w, http.StatusNotFound, "SERVICE_NOT_FOUND", "Service not found", "")
		return
	}

	// Calculate available dates
	availableDates := []map[string]interface{}{}
	now := time.Now()

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

	// TODO  Add staff filtering
	_ = staffID

	if dateStr == "" || serviceID == "" {
		response.Error(w, http.StatusBadRequest, "MISSING_PARAMETERS", "date and service_id are required", "")
		return
	}

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_DATE", "Invalid date format", err.Error())
		return
	}

	// Get service details
	service, err := h.repo.GetServiceByID(r.Context(), serviceID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "SERVICE_ERROR", "Failed to get service", err.Error())
		return
	}
	if service == nil {
		response.Error(w, http.StatusNotFound, "SERVICE_NOT_FOUND", "Service not found", "")
		return
	}

	// Get or create bitmap
	bitmap, err := h.availabilityManager.GetOrCreateBitmap(r.Context(), service, date)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "BITMAP_ERROR", "Failed to get availability", err.Error())
		return
	}

	// Extract available slots from bitmap
	slots := extractAvailableSlots(bitmap, date, service.TimeGranularity, service.DurationMinutes)

	// TODO If staff_id is provided, filter slots by staff availability (simplified)

	response.JSON(w, http.StatusOK, slots)
}

// Helper functions
func countAvailableSlots(bitmap []byte, granularity int) int {
	bitmapSize := availability.CalculateBitmapSize(granularity)
	count := 0
	for i := 0; i < bitmapSize; i++ {
		if getBitFromBitmap(bitmap, i) == 1 {
			count++
		}
	}
	return count
}

func extractAvailableSlots(bitmap []byte, date time.Time, granularity, durationMinutes int) []map[string]interface{} {
	slots := []map[string]interface{}{}
	bitmapSize := availability.CalculateBitmapSize(granularity)

	for i := 0; i < bitmapSize; i++ {
		if getBitFromBitmap(bitmap, i) == 1 {
			startTime := availability.BitIndexToTime(date, i, granularity)

			// Check if the entire duration fits
			slotsNeeded := durationMinutes / granularity
			available := true

			for j := 1; j < slotsNeeded; j++ {
				if i+j >= bitmapSize || getBitFromBitmap(bitmap, i+j) == 0 {
					available = false
					break
				}
			}

			if available {
				slots = append(slots, map[string]interface{}{
					"start_time": startTime.Format(time.RFC3339),
					"end_time":   startTime.Add(time.Duration(durationMinutes) * time.Minute).Format(time.RFC3339),
				})
			}
		}
	}

	return slots
}

func getBitFromBitmap(bitmap []byte, position int) int {
	byteIndex := position / 8
	bitIndex := uint(position % 8)

	if byteIndex >= len(bitmap) {
		return 0
	}

	return int((bitmap[byteIndex] >> bitIndex) & 1)
}
