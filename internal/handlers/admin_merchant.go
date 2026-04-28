package handlers

import (
	"net/http"

	"bookerbotapi/pkg/response"
)

func CreateMerchant(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement logic
	response.JSON(w, http.StatusCreated, map[string]interface{}{
		"id":              "mch_stub",
		"name":            "Stub Merchant",
		"welcome_message": "Welcome!",
	})
}

func GetMerchant(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement logic
	response.JSON(w, http.StatusOK, map[string]interface{}{
		"id":              "mch_stub",
		"name":            "Stub Merchant",
		"welcome_message": "Welcome!",
	})
}

func DeleteMerchant(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement logic
	w.WriteHeader(http.StatusNoContent)
}

func ListMerchantsFiltered(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement logic
	response.JSON(w, http.StatusOK, map[string]interface{}{
		"total":     0,
		"merchants": []interface{}{},
	})
}
