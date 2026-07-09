package response

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog/log"
)

// ErrorResponse represents an API error response.
type ErrorResponse struct {
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
	Details   string `json:"details,omitempty"`
}

// SuccessResponse represents a successful API response.
type SuccessResponse struct {
	Data interface{} `json:"data,omitempty"`
}

// JSON writes a JSON response with the given status code and data.
func JSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		if err := json.NewEncoder(w).Encode(data); err != nil {
			log.Error().Err(err).Msg("failed to encode JSON response")
		}
	}
}

// Error writes a structured error response.
func Error(w http.ResponseWriter, status int, errorCode, message, details string) {
	JSON(w, status, ErrorResponse{
		ErrorCode: errorCode,
		Message:   message,
		Details:   details,
	})
}

// ErrorFrom logs the error and returns a sanitized error response to the client.
// The full error is logged with zerolog; the client only sees the sanitized message.
func ErrorFrom(w http.ResponseWriter, status int, errorCode, message string, err error) {
	log.Error().Err(err).Str("error_code", errorCode).Msg(message)
	JSON(w, status, ErrorResponse{
		ErrorCode: errorCode,
		Message:   message,
	})
}

// InternalError logs the error and returns a generic 500 response.
// The full error is logged; the client receives a sanitized message.
func InternalError(w http.ResponseWriter, errorCode, message string, err error) {
	ErrorFrom(w, http.StatusInternalServerError, errorCode, message, err)
}

// Success writes a successful JSON response.
func Success(w http.ResponseWriter, status int, data interface{}) {
	JSON(w, status, SuccessResponse{Data: data})
}
