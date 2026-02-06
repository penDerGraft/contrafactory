// Package security provides security-related HTTP middleware.
package security

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

// Config holds the configuration for security middleware
type Config struct {
	// FilterEnabled enables the security filter
	FilterEnabled bool
	// MaxBodySizeMB is the maximum request body size in megabytes
	MaxBodySizeMB int
}

// healthCheckPaths are exempt from security filtering
var healthCheckPaths = map[string]bool{
	"/health":  true,
	"/healthz": true,
	"/readyz":  true,
}

// blockedPathPrefixes are path prefixes that indicate scanner/attack traffic
var blockedPathPrefixes = []string{
	"/.php",
	"/wp-admin",
	"/wp-includes",
	"/wp-content",
	"/wp-login",
	"/.git/",
	"/.env",
	"/web-inf/",
	"/cgi-bin/",
	"/admin/",
	"/phpmyadmin",
	"/phpinfo",
	"/shell",
	"/config.",
	"/.htaccess",
	"/.htpasswd",
	"/server-status",
	"/xmlrpc.php",
}

// blockedPathPatterns are patterns that indicate malicious requests
var blockedPathPatterns = []string{
	"../",     // Path traversal
	"..%2f",   // URL-encoded path traversal
	"..%5c",   // URL-encoded backslash traversal
	"%2e%2e/", // Double URL-encoded path traversal
	"%00",     // Null byte injection
}

// FilterMiddleware returns middleware that blocks requests matching known attack patterns.
// It checks for common scanner probes, path traversal attempts, and other malicious patterns.
func FilterMiddleware(enabled bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if !enabled {
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Bypass filtering for health checks
			if healthCheckPaths[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			// Get the path in lowercase for matching
			path := strings.ToLower(r.URL.Path)

			// Check for blocked path prefixes
			for _, prefix := range blockedPathPrefixes {
				if strings.HasPrefix(path, prefix) {
					writeBlockedResponse(w)
					return
				}
			}

			// Check for blocked patterns anywhere in the path
			for _, pattern := range blockedPathPatterns {
				if strings.Contains(path, pattern) {
					writeBlockedResponse(w)
					return
				}
			}

			// Also check the raw URL in case of encoding tricks
			rawPath := r.URL.RawPath
			if rawPath == "" {
				rawPath = r.URL.Path
			}

			// URL decode and check again for traversal
			decoded, err := url.PathUnescape(rawPath)
			if err == nil && decoded != path {
				decodedLower := strings.ToLower(decoded)
				for _, pattern := range blockedPathPatterns {
					if strings.Contains(decodedLower, pattern) {
						writeBlockedResponse(w)
						return
					}
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// writeBlockedResponse writes a generic 400 response without revealing what triggered the block
func writeBlockedResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    "BAD_REQUEST",
			"message": "Invalid request",
		},
	})
}
