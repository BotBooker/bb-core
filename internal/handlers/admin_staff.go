package handlers

import (
	"net/http"

	"bookerbotapi/pkg/response"
)

func CreateStaff(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement logic
	response.JSON(w, http.StatusCreated, map[string]interface{}{
		"id":          "stf_stub",
		"merchant_id": "mch_stub",
		"name":        "Stub Staff",
		"service_ids": []interface{}{},
	})
}

func DeleteStaff(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement logic
	w.WriteHeader(http.StatusNoContent)
}

func ListStaffFiltered(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement logic
	response.JSON(w, http.StatusOK, map[string]interface{}{
		"total": 0,
		"staff": []interface{}{},
	})
}
