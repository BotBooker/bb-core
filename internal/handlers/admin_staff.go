// internal/handlers/admin_staff.go
package handlers

import (
	"encoding/json"
	"net/http"

	"bookerbotapi/internal/models"
	"bookerbotapi/pkg/response"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (h *AdminHandler) CreateStaff(w http.ResponseWriter, r *http.Request) {
	var req models.Staff
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", err.Error())
		return
	}

	// Validate required fields
	if req.Name == "" || req.MerchantID == "" {
		response.Error(w, http.StatusBadRequest, "MISSING_FIELDS", "name and merchant_id are required", "")
		return
	}

	// Generate ID
	req.ID = uuid.New().String()

	// In production, implement CreateStaff in repository
	response.JSON(w, http.StatusCreated, req)
}

func (h *AdminHandler) DeleteStaff(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if id == "" {
		response.Error(w, http.StatusBadRequest, "MISSING_ID", "Staff ID is required", "")
		return
	}

	// In production, implement DeleteStaff in repository
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminHandler) ListStaffFiltered(w http.ResponseWriter, r *http.Request) {
	merchantID := r.URL.Query().Get("merchant_id")
	serviceID := r.URL.Query().Get("service_id")
	status := r.URL.Query().Get("status")

	// In production, implement filtered query in repository
	response.JSON(w, http.StatusOK, map[string]interface{}{
		"total": 0,
		"staff": []interface{}{},
		"filters": map[string]string{
			"merchant_id": merchantID,
			"service_id":  serviceID,
			"status":      status,
		},
	})
}
