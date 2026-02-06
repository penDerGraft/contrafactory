// Package logging provides structured HTTP request logging middleware.
package logging

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/pendergraft/contrafactory/internal/middleware/realip"
)

// responseWriter wraps http.ResponseWriter to capture status and bytes
type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	bytes       int
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.wroteHeader {
		rw.status = code
		rw.wroteHeader = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytes += n
	return n, err
}

// Unwrap returns the underlying ResponseWriter for middleware that need it
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

// Middleware returns an HTTP middleware that logs requests using structured logging.
// It uses slog for JSON-compatible structured output and includes:
// - request_id: correlation ID from chi middleware
// - method: HTTP method
// - path: request path
// - status: response status code
// - bytes: response body size
// - duration: request duration
// - client_ip: real client IP (from realip middleware)
func Middleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap the response writer to capture status and bytes
			wrapped := &responseWriter{
				ResponseWriter: w,
				status:         http.StatusOK,
			}

			defer func() {
				// Get the real client IP from context (set by realip middleware)
				clientIP := realip.GetClientIP(r)

				logger.Info("request",
					"request_id", middleware.GetReqID(r.Context()),
					"method", r.Method,
					"path", r.URL.Path,
					"status", wrapped.status,
					"bytes", wrapped.bytes,
					"duration", time.Since(start).String(),
					"client_ip", clientIP,
				)
			}()

			next.ServeHTTP(wrapped, r)
		})
	}
}
