// Package metrics provides Prometheus instrumentation for contrafactory.
package metrics

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// unmatchedPath is the path label used for requests that matched no route,
// so unknown URLs cannot create new time series.
const unmatchedPath = "unmatched"

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

			// Label with the route template, not the request URL, so label
			// values are bounded by the route table.
			path := routePattern(r)

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

// routePattern returns the matched route template, for example
// "/api/v1/packages/{name}/{version}". It must be called after the router has
// handled the request. Requests that matched no route share a single label.
func routePattern(r *http.Request) string {
	rctx := chi.RouteContext(r.Context())
	if rctx == nil {
		return unmatchedPath
	}
	pattern := rctx.RoutePattern()
	if pattern == "" || strings.HasSuffix(pattern, "/*") {
		return unmatchedPath
	}
	return pattern
}
