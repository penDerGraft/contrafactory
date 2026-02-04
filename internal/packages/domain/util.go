package domain

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/google/uuid"
)

// generateID generates a new UUID.
func generateID() string {
	return uuid.New().String()
}

// computeHash computes a SHA256 hash of content.
func computeHash(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}
