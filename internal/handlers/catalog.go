package handlers

import (
	"net/http"

	"bookerbotapi/pkg/response"
)

func ListServices(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement logic
	response.JSON(w, http.StatusOK, []interface{}{})
}

func ListStaff(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement logic
	response.JSON(w, http.StatusOK, []interface{}{})
}
