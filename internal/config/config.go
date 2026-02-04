package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds all configuration for the server
type Config struct {
	Server  ServerConfig
	Storage StorageConfig
	Auth    AuthConfig
	Cache   CacheConfig
	Logging LoggingConfig
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port int
	Host string
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

// Load loads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port: getEnvInt("PORT", 8080),
			Host: getEnv("HOST", "0.0.0.0"),
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
			Format: getEnv("LOG_FORMAT", "text"),
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
