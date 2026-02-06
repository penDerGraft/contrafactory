// Package realip provides middleware for extracting the real client IP
// from X-Forwarded-For headers when behind a trusted proxy.
package realip

import (
	"context"
	"net"
	"net/http"
	"strings"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const (
	// ClientIPKey is the context key for the real client IP
	ClientIPKey contextKey = "client_ip"
)

// Config holds the configuration for the real IP middleware
type Config struct {
	// TrustProxy enables X-Forwarded-For header parsing
	TrustProxy bool
	// TrustedProxies is a list of CIDR ranges for trusted proxies
	TrustedProxies []string
}

// Middleware returns an HTTP middleware that extracts the real client IP
// from X-Forwarded-For headers when the request comes from a trusted proxy.
func Middleware(cfg Config) func(http.Handler) http.Handler {
	// Parse trusted proxy CIDRs
	var trustedNets []*net.IPNet
	if cfg.TrustProxy {
		for _, cidr := range cfg.TrustedProxies {
			_, network, err := net.ParseCIDR(cidr)
			if err != nil {
				// Try parsing as a single IP
				if ip := net.ParseIP(cidr); ip != nil {
					if ip.To4() != nil {
						_, network, _ = net.ParseCIDR(cidr + "/32")
					} else {
						_, network, _ = net.ParseCIDR(cidr + "/128")
					}
				}
			}
			if network != nil {
				trustedNets = append(trustedNets, network)
			}
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientIP := extractClientIP(r, cfg.TrustProxy, trustedNets)
			ctx := context.WithValue(r.Context(), ClientIPKey, clientIP)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// extractClientIP gets the real client IP from the request
func extractClientIP(r *http.Request, trustProxy bool, trustedNets []*net.IPNet) string {
	// Get the remote address (strips port)
	remoteIP := extractIP(r.RemoteAddr)

	// If we don't trust proxies, just return the remote address
	if !trustProxy {
		return remoteIP
	}

	// Check if the remote address is a trusted proxy
	if !isTrustedProxy(remoteIP, trustedNets) {
		return remoteIP
	}

	// Parse X-Forwarded-For header
	// Format: client, proxy1, proxy2, ...
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		// Try X-Real-IP as fallback
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			return strings.TrimSpace(xri)
		}
		return remoteIP
	}

	// Split the XFF header and find the first non-trusted IP
	// We iterate from right to left to find the client IP
	ips := strings.Split(xff, ",")
	for i := len(ips) - 1; i >= 0; i-- {
		ip := strings.TrimSpace(ips[i])
		if ip == "" {
			continue
		}
		// If this IP is not a trusted proxy, it's the client
		if !isTrustedProxy(ip, trustedNets) {
			return ip
		}
	}

	// All IPs in the chain are trusted, return the leftmost (original client)
	if len(ips) > 0 {
		return strings.TrimSpace(ips[0])
	}

	return remoteIP
}

// extractIP extracts the IP address from an address:port string
func extractIP(addr string) string {
	// Try to parse as host:port
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// Maybe it's just an IP without port
		return addr
	}
	return host
}

// isTrustedProxy checks if an IP is in the trusted proxy list
func isTrustedProxy(ipStr string, trustedNets []*net.IPNet) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	for _, network := range trustedNets {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// GetClientIP retrieves the real client IP from the request context.
// Falls back to RemoteAddr if not set.
func GetClientIP(r *http.Request) string {
	if ip, ok := r.Context().Value(ClientIPKey).(string); ok && ip != "" {
		return ip
	}
	return extractIP(r.RemoteAddr)
}
