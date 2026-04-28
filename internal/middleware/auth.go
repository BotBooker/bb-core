package middleware

import (
	"net/http"

	"bookerbotapi/pkg/response"
)

func APIKeyAuth(validKeys []string) func(http.Handler) http.Handler {
	validKeysMap := make(map[string]bool)
	for _, key := range validKeys {
		validKeysMap[key] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.Header.Get("X-API-Key")

			if apiKey == "" {
				response.Error(w, http.StatusUnauthorized, "MISSING_API_KEY", "X-API-Key header is required", "")
				return
			}

			if !validKeysMap[apiKey] {
				response.Error(w, http.StatusUnauthorized, "INVALID_API_KEY", "Invalid API key", "")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
