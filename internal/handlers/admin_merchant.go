// internal/handlers/admin_merchant.go
package handlers

import (
	"encoding/json"
	"net/http"

	"bookerbotapi/internal/models"
	"bookerbotapi/pkg/response"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (h *AdminHandler) CreateMerchant(w http.ResponseWriter, r *http.Request) {
	var req models.Merchant
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", "")
		return
	}

	// Validate required fields
	if req.Name == "" {
		response.Error(w, http.StatusBadRequest, "MISSING_NAME", "name is required", "")
		return
	}

	// Generate ID
	req.ID = uuid.New().String()

	if err := h.repo.CreateMerchant(r.Context(), &req); err != nil {
		response.InternalError(w, "CREATE_MERCHANT_ERROR", "Failed to create merchant", err)
		return
	}

	response.JSON(w, http.StatusCreated, req)
}

func (h *AdminHandler) GetMerchant(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if id == "" {
		response.Error(w, http.StatusBadRequest, "MISSING_ID", "Merchant ID is required", "")
		return
	}

	merchant, err := h.repo.GetMerchantByID(r.Context(), id)
	if err != nil {
		response.InternalError(w, "MERCHANT_ERROR", "Failed to get merchant", err)
		return
	}
	if merchant == nil {
		response.Error(w, http.StatusNotFound, "MERCHANT_NOT_FOUND", "Merchant not found", "")
		return
	}

	response.JSON(w, http.StatusOK, merchant)
}

func (h *AdminHandler) DeleteMerchant(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if id == "" {
		response.Error(w, http.StatusBadRequest, "MISSING_ID", "Merchant ID is required", "")
		return
	}

	err := h.repo.DeleteMerchant(r.Context(), id)
	if err != nil {
		response.InternalError(w, "DELETE_MERCHANT_ERROR", "Failed to delete merchant", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminHandler) ListMerchantsFiltered(w http.ResponseWriter, r *http.Request) {
	nameContains := r.URL.Query().Get("name_contains")

	merchants, err := h.repo.ListMerchantsFiltered(r.Context(), nameContains)
	if err != nil {
		response.InternalError(w, "LIST_MERCHANTS_ERROR", "Failed to list merchants", err)
		return
	}

	mrResponse := make([]interface{}, len(merchants))
	for i, m := range merchants {
		mrResponse[i] = map[string]interface{}{
			"id":              m.ID,
			"name":            m.Name,
			"welcome_message": m.WelcomeMessage,
		}
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"total":     len(mrResponse),
		"merchants": mrResponse,
		"filters": map[string]string{
			"name_contains": nameContains,
		},
	})
}
