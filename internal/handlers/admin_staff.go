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
		response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", "")
		return
	}

	// Validate required fields
	if req.Name == "" || req.MerchantID == "" {
		response.Error(w, http.StatusBadRequest, "MISSING_FIELDS", "name and merchant_id are required", "")
		return
	}

	req.ID = uuid.New().String()

	if err := h.repo.CreateStaff(r.Context(), &req); err != nil {
		response.InternalError(w, "CREATE_STAFF_ERROR", "Failed to create staff", err)
		return
	}

	response.JSON(w, http.StatusCreated, req)
}

func (h *AdminHandler) DeleteStaff(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if id == "" {
		response.Error(w, http.StatusBadRequest, "MISSING_ID", "Staff ID is required", "")
		return
	}

	err := h.repo.DeleteStaff(r.Context(), id)
	if err != nil {
		response.InternalError(w, "DELETE_STAFF_ERROR", "Failed to delete staff", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminHandler) ListStaffFiltered(w http.ResponseWriter, r *http.Request) {
	merchantID := r.URL.Query().Get("merchant_id")
	nameContains := r.URL.Query().Get("name_contains")

	staffMembers, err := h.repo.ListStaffFiltered(r.Context(), merchantID, nameContains)
	if err != nil {
		response.InternalError(w, "LIST_STAFF_ERROR", "Failed to list staff", err)
		return
	}

	sResponse := make([]interface{}, len(staffMembers))
	for i, s := range staffMembers {
		sResponse[i] = map[string]interface{}{
			"id":          s.ID,
			"name":        s.Name,
			"merchant_id": s.MerchantID,
			"services":    s.Services,
		}
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"total": len(sResponse),
		"staff": sResponse,
		"filters": map[string]string{
			"merchant_id":   merchantID,
			"name_contains": nameContains,
		},
	})
}
