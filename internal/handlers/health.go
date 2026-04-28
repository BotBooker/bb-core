package handlers

import (
	"net/http"

	"bookerbotapi/pkg/response"
)

func HealthCheck(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, map[string]string{"status": "healthy"})
}
