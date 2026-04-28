package response

import (
	"encoding/json"
	"net/http"
)

type ErrorResponse struct {
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
	Details   string `json:"details,omitempty"`
}

type SuccessResponse struct {
	Data interface{} `json:"data,omitempty"`
}

func JSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

func Error(w http.ResponseWriter, status int, errorCode, message, details string) {
	JSON(w, status, ErrorResponse{
		ErrorCode: errorCode,
		Message:   message,
		Details:   details,
	})
}

func Success(w http.ResponseWriter, status int, data interface{}) {
	JSON(w, status, SuccessResponse{Data: data})
}
