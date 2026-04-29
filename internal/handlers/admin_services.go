// internal/handlers/admin_services.go
package handlers

import (
	"bookerbotapi/internal/availability"
	"encoding/json"
	"net/http"

	"bookerbotapi/internal/models"
	"bookerbotapi/internal/repository"
	"bookerbotapi/pkg/response"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type AdminHandler struct {
	repo repository.BookingRepository
}

func NewAdminHandler(repo repository.BookingRepository) *AdminHandler {
	return &AdminHandler{
		repo: repo,
	}
}

func (h *AdminHandler) CreateService(w http.ResponseWriter, r *http.Request) {
	var req models.Service
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", err.Error())
		return
	}

	// Validate required fields
	if req.Name == "" || req.MerchantID == "" {
		response.Error(w, http.StatusBadRequest, "MISSING_FIELDS", "name and merchant_id are required", "")
		return
	}

	// Validate granularity
	if req.TimeGranularity == 0 {
		req.TimeGranularity = 15 // default
	}

	if err := availability.ValidateGranularity(req.TimeGranularity); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_GRANULARITY", err.Error(), "")
		return
	}

	// Generate ID
	req.ID = uuid.New().String()

	// In production, implement CreateService in repository
	response.JSON(w, http.StatusCreated, req)
}

func (h *AdminHandler) DeleteService(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if id == "" {
		response.Error(w, http.StatusBadRequest, "MISSING_ID", "Service ID is required", "")
		return
	}

	// TODO In production, implement DeleteService in repository
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminHandler) ListServicesFiltered(w http.ResponseWriter, r *http.Request) {
	merchantID := r.URL.Query().Get("merchant_id")
	nameContains := r.URL.Query().Get("name_contains")
	status := r.URL.Query().Get("status")

	// In production, implement filtered query in repository
	response.JSON(w, http.StatusOK, map[string]interface{}{
		"total":    0,
		"services": []interface{}{},
		"filters": map[string]string{
			"merchant_id":   merchantID,
			"name_contains": nameContains,
			"status":        status,
		},
	})
}
