package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/pendergraft/contrafactory/internal/middleware/realip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testHandler returns a handler that writes a response with the given status and body
func testHandler(status int, body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(body))
	})
}

func TestMiddleware_LogsRequests(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))

	handler := Middleware(logger)(testHandler(http.StatusOK, "hello"))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Verify response
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "hello", rr.Body.String())

	// Parse log output
	var logEntry map[string]any
	err := json.Unmarshal(logBuf.Bytes(), &logEntry)
	require.NoError(t, err)

	// Verify log fields
	assert.Equal(t, "request", logEntry["msg"])
	assert.Equal(t, "GET", logEntry["method"])
	assert.Equal(t, "/api/test", logEntry["path"])
	assert.Equal(t, float64(http.StatusOK), logEntry["status"])
	assert.Equal(t, float64(5), logEntry["bytes"]) // "hello" = 5 bytes
	assert.Contains(t, logEntry, "duration")
	assert.Equal(t, "192.168.1.100", logEntry["client_ip"])
}

func TestMiddleware_LogsErrorStatus(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))

	handler := Middleware(logger)(testHandler(http.StatusInternalServerError, "error"))

	req := httptest.NewRequest("POST", "/api/error", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)

	var logEntry map[string]any
	err := json.Unmarshal(logBuf.Bytes(), &logEntry)
	require.NoError(t, err)

	assert.Equal(t, "POST", logEntry["method"])
	assert.Equal(t, "/api/error", logEntry["path"])
	assert.Equal(t, float64(http.StatusInternalServerError), logEntry["status"])
}

func TestMiddleware_CapturesResponseBytes(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))

	largeBody := "This is a larger response body for testing"
	handler := Middleware(logger)(testHandler(http.StatusOK, largeBody))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:8080"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var logEntry map[string]any
	err := json.Unmarshal(logBuf.Bytes(), &logEntry)
	require.NoError(t, err)

	assert.Equal(t, float64(len(largeBody)), logEntry["bytes"])
}

func TestMiddleware_IncludesRequestID(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))

	// Chain with RequestID middleware
	handler := middleware.RequestID(Middleware(logger)(testHandler(http.StatusOK, "")))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:8080"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var logEntry map[string]any
	err := json.Unmarshal(logBuf.Bytes(), &logEntry)
	require.NoError(t, err)

	// Request ID should be present and non-empty
	assert.Contains(t, logEntry, "request_id")
	assert.NotEmpty(t, logEntry["request_id"])
}

func TestMiddleware_UsesRealIPFromContext(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))

	// Chain with realip middleware
	realipMiddleware := realip.Middleware(realip.Config{
		TrustProxy:     true,
		TrustedProxies: []string{"10.0.0.0/8"},
	})

	handler := realipMiddleware(Middleware(logger)(testHandler(http.StatusOK, "")))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:12345" // Trusted proxy
	req.Header.Set("X-Forwarded-For", "203.0.113.50")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var logEntry map[string]any
	err := json.Unmarshal(logBuf.Bytes(), &logEntry)
	require.NoError(t, err)

	// Should log the real client IP, not the proxy
	assert.Equal(t, "203.0.113.50", logEntry["client_ip"])
}

func TestMiddleware_DefaultStatus200(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))

	// Handler that writes body without setting status
	handler := Middleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("no explicit status"))
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:8080"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var logEntry map[string]any
	err := json.Unmarshal(logBuf.Bytes(), &logEntry)
	require.NoError(t, err)

	// Default status should be 200
	assert.Equal(t, float64(http.StatusOK), logEntry["status"])
}

func TestResponseWriter_WriteHeader(t *testing.T) {
	rr := httptest.NewRecorder()
	rw := &responseWriter{
		ResponseWriter: rr,
		status:         http.StatusOK,
	}

	// First WriteHeader should work
	rw.WriteHeader(http.StatusNotFound)
	assert.Equal(t, http.StatusNotFound, rw.status)
	assert.True(t, rw.wroteHeader)

	// Second WriteHeader should be ignored
	rw.WriteHeader(http.StatusOK)
	assert.Equal(t, http.StatusNotFound, rw.status)
}

func TestResponseWriter_Write(t *testing.T) {
	rr := httptest.NewRecorder()
	rw := &responseWriter{
		ResponseWriter: rr,
		status:         http.StatusOK,
	}

	// Write should set status if not already set
	n, err := rw.Write([]byte("test"))
	require.NoError(t, err)
	assert.Equal(t, 4, n)
	assert.Equal(t, 4, rw.bytes)
	assert.True(t, rw.wroteHeader)

	// Multiple writes should accumulate bytes
	n, err = rw.Write([]byte("more"))
	require.NoError(t, err)
	assert.Equal(t, 4, n)
	assert.Equal(t, 8, rw.bytes)
}

func TestResponseWriter_Unwrap(t *testing.T) {
	rr := httptest.NewRecorder()
	rw := &responseWriter{
		ResponseWriter: rr,
		status:         http.StatusOK,
	}

	assert.Equal(t, rr, rw.Unwrap())
}

func TestMiddleware_LogsDuration(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))

	handler := Middleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Small delay to ensure duration > 0
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:8080"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var logEntry map[string]any
	err := json.Unmarshal(logBuf.Bytes(), &logEntry)
	require.NoError(t, err)

	// Duration should be present and a string (e.g., "123µs")
	duration, ok := logEntry["duration"].(string)
	assert.True(t, ok, "duration should be a string")
	assert.NotEmpty(t, duration)
}

func TestMiddleware_HandlesContextWithRequestID(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))

	handler := Middleware(logger)(testHandler(http.StatusOK, ""))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:8080"
	// Add request ID to context manually
	ctx := context.WithValue(req.Context(), middleware.RequestIDKey, "test-request-id-123")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var logEntry map[string]any
	err := json.Unmarshal(logBuf.Bytes(), &logEntry)
	require.NoError(t, err)

	assert.Equal(t, "test-request-id-123", logEntry["request_id"])
}
