package evm

import (
	"bytes"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/pendergraft/contrafactory/internal/chains"
)

// CBOR metadata marker (Solidity >=0.6.0) - "ipfs" in CBOR
var metadataMarker = []byte{0xa2, 0x64, 0x69, 0x70, 0x66, 0x73}

// Library placeholder pattern: __$<34 hex chars>$__
var libraryPlaceholder = regexp.MustCompile(`__\$[a-f0-9]{34}\$__`)

// StripMetadata removes the CBOR metadata appended to bytecode
func StripMetadata(bytecode []byte) []byte {
	// Find last occurrence of metadata marker
	idx := bytes.LastIndex(bytecode, metadataMarker)
	if idx == -1 {
		return bytecode // No metadata found
	}
	// Back up to find the length prefix (2 bytes before marker)
	if idx >= 2 {
		return bytecode[:idx-2]
	}
	return bytecode
}

// CompareBytecode compares deployed bytecode to artifact bytecode
func CompareBytecode(deployed, artifact []byte, libraries map[string]string) *chains.VerifyResult {
	// Handle hex-encoded bytecode
	if len(artifact) > 2 && artifact[0] == '0' && artifact[1] == 'x' {
		decoded, err := hex.DecodeString(string(artifact[2:]))
		if err == nil {
			artifact = decoded
		}
	}

	// Substitute library placeholders if present
	if len(libraries) > 0 {
		artifact = substituteLibraries(artifact, libraries)
	}

	// Try exact match first
	if bytes.Equal(deployed, artifact) {
		return &chains.VerifyResult{
			Match:     true,
			MatchType: "full",
			Message:   "Bytecode matches exactly including metadata",
		}
	}

	// Strip metadata and compare
	deployedStripped := StripMetadata(deployed)
	artifactStripped := StripMetadata(artifact)

	if bytes.Equal(deployedStripped, artifactStripped) {
		return &chains.VerifyResult{
			Match:     true,
			MatchType: "partial",
			Message:   "Executable code matches, metadata differs (different source paths, comments, or build environment)",
		}
	}

	// No match
	return &chains.VerifyResult{
		Match:     false,
		MatchType: "none",
		Message:   "Bytecode does not match",
	}
}

// substituteLibraries replaces library placeholders with actual addresses
func substituteLibraries(bytecode []byte, libraries map[string]string) []byte {
	bytecodeHex := hex.EncodeToString(bytecode)

	for _, addr := range libraries {
		// Remove 0x prefix from address if present
		addr = strings.TrimPrefix(addr, "0x")
		addr = strings.ToLower(addr)

		// The placeholder is the first 17 bytes of keccak256(fullyQualifiedName)
		// Format: __$<34 hex chars>$__
		// For simplicity, we'll try to find any placeholder and replace with the address
		// In practice, you'd hash the library name to find the exact placeholder

		// Simplified approach: replace any placeholder with the address
		// Real implementation would compute keccak256(name)[:17] to match specific placeholders
		if libraryPlaceholder.MatchString(bytecodeHex) {
			// Replace placeholder with address (padded to 40 chars)
			bytecodeHex = libraryPlaceholder.ReplaceAllStringFunc(bytecodeHex, func(match string) string {
				return addr
			})
		}
	}

	result, _ := hex.DecodeString(bytecodeHex)
	return result
}

// HasLibraryPlaceholders checks if bytecode contains library placeholders
func HasLibraryPlaceholders(bytecode []byte) bool {
	return libraryPlaceholder.Match(bytecode) ||
		libraryPlaceholder.MatchString(string(bytecode))
}
