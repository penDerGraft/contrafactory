package validation

import (
	"testing"
)

func TestValidatePackageName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "my-package", false},
		{"valid with numbers", "my-package-v2", false},
		{"valid min length", "ab", false},
		{"too short", "a", true},
		{"too long", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true},
		{"starts with number", "1package", true},
		{"contains uppercase", "MyPackage", true},
		{"contains underscore", "my_package", true},
		{"consecutive hyphens", "my--package", true},
		{"ends with hyphen", "my-package-", true},
		{"path traversal", "my..package", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePackageName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePackageName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid semver", "1.0.0", false},
		{"valid with v prefix", "v1.0.0", false},
		{"valid prerelease", "1.0.0-beta.1", false},
		{"valid prerelease with v", "v1.0.0-rc.1", false},
		{"valid with build metadata", "1.0.0+build.123", false},
		{"valid major only", "1.0.0", false},
		{"invalid no minor", "1", true},
		{"invalid no patch", "1.0", true},
		{"invalid characters", "1.0.0-beta!", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVersion(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateVersion(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"1.0.0", "1.0.0"},
		{"v1.0.0", "1.0.0"},
		{"v1.0.0-beta", "1.0.0-beta"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeVersion(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizeVersion(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestIsPrerelease(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"1.0.0", false},
		{"1.0.0-beta", true},
		{"1.0.0-beta.1", true},
		{"1.0.0-rc.1", true},
		{"v1.0.0", false},
		{"v1.0.0-alpha", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := IsPrerelease(tt.input)
			if got != tt.expected {
				t.Errorf("IsPrerelease(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestResolveLatest(t *testing.T) {
	tests := []struct {
		name              string
		versions          []string
		includePrerelease bool
		expected          string
	}{
		{
			name:              "stable versions only",
			versions:          []string{"1.0.0", "1.1.0", "2.0.0"},
			includePrerelease: false,
			expected:          "2.0.0",
		},
		{
			name:              "exclude prerelease",
			versions:          []string{"1.0.0", "2.0.0-beta", "1.5.0"},
			includePrerelease: false,
			expected:          "1.5.0",
		},
		{
			name:              "include prerelease",
			versions:          []string{"1.0.0", "2.0.0-beta", "1.5.0"},
			includePrerelease: true,
			expected:          "2.0.0-beta",
		},
		{
			name:              "all prereleases, exclude",
			versions:          []string{"1.0.0-alpha", "1.0.0-beta"},
			includePrerelease: false,
			expected:          "1.0.0-beta", // Falls back to latest prerelease
		},
		{
			name:              "empty list",
			versions:          []string{},
			includePrerelease: false,
			expected:          "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveLatest(tt.versions, tt.includePrerelease)
			if got != tt.expected {
				t.Errorf("ResolveLatest() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestValidateAddress(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid address", "0x1234567890abcdef1234567890abcdef12345678", false},
		{"valid uppercase", "0x1234567890ABCDEF1234567890ABCDEF12345678", false},
		{"missing 0x", "1234567890abcdef1234567890abcdef12345678", true},
		{"too short", "0x1234", true},
		{"too long", "0x1234567890abcdef1234567890abcdef123456789", true},
		{"invalid characters", "0x1234567890abcdef1234567890abcdef1234567g", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAddress(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAddress(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}
