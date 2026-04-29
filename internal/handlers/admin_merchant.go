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
		response.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", err.Error())
		return
	}

	// Validate required fields
	if req.Name == "" {
		response.Error(w, http.StatusBadRequest, "MISSING_NAME", "name is required", "")
		return
	}

	// Generate ID
	req.ID = uuid.New().String()

	// TODO In production, implement CreateMerchant in repository
	response.JSON(w, http.StatusCreated, req)
}

func (h *AdminHandler) GetMerchant(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if id == "" {
		response.Error(w, http.StatusBadRequest, "MISSING_ID", "Merchant ID is required", "")
		return
	}

	// TODO In production, implement GetMerchantByID in repository
	merchant := &models.Merchant{
		ID:             id,
		Name:           "Sample Merchant",
		WelcomeMessage: "Welcome to our service!",
	}

	response.JSON(w, http.StatusOK, merchant)
}

func (h *AdminHandler) DeleteMerchant(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if id == "" {
		response.Error(w, http.StatusBadRequest, "MISSING_ID", "Merchant ID is required", "")
		return
	}

	// TODO In production, implement DeleteMerchant in repository
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminHandler) ListMerchantsFiltered(w http.ResponseWriter, r *http.Request) {
	nameContains := r.URL.Query().Get("name_contains")
	status := r.URL.Query().Get("status")

	// TODO In production, implement filtered query in repository
	response.JSON(w, http.StatusOK, map[string]interface{}{
		"total":     0,
		"merchants": []interface{}{},
		"filters": map[string]string{
			"name_contains": nameContains,
			"status":        status,
		},
	})
}
