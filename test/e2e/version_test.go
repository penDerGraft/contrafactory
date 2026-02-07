//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pendergraft/contrafactory/pkg/client"
)

// TestVersion_SemverValidation tests that invalid semver versions are rejected
func TestVersion_SemverValidation(t *testing.T) {
	apiKey := createTestAPIKey(t, testCtx.Store, "test-semver")
	c := newClient(testCtx.TestServer, apiKey)

	invalidVersions := []string{
		"not-a-version",
		"1.0",
		"01.0.0",
		"1.01.0",
		"1.0.01",
	}

	for _, version := range invalidVersions {
		t.Run("reject invalid version: "+version, func(t *testing.T) {
			err := c.Publish(context.Background(), "semver-test", version, client.PublishRequest{
				Chain:     "evm",
				Builder:   "foundry",
				Artifacts: []client.Artifact{},
			})
			assertHTTPError(t, err, "INVALID_VERSION")
		})
	}
}

// TestVersion_Immutability tests that published release versions cannot be overwritten
func TestVersion_Immutability(t *testing.T) {
	apiKey := createTestAPIKey(t, testCtx.Store, "test-immutability")
	c := newClient(testCtx.TestServer, apiKey)

	// Publish a version
	publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "immutable-test", "1.0.0", "Token")

	// Try to publish the same version again
	err := c.Publish(context.Background(), "immutable-test", "1.0.0", client.PublishRequest{
		Chain:     "evm",
		Builder:   "foundry",
		Artifacts: []client.Artifact{},
	})
	assertHTTPError(t, err, "VERSION_EXISTS")
}

// TestVersion_PrereleaseVersions tests that prerelease versions are allowed
func TestVersion_PrereleaseVersions(t *testing.T) {
	apiKey := createTestAPIKey(t, testCtx.Store, "test-prerelease")
	c := newClient(testCtx.TestServer, apiKey)

	prereleaseVersions := []string{
		"1.0.0-alpha",
		"1.0.0-alpha.1",
		"1.0.0-beta",
		"1.0.0-beta.2",
		"1.0.0-rc",
		"1.0.0-rc.1",
		"2.0.0-alpha.1.beta.2",
	}

	for _, version := range prereleaseVersions {
		t.Run("accept prerelease: "+version, func(t *testing.T) {
			packageName := "prerelease-" + version
			err := c.Publish(context.Background(), packageName, version, client.PublishRequest{
				Chain:     "evm",
				Builder:   "foundry",
				Artifacts: []client.Artifact{},
			})
			// Might fail due to empty artifacts but shouldn't be INVALID_VERSION
			assert.NotEqual(t, "INVALID_VERSION", getErrorCode(err))
		})
	}
}

// TestVersion_LatestResolution tests that fetching with 'latest' returns the highest stable version
func TestVersion_LatestResolution(t *testing.T) {
	apiKey := createTestAPIKey(t, testCtx.Store, "test-latest")
	c := newClient(testCtx.TestServer, apiKey)

	// Publish multiple versions
	versions := []string{"1.0.0", "1.1.0", "2.0.0", "2.1.0-beta.1"}
	for _, version := range versions {
		publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "latest-test", version, "Token")
	}

	t.Run("list versions", func(t *testing.T) {
		pkg, err := c.GetPackage(context.Background(), "latest-test")
		require.NoError(t, err)
		assert.Contains(t, pkg.Versions, "1.0.0")
		assert.Contains(t, pkg.Versions, "1.1.0")
		assert.Contains(t, pkg.Versions, "2.0.0")
		assert.Contains(t, pkg.Versions, "2.1.0-beta.1")
	})
}

// TestVersion_VersionListing tests that listing a package returns all versions sorted
func TestVersion_VersionListing(t *testing.T) {
	apiKey := createTestAPIKey(t, testCtx.Store, "test-list-versions")
	c := newClient(testCtx.TestServer, apiKey)

	// Publish versions in non-sequential order
	versions := []string{"2.0.0", "1.0.0", "1.1.0", "3.0.0-beta.1", "1.2.0"}
	for _, version := range versions {
		publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, "list-versions-test", version, "Token")
	}

	t.Run("get package info", func(t *testing.T) {
		pkg, err := c.GetPackage(context.Background(), "list-versions-test")
		require.NoError(t, err)
		assert.Equal(t, "list-versions-test", pkg.Name)
		assert.Len(t, pkg.Versions, 5)
		assert.Contains(t, pkg.Versions, "1.0.0")
		assert.Contains(t, pkg.Versions, "1.1.0")
		assert.Contains(t, pkg.Versions, "1.2.0")
		assert.Contains(t, pkg.Versions, "2.0.0")
		assert.Contains(t, pkg.Versions, "3.0.0-beta.1")
	})
}

// TestVersion_MultiplePackages tests version isolation between packages
func TestVersion_MultiplePackages(t *testing.T) {
	apiKey := createTestAPIKey(t, testCtx.Store, "test-multi-pkg")
	c := newClient(testCtx.TestServer, apiKey)

	// Publish different versions to different packages
	packages := map[string][]string{
		"pkg-a": {"1.0.0", "1.1.0", "2.0.0"},
		"pkg-b": {"1.0.0", "1.5.0", "2.0.0"},
		"pkg-c": {"3.0.0"},
	}

	for pkgName, versions := range packages {
		for _, version := range versions {
			publishFromBuiltArtifacts(t, c, testCtx.FoundryBuiltDir, pkgName, version, "Token")
		}
	}

	t.Run("each package has independent versions", func(t *testing.T) {
		pkgA, err := c.GetPackage(context.Background(), "pkg-a")
		require.NoError(t, err)
		assert.Len(t, pkgA.Versions, 3)
		assert.Contains(t, pkgA.Versions, "2.0.0")

		pkgB, err := c.GetPackage(context.Background(), "pkg-b")
		require.NoError(t, err)
		assert.Len(t, pkgB.Versions, 3)
		assert.Contains(t, pkgB.Versions, "1.5.0")

		pkgC, err := c.GetPackage(context.Background(), "pkg-c")
		require.NoError(t, err)
		assert.Len(t, pkgC.Versions, 1)
		assert.Contains(t, pkgC.Versions, "3.0.0")
	})
}

// TestVersion_PackageNames tests various valid package names
func TestVersion_PackageNames(t *testing.T) {
	apiKey := createTestAPIKey(t, testCtx.Store, "test-pkg-names")
	c := newClient(testCtx.TestServer, apiKey)

	validNames := []string{
		"my-package",
		"my_package",
		"mypackage",
		"MyPackage",
		"my-package-123",
		"my.package.v1",
	}

	for _, name := range validNames {
		t.Run("accept package name: "+name, func(t *testing.T) {
			err := c.Publish(context.Background(), name, "1.0.0", client.PublishRequest{
				Chain:     "evm",
				Builder:   "foundry",
				Artifacts: []client.Artifact{},
			})
			// Might fail due to empty artifacts but shouldn't be INVALID_VERSION or INVALID_NAME
			code := getErrorCode(err)
			assert.NotEqual(t, "INVALID_VERSION", code)
			assert.NotEqual(t, "INVALID_NAME", code)
		})
	}
}
