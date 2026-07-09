// internal/handlers/catalog.go
package handlers

import (
	"net/http"

	"bookerbotapi/internal/repository"
	"bookerbotapi/pkg/response"
)

// CatalogHandler handles public catalog endpoints.
// It depends only on service and staff repository operations.
type CatalogHandler struct {
	svcRepo repository.ServiceRepository
	stfRepo repository.StaffRepository
}

// NewCatalogHandler creates a new CatalogHandler.
func NewCatalogHandler(svcRepo repository.ServiceRepository, stfRepo repository.StaffRepository) *CatalogHandler {
	return &CatalogHandler{
		svcRepo: svcRepo,
		stfRepo: stfRepo,
	}
}

// ListServices returns services filtered by merchant_id from the repository.
func (h *CatalogHandler) ListServices(w http.ResponseWriter, r *http.Request) {
	merchantID := r.URL.Query().Get("merchant_id")

	if merchantID == "" {
		response.Error(w, http.StatusBadRequest, "MISSING_MERCHANT", "merchant_id is required", "")
		return
	}

	services, err := h.svcRepo.ListServicesFiltered(r.Context(), merchantID, "")
	if err != nil {
		response.InternalError(w, "LIST_SERVICES_ERROR", "Failed to list services", err)
		return
	}

	svcsResponse := make([]map[string]interface{}, len(services))
	for i, svc := range services {
		svcsResponse[i] = map[string]interface{}{
			"id":               svc.ID,
			"name":             svc.Name,
			"merchant_id":      svc.MerchantID,
			"duration_minutes": svc.DurationMinutes,
			"time_granularity": svc.TimeGranularity,
		}
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"total":    len(svcsResponse),
		"services": svcsResponse,
		"filters": map[string]string{
			"merchant_id":   merchantID,
			"name_contains": "",
		},
	})
}

// ListStaff returns staff filtered by merchant_id and optionally service_id from the repository.
func (h *CatalogHandler) ListStaff(w http.ResponseWriter, r *http.Request) {
	merchantID := r.URL.Query().Get("merchant_id")
	serviceID := r.URL.Query().Get("service_id")

	if merchantID == "" {
		response.Error(w, http.StatusBadRequest, "MISSING_MERCHANT", "merchant_id is required", "")
		return
	}

	staffMembers, err := h.stfRepo.ListStaffFiltered(r.Context(), merchantID, "")
	if err != nil {
		response.InternalError(w, "LIST_STAFF_ERROR", "Failed to list staff", err)
		return
	}

	// Filter by service_id if provided (staff.Services contains the service IDs they can provide)
	sResponse := make([]map[string]interface{}, 0, len(staffMembers))
	for _, st := range staffMembers {
		// If serviceID filter is set, check staff can provide this service
		if serviceID != "" {
			found := false
			for _, svcID := range st.Services {
				if svcID == serviceID {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		sResponse = append(sResponse, map[string]interface{}{
			"id":          st.ID,
			"name":        st.Name,
			"merchant_id": st.MerchantID,
			"services":    st.Services,
		})
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"total": len(sResponse),
		"staff": sResponse,
		"filters": map[string]string{
			"merchant_id":   merchantID,
			"name_contains": "",
			"service_id":    serviceID,
		},
	})
}
