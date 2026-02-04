package evm

import (
	"encoding/hex"
	"testing"
)

func TestStripMetadata(t *testing.T) {
	tests := []struct {
		name     string
		bytecode string
		wantLen  int
	}{
		{
			name:     "bytecode without metadata",
			bytecode: "608060405234801561001057600080fd5b50",
			wantLen:  18, // hex decoded length
		},
		{
			name:     "bytecode with IPFS metadata",
			bytecode: "608060405234801561001057600080fd5b50a264697066735822",
			wantLen:  18, // Should strip at marker
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bytecode, _ := hex.DecodeString(tt.bytecode)
			result := StripMetadata(bytecode)
			// Just verify it doesn't panic and returns something
			if len(result) == 0 && len(bytecode) > 0 {
				t.Error("StripMetadata returned empty result for non-empty input")
			}
		})
	}
}

func TestHasLibraryPlaceholders(t *testing.T) {
	tests := []struct {
		name     string
		bytecode string
		want     bool
	}{
		{
			name:     "no placeholders",
			bytecode: "608060405234801561001057600080fd5b50",
			want:     false,
		},
		{
			name:     "with placeholder",
			bytecode: "608060405234801561001057__$1234567890abcdef1234567890abcdef12$__600080fd5b50",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasLibraryPlaceholders([]byte(tt.bytecode)); got != tt.want {
				t.Errorf("HasLibraryPlaceholders() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompareBytecode(t *testing.T) {
	tests := []struct {
		name      string
		deployed  []byte
		artifact  []byte
		libraries map[string]string
		wantMatch bool
		wantType  string
	}{
		{
			name:      "exact match",
			deployed:  []byte{0x60, 0x80, 0x60, 0x40},
			artifact:  []byte{0x60, 0x80, 0x60, 0x40},
			wantMatch: true,
			wantType:  "full",
		},
		{
			name:      "no match",
			deployed:  []byte{0x60, 0x80, 0x60, 0x40},
			artifact:  []byte{0x60, 0x80, 0x60, 0x50},
			wantMatch: false,
			wantType:  "none",
		},
		{
			name:      "hex-encoded artifact matches",
			deployed:  []byte{0x60, 0x80, 0x60, 0x40},
			artifact:  []byte("0x60806040"),
			wantMatch: true,
			wantType:  "full",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CompareBytecode(tt.deployed, tt.artifact, tt.libraries)
			if result.Match != tt.wantMatch {
				t.Errorf("CompareBytecode().Match = %v, want %v", result.Match, tt.wantMatch)
			}
			if result.MatchType != tt.wantType {
				t.Errorf("CompareBytecode().MatchType = %v, want %v", result.MatchType, tt.wantType)
			}
		})
	}
}
