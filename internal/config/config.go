package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds all configuration for the server
type Config struct {
	Server    ServerConfig
	Storage   StorageConfig
	Auth      AuthConfig
	Cache     CacheConfig
	Logging   LoggingConfig
	RateLimit RateLimitConfig
	Security  SecurityConfig
	Proxy     ProxyConfig
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port           int
	Host           string
	ReadTimeout    int // seconds
	WriteTimeout   int // seconds
	IdleTimeout    int // seconds
	RequestTimeout int // seconds
}

// StorageConfig holds storage configuration
type StorageConfig struct {
	Type     string // "sqlite" or "postgres"
	Postgres PostgresConfig
	SQLite   SQLiteConfig
	Blobs    BlobsConfig
}

// PostgresConfig holds Postgres connection settings
type PostgresConfig struct {
	URL string
}

// SQLiteConfig holds SQLite settings
type SQLiteConfig struct {
	Path string
}

// BlobsConfig holds blob storage settings
type BlobsConfig struct {
	Type     string // "postgres", "filesystem", "s3"
	BasePath string // for filesystem
}

// AuthConfig holds authentication settings
type AuthConfig struct {
	Type string // "none" or "api-key"
}

// CacheConfig holds cache settings
type CacheConfig struct {
	Enabled    bool
	MaxSizeMB  int
	TTLSeconds int
}

// LoggingConfig holds logging settings
type LoggingConfig struct {
	Level  string
	Format string // "text" or "json"
}

// RateLimitConfig holds rate limiting settings
type RateLimitConfig struct {
	Enabled        bool
	RequestsPerMin int
	BurstSize      int
	CleanupMinutes int
}

// SecurityConfig holds security filter settings
type SecurityConfig struct {
	FilterEnabled bool
	MaxBodySizeMB int
}

// ProxyConfig holds trusted proxy settings for X-Forwarded-For handling
type ProxyConfig struct {
	TrustProxy     bool
	TrustedProxies []string // CIDR notation
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port:           getEnvInt("PORT", 8080),
			Host:           getEnv("HOST", "0.0.0.0"),
			ReadTimeout:    getEnvInt("SERVER_READ_TIMEOUT", 30),
			WriteTimeout:   getEnvInt("SERVER_WRITE_TIMEOUT", 60),
			IdleTimeout:    getEnvInt("SERVER_IDLE_TIMEOUT", 120),
			RequestTimeout: getEnvInt("SERVER_REQUEST_TIMEOUT", 30),
		},
		Storage: StorageConfig{
			Type: getEnv("STORAGE_TYPE", "sqlite"),
			Postgres: PostgresConfig{
				URL: getEnv("DATABASE_URL", ""),
			},
			SQLite: SQLiteConfig{
				Path: getEnv("SQLITE_PATH", "./data/contrafactory.db"),
			},
			Blobs: BlobsConfig{
				Type:     getEnv("BLOB_STORAGE_TYPE", ""),
				BasePath: getEnv("BLOB_STORAGE_PATH", "./data/blobs"),
			},
		},
		Auth: AuthConfig{
			Type: getEnv("AUTH_TYPE", "none"),
		},
		Cache: CacheConfig{
			Enabled:    getEnvBool("CACHE_ENABLED", true),
			MaxSizeMB:  getEnvInt("CACHE_MAX_SIZE_MB", 100),
			TTLSeconds: getEnvInt("CACHE_TTL_SECONDS", 3600),
		},
		Logging: LoggingConfig{
			Level:  getEnv("LOG_LEVEL", "info"),
			Format: getEnv("LOG_FORMAT", "json"),
		},
		RateLimit: RateLimitConfig{
			Enabled:        getEnvBool("RATE_LIMIT_ENABLED", true),
			RequestsPerMin: getEnvInt("RATE_LIMIT_RPM", 300),
			BurstSize:      getEnvInt("RATE_LIMIT_BURST", 50),
			CleanupMinutes: getEnvInt("RATE_LIMIT_CLEANUP_MINUTES", 10),
		},
		Security: SecurityConfig{
			FilterEnabled: getEnvBool("SECURITY_FILTER_ENABLED", true),
			MaxBodySizeMB: getEnvInt("SECURITY_MAX_BODY_SIZE_MB", 50),
		},
		Proxy: ProxyConfig{
			TrustProxy:     getEnvBool("TRUST_PROXY", false),
			TrustedProxies: getEnvStringSlice("TRUSTED_PROXIES", []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}),
		},
	}

	// If DATABASE_URL is set, default to postgres
	if cfg.Storage.Postgres.URL != "" && cfg.Storage.Type == "sqlite" {
		cfg.Storage.Type = "postgres"
	}

	// Default blob storage to same as main storage
	if cfg.Storage.Blobs.Type == "" {
		cfg.Storage.Blobs.Type = cfg.Storage.Type
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return strings.ToLower(value) == "true" || value == "1"
	}
	return defaultValue
}

func getEnvStringSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		parts := strings.Split(value, ",")
		result := make([]string, 0, len(parts))
		for _, p := range parts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				result = append(result, trimmed)
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return defaultValue
}
