package storage

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/google/uuid"
)

// generateID generates a new UUID
func generateID() string {
	return uuid.New().String()
}

// computeHash computes SHA256 hash of content
func computeHash(content []byte) string {
	h := sha256.Sum256(content)
	return hex.EncodeToString(h[:])
}

// generateAPIKey generates a new API key
func generateAPIKey() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return fmt.Sprintf("cf_key_%s", hex.EncodeToString(b))
}

// hashAPIKey hashes an API key for storage
func hashAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}
