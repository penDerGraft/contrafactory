package ratelimit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimiter_AllowsRequests(t *testing.T) {
	cfg := Config{
		Enabled:        true,
		RequestsPerMin: 60, // 1 per second
		BurstSize:      5,
		CleanupMinutes: 1,
	}

	rl := New(cfg)
	defer rl.Stop()

	handler := rl.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First few requests should succeed (within burst)
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code, "Request %d should succeed", i+1)
	}
}

func TestRateLimiter_BlocksExcessRequests(t *testing.T) {
	cfg := Config{
		Enabled:        true,
		RequestsPerMin: 60,
		BurstSize:      2, // Small burst to hit limit quickly
		CleanupMinutes: 1,
	}

	rl := New(cfg)
	defer rl.Stop()

	handler := rl.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust the burst
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}

	// Next request should be rate limited
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusTooManyRequests, rr.Code)

	// Check response body
	var response map[string]any
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)
	errObj, ok := response["error"].(map[string]any)
	require.True(t, ok, "error should be an object")
	assert.Equal(t, "RATE_LIMIT_EXCEEDED", errObj["code"])
}

func TestRateLimiter_SeparateLimitsPerIP(t *testing.T) {
	cfg := Config{
		Enabled:        true,
		RequestsPerMin: 60,
		BurstSize:      2,
		CleanupMinutes: 1,
	}

	rl := New(cfg)
	defer rl.Stop()

	handler := rl.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust burst for IP 1
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}

	// IP 1 should be limited
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusTooManyRequests, rr.Code)

	// IP 2 should still have quota
	req2 := httptest.NewRequest("GET", "/api/test", nil)
	req2.RemoteAddr = "192.168.1.101:12345"
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	assert.Equal(t, http.StatusOK, rr2.Code)
}

func TestRateLimiter_BypassesHealthChecks(t *testing.T) {
	cfg := Config{
		Enabled:        true,
		RequestsPerMin: 60,
		BurstSize:      1, // Very restrictive
		CleanupMinutes: 1,
	}

	rl := New(cfg)
	defer rl.Stop()

	handler := rl.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	healthPaths := []string{"/health", "/healthz", "/readyz"}

	for _, path := range healthPaths {
		// Make many requests to the health endpoint
		for i := 0; i < 10; i++ {
			req := httptest.NewRequest("GET", path, nil)
			req.RemoteAddr = "192.168.1.100:12345"
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			assert.Equal(t, http.StatusOK, rr.Code, "Health check %s request %d should not be rate limited", path, i+1)
		}
	}
}

func TestRateLimiter_Disabled(t *testing.T) {
	cfg := Config{
		Enabled:        false,
		RequestsPerMin: 1, // Would be very restrictive if enabled
		BurstSize:      1,
		CleanupMinutes: 1,
	}

	// Use the Middleware() factory function which checks the Enabled flag
	handler := Middleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Many requests should succeed when disabled
	for i := 0; i < 100; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	}
}

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	cfg := Config{
		Enabled:        true,
		RequestsPerMin: 6000, // High enough to not hit limits
		BurstSize:      100,
		CleanupMinutes: 1,
	}

	rl := New(cfg)
	defer rl.Stop()

	handler := rl.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	var wg sync.WaitGroup
	numGoroutines := 10
	requestsPerGoroutine := 10

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for i := 0; i < requestsPerGoroutine; i++ {
				req := httptest.NewRequest("GET", "/api/test", nil)
				req.RemoteAddr = "192.168.1.100:12345"
				rr := httptest.NewRecorder()
				handler.ServeHTTP(rr, req)
				// Just ensure no panic
			}
		}(g)
	}

	wg.Wait()
}

func TestMiddleware_Factory(t *testing.T) {
	cfg := Config{
		Enabled:        true,
		RequestsPerMin: 60,
		BurstSize:      5,
		CleanupMinutes: 1,
	}

	// Test the Middleware factory function
	handler := Middleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestRateLimiter_CleanupStale(t *testing.T) {
	cfg := Config{
		Enabled:        true,
		RequestsPerMin: 60,
		BurstSize:      5,
		CleanupMinutes: 1, // Will be used for stale threshold
	}

	rl := New(cfg)
	defer rl.Stop()

	// Manually add an entry
	rl.getLimiter("test-ip")

	// Verify it exists
	rl.mu.Lock()
	_, exists := rl.limiters["test-ip"]
	rl.mu.Unlock()
	assert.True(t, exists, "IP should exist after getLimiter")

	// Simulate staleness by manipulating lastSeen
	rl.mu.Lock()
	if entry, ok := rl.limiters["test-ip"]; ok {
		entry.lastSeen = time.Now().Add(-2 * time.Minute)
	}
	rl.mu.Unlock()

	// Run cleanup
	rl.cleanup_stale()

	// Verify it was cleaned up
	rl.mu.Lock()
	_, exists = rl.limiters["test-ip"]
	rl.mu.Unlock()
	assert.False(t, exists, "Stale IP should be cleaned up")
}
