package storage

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"golang.org/x/mod/semver"
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

// nullIfEmpty returns nil for empty string (for NULL in DB), otherwise the string
func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// latestVersionBySemver returns the latest version from a list using semver sorting
func latestVersionBySemver(versions []string) string {
	if len(versions) == 0 {
		return ""
	}
	// Ensure v prefix for semver comparison
	withV := make([]string, len(versions))
	for i, v := range versions {
		if v != "" && v[0] != 'v' {
			withV[i] = "v" + v
		} else {
			withV[i] = v
		}
	}
	sort.Slice(withV, func(i, j int) bool {
		return semver.Compare(withV[i], withV[j]) > 0
	})
	latest := withV[0]
	if latest != "" && latest[0] == 'v' {
		return latest[1:]
	}
	return latest
}
