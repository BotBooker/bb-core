// internal/handlers/admin_services.go
package handlers

import (
	"encoding/json"
	"net/http"

	"bookerbotapi/internal/availability"
	"bookerbotapi/internal/models"
	"bookerbotapi/internal/repository"
	"bookerbotapi/pkg/response"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// AdminHandler handles admin CRUD endpoints.
// It depends on the composite repository (needs all domains).
type AdminHandler struct {
	repo repository.Repository
}

func NewAdminHandler(repo repository.Repository) *AdminHandler {
	return &AdminHandler{
		repo: repo,
	}
}

func (h *AdminHandler) CreateService(w http.ResponseWriter, r *http.Request) {
	var req models.Service
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.InternalError(w, "INVALID_REQUEST", "Invalid request body", err)
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

	if err := h.repo.CreateService(r.Context(), &req); err != nil {
		response.InternalError(w, "CREATE_SERVICE_ERROR", "Failed to create service", err)
		return
	}

	response.JSON(w, http.StatusCreated, req)
}

func (h *AdminHandler) DeleteService(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if id == "" {
		response.Error(w, http.StatusBadRequest, "MISSING_ID", "Service ID is required", "")
		return
	}

	if err := h.repo.DeleteService(r.Context(), id); err != nil {
		response.InternalError(w, "DELETE_SERVICE_ERROR", "Failed to delete service", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminHandler) ListServicesFiltered(w http.ResponseWriter, r *http.Request) {
	merchantID := r.URL.Query().Get("merchant_id")
	nameContains := r.URL.Query().Get("name_contains")

	services, err := h.repo.ListServicesFiltered(r.Context(), merchantID, nameContains)
	if err != nil {
		response.InternalError(w, "LIST_SERVICES_ERROR", "Failed to list services", err)
		return
	}

	svcsResponse := make([]interface{}, len(services))
	for i, svc := range services {
		svcsResponse[i] = map[string]interface{}{
			"id":              svc.ID,
			"name":            svc.Name,
			"merchant_id":     svc.MerchantID,
			"duration_minutes": svc.DurationMinutes,
			"time_granularity": svc.TimeGranularity,
		}
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"total":    len(svcsResponse),
		"services": svcsResponse,
		"filters": map[string]string{
			"merchant_id":   merchantID,
			"name_contains": nameContains,
		},
	})
}
