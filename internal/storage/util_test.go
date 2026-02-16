package storage

import (
	"testing"
)

func TestLatestVersionBySemver(t *testing.T) {
	tests := []struct {
		name     string
		versions []string
		want     string
	}{
		{"empty list", []string{}, ""},
		{"single version", []string{"1.0.0"}, "1.0.0"},
		{"multiple versions ascending", []string{"1.0.0", "1.1.0", "2.0.0"}, "2.0.0"},
		{"multiple versions descending", []string{"2.0.0", "1.1.0", "1.0.0"}, "2.0.0"},
		{"multiple versions unsorted", []string{"1.1.0", "0.9.0", "1.0.0"}, "1.1.0"},
		{"with v prefix", []string{"v1.0.0", "v1.1.0"}, "1.1.0"},
		{"mixed v prefix", []string{"1.0.0", "v1.1.0"}, "1.1.0"},
		{"prerelease", []string{"1.0.0", "1.0.0-beta.1"}, "1.0.0"},
		{"prerelease vs release", []string{"1.0.0-beta.1", "1.0.0-alpha.1"}, "1.0.0-beta.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := latestVersionBySemver(tt.versions)
			if got != tt.want {
				t.Errorf("latestVersionBySemver(%v) = %v, want %v", tt.versions, got, tt.want)
			}
		})
	}
}
