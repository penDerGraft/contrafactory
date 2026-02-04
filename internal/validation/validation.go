// Package validation provides input validation for Contrafactory.
package validation

import (
	"errors"
	"regexp"
	"strings"

	"golang.org/x/mod/semver"
)

// Package name validation
// Simple names: lowercase alphanumeric with hyphens, 2-64 chars
var packageNameRegex = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}[a-z0-9]$`)

// ValidatePackageName validates a package name
func ValidatePackageName(name string) error {
	if len(name) < 2 {
		return errors.New("package name too short (min 2 chars)")
	}
	if len(name) > 64 {
		return errors.New("package name too long (max 64 chars)")
	}
	if !packageNameRegex.MatchString(name) {
		return errors.New("invalid package name: must be lowercase alphanumeric with hyphens, starting with a letter")
	}
	// Prevent path traversal and consecutive hyphens
	if strings.Contains(name, "..") || strings.Contains(name, "--") {
		return errors.New("invalid characters in package name")
	}
	return nil
}

// ValidateVersion validates a semantic version string
func ValidateVersion(v string) error {
	// Normalize: strip leading 'v' if present, then add it back for semver library
	normalized := strings.TrimPrefix(v, "v")
	if normalized == "" {
		return errors.New("version cannot be empty")
	}

	// semver library expects version to start with 'v'
	versionWithV := "v" + normalized
	if !semver.IsValid(versionWithV) {
		return errors.New("invalid semver version: must be in format X.Y.Z or X.Y.Z-prerelease")
	}

	// Ensure we have major.minor.patch (not just major or major.minor)
	// semver.Canonical will add .0 if needed, so we can check if the normalized
	// version already has all three parts
	parts := strings.SplitN(normalized, "-", 2) // Split off prerelease/build
	mainPart := parts[0]
	dotCount := strings.Count(mainPart, ".")
	if dotCount < 2 {
		return errors.New("invalid semver version: must be in format X.Y.Z (major.minor.patch)")
	}

	return nil
}

// NormalizeVersion normalizes a version string (strips leading 'v')
func NormalizeVersion(v string) string {
	return strings.TrimPrefix(v, "v")
}

// IsPrerelease checks if a version is a prerelease
func IsPrerelease(v string) bool {
	normalized := "v" + NormalizeVersion(v)
	return semver.Prerelease(normalized) != ""
}

// CompareVersions compares two versions
// Returns -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
func CompareVersions(v1, v2 string) int {
	n1 := "v" + NormalizeVersion(v1)
	n2 := "v" + NormalizeVersion(v2)
	return semver.Compare(n1, n2)
}

// ResolveLatest finds the latest version from a list
func ResolveLatest(versions []string, includePrerelease bool) string {
	if len(versions) == 0 {
		return ""
	}

	var candidates []string
	for _, v := range versions {
		if !includePrerelease && IsPrerelease(v) {
			continue
		}
		candidates = append(candidates, v)
	}

	if len(candidates) == 0 {
		// If no stable versions, return latest prerelease
		candidates = versions
	}

	// Sort versions
	latest := candidates[0]
	for _, v := range candidates[1:] {
		if CompareVersions(v, latest) > 0 {
			latest = v
		}
	}

	return latest
}

// ValidateAddress validates an Ethereum address
func ValidateAddress(addr string) error {
	if len(addr) != 42 {
		return errors.New("invalid address length: must be 42 characters (0x + 40 hex)")
	}
	if !strings.HasPrefix(addr, "0x") {
		return errors.New("invalid address: must start with 0x")
	}
	// Check hex characters
	for _, c := range addr[2:] {
		isDigit := c >= '0' && c <= '9'
		isLowerHex := c >= 'a' && c <= 'f'
		isUpperHex := c >= 'A' && c <= 'F'
		if !isDigit && !isLowerHex && !isUpperHex {
			return errors.New("invalid address: contains non-hex characters")
		}
	}
	return nil
}

// ValidateChainID validates a chain ID
func ValidateChainID(chainID int) error {
	if chainID <= 0 {
		return errors.New("chain ID must be positive")
	}
	return nil
}
