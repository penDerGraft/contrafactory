package security

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterMiddleware_Disabled(t *testing.T) {
	handler := FilterMiddleware(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Even malicious paths should pass when disabled
	maliciousPaths := []string{
		"/wp-admin/",
		"/.git/config",
		"/../etc/passwd",
		"/phpinfo.php",
	}

	for _, path := range maliciousPaths {
		req := httptest.NewRequest("GET", path, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code, "Path %s should pass when filter disabled", path)
	}
}

func TestFilterMiddleware_BlocksWordPressPaths(t *testing.T) {
	handler := FilterMiddleware(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	blockedPaths := []string{
		"/wp-admin/",
		"/wp-admin/index.php",
		"/wp-includes/something.php",
		"/wp-content/uploads/",
		"/wp-login.php",
		"/xmlrpc.php",
	}

	for _, path := range blockedPaths {
		req := httptest.NewRequest("GET", path, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code, "Path %s should be blocked", path)
	}
}

func TestFilterMiddleware_BlocksScannerProbes(t *testing.T) {
	handler := FilterMiddleware(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	blockedPaths := []string{
		"/.php",
		"/.git/config",
		"/.env",
		"/phpmyadmin/",
		"/phpinfo.php",
		"/cgi-bin/script.cgi",
		"/admin/login",
		"/.htaccess",
		"/.htpasswd",
		"/server-status",
		"/shell.php",
		"/config.php",
	}

	for _, path := range blockedPaths {
		req := httptest.NewRequest("GET", path, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code, "Path %s should be blocked", path)
	}
}

func TestFilterMiddleware_BlocksPathTraversal(t *testing.T) {
	handler := FilterMiddleware(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	blockedPaths := []string{
		"/../../etc/passwd",
		"/files/../../../etc/passwd",
		"/foo%2e%2e/bar", // Double URL-encoded ..
	}

	for _, path := range blockedPaths {
		req := httptest.NewRequest("GET", path, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code, "Path %s should be blocked", path)
	}
}


func TestFilterMiddleware_BypassesHealthChecks(t *testing.T) {
	handler := FilterMiddleware(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	healthPaths := []string{"/health", "/healthz", "/readyz"}

	for _, path := range healthPaths {
		req := httptest.NewRequest("GET", path, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code, "Health path %s should bypass filter", path)
	}
}

func TestFilterMiddleware_AllowsLegitimateRequests(t *testing.T) {
	handler := FilterMiddleware(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	legitimatePaths := []string{
		"/",
		"/api/v1/users",
		"/api/v1/products/123",
		"/static/css/style.css",
		"/images/logo.png",
		"/about",
		"/contact",
		"/search?q=test",
	}

	for _, path := range legitimatePaths {
		req := httptest.NewRequest("GET", path, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code, "Legitimate path %s should be allowed", path)
	}
}

func TestFilterMiddleware_CaseInsensitive(t *testing.T) {
	handler := FilterMiddleware(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Test case variations that should still be blocked
	blockedPaths := []string{
		"/WP-ADMIN/",
		"/Wp-Admin/",
		"/.GIT/config",
		"/.ENV",
		"/PHPMYADMIN/",
	}

	for _, path := range blockedPaths {
		req := httptest.NewRequest("GET", path, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code, "Path %s (case variation) should be blocked", path)
	}
}

func TestFilterMiddleware_ResponseFormat(t *testing.T) {
	handler := FilterMiddleware(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/wp-admin/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var response map[string]any
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	errObj, ok := response["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "BAD_REQUEST", errObj["code"])
	assert.Equal(t, "Invalid request", errObj["message"])
}

func TestMaxBodySizeMiddleware_AllowsSmallBody(t *testing.T) {
	handler := MaxBodySizeMiddleware(1)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))

	smallBody := []byte("small body")
	req := httptest.NewRequest("POST", "/api/data", bytes.NewReader(smallBody))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "small body", rr.Body.String())
}

func TestMaxBodySizeMiddleware_RejectsLargeBody(t *testing.T) {
	// 1 MB limit
	handler := MaxBodySizeMiddleware(1)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	// Create a body larger than 1 MB
	largeBody := strings.Repeat("x", 2*1024*1024) // 2 MB
	req := httptest.NewRequest("POST", "/api/data", strings.NewReader(largeBody))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// The handler should return an error because reading the body fails
	assert.Equal(t, http.StatusRequestEntityTooLarge, rr.Code)
}

func TestMaxBodySizeMiddleware_ExactLimit(t *testing.T) {
	// 1 MB limit
	handler := MaxBodySizeMiddleware(1)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	// Create a body exactly at the limit
	exactBody := strings.Repeat("x", 1*1024*1024) // 1 MB
	req := httptest.NewRequest("POST", "/api/data", strings.NewReader(exactBody))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Should succeed at exact limit
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestMaxBodySizeMiddleware_NoBody(t *testing.T) {
	handler := MaxBodySizeMiddleware(1)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/data", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}
