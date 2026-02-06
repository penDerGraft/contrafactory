# Contrafactory Documentation

This directory contains technical documentation for operating Contrafactory.

## Documentation Index

### Operations

| Document | Description |
|----------|-------------|
| [Security Middleware](./security-middleware.md) | Rate limiting, real IP detection, security filtering, and request timeouts |

## Quick Reference

### Environment Variables

#### Server

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP server port |
| `HOST` | `0.0.0.0` | HTTP server bind address |
| `SERVER_READ_TIMEOUT` | `30` | Read timeout in seconds |
| `SERVER_WRITE_TIMEOUT` | `60` | Write timeout in seconds |
| `SERVER_IDLE_TIMEOUT` | `120` | Idle timeout in seconds |
| `SERVER_REQUEST_TIMEOUT` | `30` | Request handler timeout in seconds |

#### Storage

| Variable | Default | Description |
|----------|---------|-------------|
| `STORAGE_TYPE` | `sqlite` | Storage backend: `sqlite` or `postgres` |
| `DATABASE_URL` | - | PostgreSQL connection string |
| `SQLITE_PATH` | `./data/contrafactory.db` | SQLite database path |
| `BLOB_STORAGE_TYPE` | (same as STORAGE_TYPE) | Blob storage: `postgres`, `filesystem`, `s3` |
| `BLOB_STORAGE_PATH` | `./data/blobs` | Filesystem blob storage path |

#### Authentication

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_TYPE` | `none` | Authentication type: `none` or `api-key` |

#### Caching

| Variable | Default | Description |
|----------|---------|-------------|
| `CACHE_ENABLED` | `true` | Enable in-memory caching |
| `CACHE_MAX_SIZE_MB` | `100` | Maximum cache size in MB |
| `CACHE_TTL_SECONDS` | `3600` | Cache entry TTL in seconds |

#### Logging

| Variable | Default | Description |
|----------|---------|-------------|
| `LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | `json` | Log format: `json` or `text` |

#### Rate Limiting

| Variable | Default | Description |
|----------|---------|-------------|
| `RATE_LIMIT_ENABLED` | `true` | Enable per-IP rate limiting |
| `RATE_LIMIT_RPM` | `300` | Requests per minute per IP |
| `RATE_LIMIT_BURST` | `50` | Maximum burst size |
| `RATE_LIMIT_CLEANUP_MINUTES` | `10` | Stale entry cleanup interval |

#### Security

| Variable | Default | Description |
|----------|---------|-------------|
| `SECURITY_FILTER_ENABLED` | `true` | Enable security filter |
| `SECURITY_MAX_BODY_SIZE_MB` | `50` | Maximum request body size in MB |

#### Proxy / Real IP

| Variable | Default | Description |
|----------|---------|-------------|
| `TRUST_PROXY` | `false` | Trust X-Forwarded-For headers |
| `TRUSTED_PROXIES` | `10.0.0.0/8,172.16.0.0/12,192.168.0.0/16` | Trusted proxy CIDR ranges |

## Health Endpoints

| Endpoint | Purpose |
|----------|---------|
| `/health` | Basic health check |
| `/healthz` | Kubernetes liveness probe |
| `/readyz` | Kubernetes readiness probe |

These endpoints bypass rate limiting and security filtering to ensure reliable health checks.

## Helm Chart

The Helm chart is located at `charts/contrafactory/`. See the chart's `values.yaml` for all available configuration options.

```bash
# Install from repository
helm repo add contrafactory https://pendergraft.github.io/contrafactory
helm install contrafactory contrafactory/contrafactory

# Install from source
helm install contrafactory ./charts/contrafactory
```
