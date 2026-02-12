// Package metrics provides Prometheus instrumentation for contrafactory.
package metrics

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Middleware returns HTTP middleware for request metrics.
func Middleware(next http.Handler) http.Handler {
	if !enabled {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		defer func() {
			duration := time.Since(start).Seconds()

			// Normalize path to avoid high cardinality from IDs
			path := normalizePath(r.URL.Path)

			httpRequestsTotal.WithLabelValues(
				r.Method,
				path,
				strconv.Itoa(rw.status),
			).Inc()

			httpDuration.WithLabelValues(
				r.Method,
				path,
			).Observe(duration)
		}()

		next.ServeHTTP(rw, r)
	})
}

// responseWriter wraps http.ResponseWriter to capture status code.
type responseWriter struct {
	http.ResponseWriter
	status int
}

// WriteHeader captures status code.
func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

// normalizePath converts dynamic path segments to placeholders to avoid
// high cardinality metrics. For example:
//
//	/api/v1/packages/abc123def456/version -> /api/v1/packages/{id}/version
//	/api/v1/deployments/0x1234.../verify -> /api/v1/deployments/{id}/verify
func normalizePath(path string) string {
	// Health check endpoints - keep as-is
	if path == "/health" || path == "/healthz" || path == "/readyz" {
		return path
	}
	// Metrics endpoint - keep as-is
	if path == "/metrics" {
		return path
	}

	// API v1 paths - normalize dynamic segments
	// Pattern: /api/v1/{resource}/{id}/{action}
	// Extract resource type and normalize
	if strings.HasPrefix(path, "/api/v1/") {
		parts := strings.Split(path[8:], "/")
		if len(parts) < 2 {
			return path
		}
		resource := parts[1] // "packages", "deployments", etc.

		// Rebuild with normalized segments
		var normalized []string
		normalized = append(normalized, "/api/v1", resource)

		// Add remaining parts, replacing ID segments with placeholders
		for i := 2; i < len(parts); i++ {
			part := parts[i]
			// Skip empty parts
			if part == "" {
				continue
			}
			// Replace likely ID segments with placeholders
			if isLikelyID(part) {
				normalized = append(normalized, "{id}")
			} else {
				normalized = append(normalized, part)
			}
		}
		return "/" + strings.Join(normalized, "/")
	}
	return path
}

// isLikelyID returns true if segment looks like an identifier
func isLikelyID(segment string) bool {
	// Hash strings (common for transaction IDs, contract addresses)
	if len(segment) >= 64 && isHex(segment) {
		return true
	}
	// UUIDs with dashes
	if strings.Count(segment, "-") >= 4 {
		return true
	}
	// Pure numbers (could be chain IDs)
	if isNumeric(segment) {
		return true
	}
	return false
}

// isHex returns true if string is hexadecimal (supports both upper and lowercase)
func isHex(s string) bool {
	for _, c := range s {
		isDigit := c >= '0' && c <= '9'
		isLowerHex := c >= 'a' && c <= 'f'
		isUpperHex := c >= 'A' && c <= 'F'
		if !isDigit && !isLowerHex && !isUpperHex {
			return false
		}
	}
	return len(s) > 0
}

// isNumeric returns true if string contains only digits
func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}
