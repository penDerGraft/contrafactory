// Package auth provides authentication middleware and API key management.
package auth

import (
	"context"
	"net/http"

	"github.com/pendergraft/contrafactory/internal/storage"
)

// Context key type for avoiding collisions
type contextKey string

const apiKeyContextKey contextKey = "apiKey"

// GetAPIKeyFromContext retrieves the API key info from context.
func GetAPIKeyFromContext(ctx context.Context) *storage.APIKey {
	if key, ok := ctx.Value(apiKeyContextKey).(*storage.APIKey); ok {
		return key
	}
	return nil
}

// GetOwnerIDFromContext retrieves the owner ID from context.
func GetOwnerIDFromContext(ctx context.Context) string {
	if key := GetAPIKeyFromContext(ctx); key != nil {
		return key.ID
	}
	return ""
}

// Middleware returns an HTTP middleware that validates API keys.
func Middleware(store storage.APIKeyStore, writeError func(w http.ResponseWriter, status int, code, message string)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "" {
				// Check Authorization header
				auth := r.Header.Get("Authorization")
				if len(auth) > 7 && auth[:7] == "Bearer " {
					apiKey = auth[7:]
				}
			}

			if apiKey == "" {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "API key required")
				return
			}

			key, err := store.ValidateAPIKey(r.Context(), apiKey)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid API key")
				return
			}

			// Store API key info in context
			ctx := context.WithValue(r.Context(), apiKeyContextKey, key)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OptionalMiddleware returns an HTTP middleware that validates API keys if present,
// but allows requests without keys to proceed.
func OptionalMiddleware(store storage.APIKeyStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "" {
				// Check Authorization header
				auth := r.Header.Get("Authorization")
				if len(auth) > 7 && auth[:7] == "Bearer " {
					apiKey = auth[7:]
				}
			}

			if apiKey != "" {
				key, err := store.ValidateAPIKey(r.Context(), apiKey)
				if err == nil && key != nil {
					ctx := context.WithValue(r.Context(), apiKeyContextKey, key)
					r = r.WithContext(ctx)
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}
