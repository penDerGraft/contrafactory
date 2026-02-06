package realip

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMiddleware_TrustProxyDisabled(t *testing.T) {
	cfg := Config{
		TrustProxy:     false,
		TrustedProxies: []string{"10.0.0.0/8"},
	}

	var capturedIP string
	handler := Middleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedIP = GetClientIP(r)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.50")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Should use RemoteAddr, not X-Forwarded-For
	assert.Equal(t, "192.168.1.100", capturedIP)
}

func TestMiddleware_TrustProxyEnabled_TrustedProxy(t *testing.T) {
	cfg := Config{
		TrustProxy:     true,
		TrustedProxies: []string{"10.0.0.0/8", "192.168.0.0/16"},
	}

	var capturedIP string
	handler := Middleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedIP = GetClientIP(r)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:12345" // Trusted proxy
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 10.0.0.5")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Should use the first non-trusted IP from X-Forwarded-For
	assert.Equal(t, "203.0.113.50", capturedIP)
}

func TestMiddleware_TrustProxyEnabled_UntrustedProxy(t *testing.T) {
	cfg := Config{
		TrustProxy:     true,
		TrustedProxies: []string{"10.0.0.0/8"},
	}

	var capturedIP string
	handler := Middleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedIP = GetClientIP(r)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.100:12345" // Not a trusted proxy
	req.Header.Set("X-Forwarded-For", "203.0.113.50")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Should use RemoteAddr because proxy is not trusted
	assert.Equal(t, "192.168.1.100", capturedIP)
}

func TestMiddleware_XRealIP_Fallback(t *testing.T) {
	cfg := Config{
		TrustProxy:     true,
		TrustedProxies: []string{"10.0.0.0/8"},
	}

	var capturedIP string
	handler := Middleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedIP = GetClientIP(r)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Real-IP", "203.0.113.50")
	// No X-Forwarded-For

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Should use X-Real-IP as fallback
	assert.Equal(t, "203.0.113.50", capturedIP)
}

func TestMiddleware_MultipleProxiesInChain(t *testing.T) {
	cfg := Config{
		TrustProxy:     true,
		TrustedProxies: []string{"10.0.0.0/8", "172.16.0.0/12"},
	}

	var capturedIP string
	handler := Middleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedIP = GetClientIP(r)
		w.WriteHeader(http.StatusOK)
	}))

	// Request passes through multiple trusted proxies
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 172.16.0.1, 10.0.0.2")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Should find the first non-trusted IP (the original client)
	assert.Equal(t, "203.0.113.50", capturedIP)
}

func TestMiddleware_AllTrustedProxies(t *testing.T) {
	cfg := Config{
		TrustProxy:     true,
		TrustedProxies: []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"},
	}

	var capturedIP string
	handler := Middleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedIP = GetClientIP(r)
		w.WriteHeader(http.StatusOK)
	}))

	// All IPs in the chain are trusted
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "192.168.1.1, 172.16.0.1, 10.0.0.2")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Should return the leftmost IP (original client)
	assert.Equal(t, "192.168.1.1", capturedIP)
}

func TestMiddleware_NoForwardedHeader(t *testing.T) {
	cfg := Config{
		TrustProxy:     true,
		TrustedProxies: []string{"10.0.0.0/8"},
	}

	var capturedIP string
	handler := Middleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedIP = GetClientIP(r)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	// No X-Forwarded-For or X-Real-IP

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Should fall back to RemoteAddr
	assert.Equal(t, "10.0.0.1", capturedIP)
}

func TestGetClientIP_NoContext(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.100:12345"

	// Without middleware, should fall back to RemoteAddr
	ip := GetClientIP(req)
	assert.Equal(t, "192.168.1.100", ip)
}

func TestExtractIP(t *testing.T) {
	tests := []struct {
		addr     string
		expected string
	}{
		{"192.168.1.100:12345", "192.168.1.100"},
		{"10.0.0.1:80", "10.0.0.1"},
		{"192.168.1.100", "192.168.1.100"}, // No port
		{"[::1]:8080", "::1"},              // IPv6 with port
		{"::1", "::1"},                     // IPv6 without port
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			result := extractIP(tt.addr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsTrustedProxy(t *testing.T) {
	trustedNets := parseNetworks([]string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"})
	require.Len(t, trustedNets, 3)

	tests := []struct {
		ip       string
		expected bool
	}{
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"192.168.0.1", true},
		{"192.168.255.255", true},
		{"203.0.113.50", false},  // Public IP
		{"8.8.8.8", false},       // Google DNS
		{"172.32.0.1", false},    // Just outside 172.16/12
		{"invalid", false},       // Invalid IP
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			result := isTrustedProxy(tt.ip, trustedNets)
			assert.Equal(t, tt.expected, result, "IP: %s", tt.ip)
		})
	}
}

// parseNetworks is a test helper that parses CIDR strings
func parseNetworks(cidrs []string) []*net.IPNet {
	var networks []*net.IPNet
	for _, cidr := range cidrs {
		_, network, err := net.ParseCIDR(cidr)
		if err == nil && network != nil {
			networks = append(networks, network)
		}
	}
	return networks
}
