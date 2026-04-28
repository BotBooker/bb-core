package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// RequestID adds a request ID to the request context
func RequestID(next http.Handler) http.Handler {
	return middleware.RequestID(next)
}

// Logger logs requests
func Logger(next http.Handler) http.Handler {
	return middleware.Logger(next)
}

// Recoverer recovers from panics
func Recoverer(next http.Handler) http.Handler {
	return middleware.Recoverer(next)
}

// Timeout sets a timeout for requests
func Timeout(timeout time.Duration) func(next http.Handler) http.Handler {
	return middleware.Timeout(timeout)
}

