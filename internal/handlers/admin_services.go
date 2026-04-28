package handlers

import (
	"net/http"

	"bookerbotapi/pkg/response"
)

func CreateService(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement logic
	response.JSON(w, http.StatusCreated, map[string]interface{}{
		"id":               "svc_stub",
		"merchant_id":      "mch_stub",
		"name":             "Stub Service",
		"duration_minutes": 30,
	})
}

func DeleteService(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement logic
	w.WriteHeader(http.StatusNoContent)
}

func ListServicesFiltered(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement logic
	response.JSON(w, http.StatusOK, map[string]interface{}{
		"total":    0,
		"services": []interface{}{},
	})
}
