# Security Middleware Configuration

This document describes the security middleware features built into Contrafactory and how to configure them for production deployments.

## Overview

Contrafactory includes several security middleware components designed to protect the application when exposed to the internet:

| Middleware | Purpose |
|------------|---------|
| **Real IP Detection** | Extracts the real client IP from proxy headers |
| **Rate Limiting** | Per-IP request throttling to prevent abuse |
| **Security Filter** | Blocks common attack patterns and scanner probes |
| **Body Size Limit** | Prevents oversized request payloads |
| **Request Timeouts** | Configurable timeouts to prevent slow attacks |

## Middleware Execution Order

The middleware executes in this order for each request:

1. **Real IP Extraction** - Determines the actual client IP
2. **Security Filter** - Blocks malicious request patterns
3. **Body Size Limit** - Enforces maximum request body size
4. **Rate Limiting** - Applies per-IP rate limits
5. **Request ID** - Assigns correlation ID for tracing
6. **Logging** - Structured request/response logging
7. **Recovery** - Panic recovery
8. **Compression** - Response compression

Health check endpoints (`/health`, `/healthz`, `/readyz`) bypass the security filter and rate limiting to ensure reliable health checks from load balancers and orchestrators.

---

## Real IP Detection

When running behind a reverse proxy or load balancer, the server's `RemoteAddr` shows the proxy's IP, not the actual client. The Real IP middleware extracts the true client IP from `X-Forwarded-For` headers.

### Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `TRUST_PROXY` | `false` | Enable parsing of `X-Forwarded-For` headers |
| `TRUSTED_PROXIES` | `10.0.0.0/8,172.16.0.0/12,192.168.0.0/16` | Comma-separated list of trusted proxy CIDR ranges |

### Helm Values

```yaml
proxy:
  trustProxy: true
  trustedProxies:
    - "10.0.0.0/8"
    - "172.16.0.0/12"
    - "192.168.0.0/16"
```

### Security Considerations

- **Never enable `TRUST_PROXY` unless you are behind a proxy.** If enabled without a proxy, attackers can spoof their IP address.
- Only add your actual proxy/load balancer IP ranges to `TRUSTED_PROXIES`.
- The middleware processes `X-Forwarded-For` from right to left, skipping trusted proxy IPs until it finds the first untrusted IP (the real client).

### Example Configurations

**Behind AWS ALB/NLB:**
```yaml
proxy:
  trustProxy: true
  trustedProxies:
    - "10.0.0.0/8"      # VPC range
    - "172.16.0.0/12"   # VPC range
```

**Behind Cloudflare:**
```yaml
proxy:
  trustProxy: true
  trustedProxies:
    # Cloudflare IPv4 ranges (check https://cloudflare.com/ips for current list)
    - "173.245.48.0/20"
    - "103.21.244.0/22"
    - "103.22.200.0/22"
    - "103.31.4.0/22"
    - "141.101.64.0/18"
    - "108.162.192.0/18"
    - "190.93.240.0/20"
    - "188.114.96.0/20"
    - "197.234.240.0/22"
    - "198.41.128.0/17"
    - "162.158.0.0/15"
    - "104.16.0.0/13"
    - "104.24.0.0/14"
    - "172.64.0.0/13"
    - "131.0.72.0/22"
```

**Direct exposure (no proxy):**
```yaml
proxy:
  trustProxy: false  # Default - uses RemoteAddr directly
```

---

## Rate Limiting

Per-IP rate limiting uses a token bucket algorithm to throttle excessive requests. Each unique client IP gets its own bucket.

### Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `RATE_LIMIT_ENABLED` | `true` | Enable rate limiting |
| `RATE_LIMIT_RPM` | `300` | Requests allowed per minute per IP |
| `RATE_LIMIT_BURST` | `50` | Maximum burst size (concurrent requests) |
| `RATE_LIMIT_CLEANUP_MINUTES` | `10` | How often to clean up stale IP entries |

### Helm Values

```yaml
rateLimit:
  enabled: true
  requestsPerMin: 300
  burstSize: 50
  cleanupMinutes: 10
```

### How It Works

- Each IP gets a token bucket that refills at `requestsPerMin / 60` tokens per second
- The bucket can hold up to `burstSize` tokens
- Each request consumes one token
- If no tokens are available, the request is rejected with HTTP 429

### Response Headers

When rate limited, the response includes:
- `Retry-After: 60` - Suggested wait time in seconds
- `X-Rate-Limit-Exceeded: true` - Indicates rate limit was hit

### Response Body

```json
{
  "error": {
    "code": "RATE_LIMIT_EXCEEDED",
    "message": "Too many requests. Please try again later."
  }
}
```

### Tuning Guidelines

| Use Case | Recommended Settings |
|----------|---------------------|
| Public API (restrictive) | `requestsPerMin: 60`, `burstSize: 10` |
| Default (balanced) | `requestsPerMin: 300`, `burstSize: 50` |
| Internal/trusted clients | `requestsPerMin: 600`, `burstSize: 100` |
| High-volume CI/CD | `requestsPerMin: 1000`, `burstSize: 100` |
| Disable (trusted network) | `enabled: false` |

### Bypassed Endpoints

Health check endpoints are excluded from rate limiting:
- `/health`
- `/healthz`
- `/readyz`

---

## Security Filter

The security filter blocks requests matching known attack patterns, protecting against automated vulnerability scanners and common exploits.

### Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `SECURITY_FILTER_ENABLED` | `true` | Enable security filter |
| `SECURITY_MAX_BODY_SIZE_MB` | `50` | Maximum request body size in megabytes |

### Helm Values

```yaml
security:
  filterEnabled: true
  maxBodySizeMB: 50
```

### Blocked Patterns

The filter blocks requests with paths matching:

**Scanner Probes:**
- `/.php` - PHP scanner probes
- `/wp-admin`, `/wp-includes`, `/wp-content`, `/wp-login` - WordPress scanners
- `/xmlrpc.php` - WordPress XML-RPC
- `/phpmyadmin`, `/phpinfo` - PHP admin tools
- `/cgi-bin/` - CGI scanners
- `/admin/` - Admin panel scanners
- `/shell` - Web shell probes

**Sensitive Files:**
- `/.git/` - Git repository exposure
- `/.env` - Environment file exposure
- `/.htaccess`, `/.htpasswd` - Apache config files
- `/config.` - Configuration file access
- `/server-status` - Apache status page
- `/web-inf/` - Java WEB-INF directory

**Path Traversal:**
- `../` - Directory traversal
- `..%2f`, `..%5c` - URL-encoded traversal
- `%2e%2e/` - Double-encoded traversal
- `%00` - Null byte injection

### Response

Blocked requests receive a generic HTTP 400 response to avoid leaking information:

```json
{
  "error": {
    "code": "BAD_REQUEST",
    "message": "Invalid request"
  }
}
```

### Body Size Limit

The body size middleware prevents denial of service through oversized payloads. When a request exceeds the limit, reading the body returns an error.

Set `SECURITY_MAX_BODY_SIZE_MB` based on your expected maximum artifact size. For contract artifacts, 50MB is typically sufficient.

---

## Server Timeouts

HTTP server timeouts protect against slowloris attacks and resource exhaustion.

### Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `SERVER_READ_TIMEOUT` | `30` | Max time (seconds) to read request headers and body |
| `SERVER_WRITE_TIMEOUT` | `60` | Max time (seconds) to write response |
| `SERVER_IDLE_TIMEOUT` | `120` | Max time (seconds) to keep idle connections open |
| `SERVER_REQUEST_TIMEOUT` | `30` | Max time (seconds) for handler to process request |

### Helm Values

```yaml
serverTimeouts:
  readTimeout: 30
  writeTimeout: 60
  idleTimeout: 120
  requestTimeout: 30
```

### Timeout Descriptions

| Timeout | Purpose |
|---------|---------|
| **Read Timeout** | Protects against slow clients sending data. Starts when connection is accepted, ends when request body is fully read. |
| **Write Timeout** | Protects against slow clients receiving data. Starts after request headers are read, ends when response is written. |
| **Idle Timeout** | Controls keep-alive connection lifetime. Higher values allow connection reuse but consume resources. |
| **Request Timeout** | Total time allowed for request handler. Protects against hung handlers or slow backends. |

### Tuning Guidelines

| Scenario | Recommendation |
|----------|---------------|
| Large artifact uploads | Increase `writeTimeout` to allow time for processing |
| Slow clients expected | Increase `readTimeout` |
| Resource-constrained | Lower `idleTimeout` to free connections faster |
| Fast network/clients | Lower all timeouts for faster failure detection |

---

## Structured Logging

All requests are logged in structured JSON format for easy parsing by log aggregation systems.

### Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `LOG_LEVEL` | `info` | Minimum log level: `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | `json` | Log format: `json` or `text` |

### Helm Values

```yaml
logging:
  level: info
  format: json
```

### Log Fields

Each request log entry includes:

| Field | Description |
|-------|-------------|
| `request_id` | Unique correlation ID for the request |
| `method` | HTTP method (GET, POST, etc.) |
| `path` | Request path |
| `status` | Response status code |
| `bytes` | Response body size in bytes |
| `duration` | Request processing time |
| `client_ip` | Real client IP (from Real IP middleware) |

### Example Log Entry

```json
{
  "time": "2025-02-04T10:30:00Z",
  "level": "INFO",
  "msg": "request",
  "request_id": "abc123",
  "method": "GET",
  "path": "/api/v1/packages/mypackage",
  "status": 200,
  "bytes": 1234,
  "duration": "45.2ms",
  "client_ip": "203.0.113.50"
}
```

---

## Complete Example: Production Kubernetes Deployment

Here's a complete `values.yaml` example for a production Kubernetes deployment behind an ingress controller:

```yaml
# Production security configuration

# Trust the ingress controller's X-Forwarded-For headers
proxy:
  trustProxy: true
  trustedProxies:
    - "10.0.0.0/8"   # Cluster pod network

# Moderate rate limiting for public access
rateLimit:
  enabled: true
  requestsPerMin: 60
  burstSize: 10
  cleanupMinutes: 5

# Enable all security features
security:
  filterEnabled: true
  maxBodySizeMB: 100  # Allow larger artifacts

# Aggressive timeouts for public internet
serverTimeouts:
  readTimeout: 15
  writeTimeout: 30
  idleTimeout: 60
  requestTimeout: 30

# JSON logging for log aggregation
logging:
  level: info
  format: json
```

---

## Disabling Security Features

For development or trusted internal networks, security features can be disabled:

```yaml
# Development/internal configuration

proxy:
  trustProxy: false

rateLimit:
  enabled: false

security:
  filterEnabled: false
  maxBodySizeMB: 100

logging:
  level: debug
  format: text  # Human-readable for development
```

Or via environment variables:

```bash
export TRUST_PROXY=false
export RATE_LIMIT_ENABLED=false
export SECURITY_FILTER_ENABLED=false
export LOG_FORMAT=text
export LOG_LEVEL=debug
```

---

## Monitoring and Alerting

Consider setting up alerts for:

1. **High rate of 429 responses** - May indicate attack or legitimate traffic spike
2. **High rate of 400 responses from security filter** - May indicate scanning activity
3. **Requests with spoofed IPs** - If seeing unexpected IPs despite trusted proxies configuration
4. **Request duration outliers** - May indicate performance issues or attacks

Use the `request_id` field to correlate logs across your infrastructure for debugging.
