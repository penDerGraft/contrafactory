package security

import (
	"net/http"
)

// MaxBodySizeMiddleware returns middleware that limits the request body size.
// The limit is specified in megabytes.
func MaxBodySizeMiddleware(maxSizeMB int) func(http.Handler) http.Handler {
	maxBytes := int64(maxSizeMB) * 1024 * 1024

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Wrap the body with a size limiter
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
