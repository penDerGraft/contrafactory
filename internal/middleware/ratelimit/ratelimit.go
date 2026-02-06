// Package ratelimit provides per-IP rate limiting middleware using token bucket algorithm.
package ratelimit

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/pendergraft/contrafactory/internal/middleware/realip"
)

// Config holds the configuration for rate limiting
type Config struct {
	// Enabled enables rate limiting
	Enabled bool
	// RequestsPerMin is the number of requests allowed per minute per IP
	RequestsPerMin int
	// BurstSize is the maximum burst size
	BurstSize int
	// CleanupMinutes is how often to clean up stale entries
	CleanupMinutes int
}

// ipLimiter tracks a rate limiter and its last access time
type ipLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiter manages per-IP rate limiters
type RateLimiter struct {
	mu       sync.RWMutex
	limiters map[string]*ipLimiter
	rate     rate.Limit
	burst    int
	cleanup  time.Duration
	stopCh   chan struct{}
}

// New creates a new RateLimiter with the given configuration
func New(cfg Config) *RateLimiter {
	// Convert requests per minute to rate.Limit (requests per second)
	r := rate.Limit(float64(cfg.RequestsPerMin) / 60.0)

	cleanupDuration := time.Duration(cfg.CleanupMinutes) * time.Minute
	if cleanupDuration <= 0 {
		cleanupDuration = 10 * time.Minute
	}

	rl := &RateLimiter{
		limiters: make(map[string]*ipLimiter),
		rate:     r,
		burst:    cfg.BurstSize,
		cleanup:  cleanupDuration,
		stopCh:   make(chan struct{}),
	}

	// Start cleanup goroutine
	go rl.cleanupLoop()

	return rl
}

// Stop stops the cleanup goroutine
func (rl *RateLimiter) Stop() {
	close(rl.stopCh)
}

// cleanupLoop periodically removes stale IP entries
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.cleanup)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.cleanup_stale()
		case <-rl.stopCh:
			return
		}
	}
}

// cleanup_stale removes entries that haven't been seen recently
func (rl *RateLimiter) cleanup_stale() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-rl.cleanup)
	for ip, limiter := range rl.limiters {
		if limiter.lastSeen.Before(cutoff) {
			delete(rl.limiters, ip)
		}
	}
}

// getLimiter gets or creates a rate limiter for the given IP
func (rl *RateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if limiter, exists := rl.limiters[ip]; exists {
		limiter.lastSeen = time.Now()
		return limiter.limiter
	}

	limiter := rate.NewLimiter(rl.rate, rl.burst)
	rl.limiters[ip] = &ipLimiter{
		limiter:  limiter,
		lastSeen: time.Now(),
	}
	return limiter
}

// healthCheckPaths are exempt from rate limiting
var healthCheckPaths = map[string]bool{
	"/health":  true,
	"/healthz": true,
	"/readyz":  true,
}

// Middleware returns an HTTP middleware that rate limits requests per IP
func (rl *RateLimiter) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Bypass rate limiting for health checks
			if healthCheckPaths[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			// Get the real client IP
			clientIP := realip.GetClientIP(r)

			// Get or create a limiter for this IP
			limiter := rl.getLimiter(clientIP)

			if !limiter.Allow() {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "60")
				w.Header().Set("X-Rate-Limit-Exceeded", "true")
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]any{
						"code":    "RATE_LIMIT_EXCEEDED",
						"message": "Too many requests. Please try again later.",
					},
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Middleware returns a rate limiting middleware with the given configuration.
// This is a convenience function that creates a RateLimiter and returns its middleware.
// Note: The returned RateLimiter's cleanup goroutine will run for the lifetime of the process.
func Middleware(cfg Config) func(http.Handler) http.Handler {
	if !cfg.Enabled {
		// Return a no-op middleware if rate limiting is disabled
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	rl := New(cfg)
	return rl.Middleware()
}
