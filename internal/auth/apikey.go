package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const (
	// KeyPrefix is the prefix for all API keys
	KeyPrefix = "cf_key_"
	// KeyLength is the length of the random part of the key
	KeyLength = 32
)

// GenerateAPIKey generates a new API key.
func GenerateAPIKey() (string, error) {
	bytes := make([]byte, KeyLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generating random bytes: %w", err)
	}
	return KeyPrefix + hex.EncodeToString(bytes), nil
}

// HashAPIKey hashes an API key for storage.
func HashAPIKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}
