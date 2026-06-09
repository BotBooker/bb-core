// internal/handlers/catalog.go
package handlers

import (
	"net/http"

	"bookerbotapi/internal/repository"
	"bookerbotapi/pkg/response"
)

type CatalogHandler struct {
	repo repository.AdminRepository
}

func NewCatalogHandler(repo repository.AdminRepository) *CatalogHandler {
	return &CatalogHandler{
		repo: repo,
	}
}

func (h *CatalogHandler) ListServices(w http.ResponseWriter, r *http.Request) {
	merchantID := r.URL.Query().Get("merchant_id")

	if merchantID == "" {
		response.Error(w, http.StatusBadRequest, "MISSING_MERCHANT", "merchant_id is required", "")
		return
	}

	// In production, implement GetServicesByMerchantID in repository
	// For now, returning empty array
	response.JSON(w, http.StatusOK, []interface{}{})
}

func (h *CatalogHandler) ListStaff(w http.ResponseWriter, r *http.Request) {
	merchantID := r.URL.Query().Get("merchant_id")
	serviceID := r.URL.Query().Get("service_id")
	_ = serviceID // todo

	if merchantID == "" {
		response.Error(w, http.StatusBadRequest, "MISSING_MERCHANT", "merchant_id is required", "")
		return
	}

	// In production, implement GetStaffByMerchantID in repository
	// For now, returning empty array
	response.JSON(w, http.StatusOK, []interface{}{})
}
