//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/pendergraft/contrafactory/internal/chains"
	"github.com/pendergraft/contrafactory/internal/chains/evm"
	"github.com/pendergraft/contrafactory/internal/config"
	"github.com/pendergraft/contrafactory/internal/server"
	"github.com/pendergraft/contrafactory/internal/storage"
	"github.com/pendergraft/contrafactory/pkg/client"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestContext holds shared test infrastructure
type TestContext struct {
	PostgresContainer *postgres.PostgresContainer
	FoundryBuiltDir   string
	ConnString        string
	TestServer        *httptest.Server
	Store             storage.Store
}

// setupPostgres starts a Postgres container and returns the connection string
func setupPostgres(ctx context.Context, t *testing.T) (*postgres.PostgresContainer, string) {
	container, connStr, err := setupPostgresE(ctx)
	if err != nil {
		t.Fatalf("Failed to start postgres: %v", err)
	}
	return container, connStr
}

// setupPostgresE starts a Postgres container and returns the connection string (error-returning variant for TestMain)
func setupPostgresE(ctx context.Context) (*postgres.PostgresContainer, string, error) {
	postgresContainer, err := postgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:16-alpine"),
		postgres.WithDatabase("contrafactory"),
		postgres.WithUsername("contrafactory"),
		postgres.WithPassword("contrafactory"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		return nil, "", fmt.Errorf("failed to start postgres container: %w", err)
	}

	connString, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = postgresContainer.Terminate(ctx)
		return nil, "", fmt.Errorf("failed to get postgres connection string: %w", err)
	}

	return postgresContainer, connString, nil
}

// buildFoundryProject runs forge build in a Foundry container and returns the path to built artifacts
func buildFoundryProject(ctx context.Context, t *testing.T, projectDir string) string {
	builtDir, err := buildFoundryProjectE(projectDir)
	if err != nil {
		t.Fatalf("Failed to build Foundry project: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(builtDir)
	})
	return builtDir
}

// buildFoundryProjectE runs forge build in a Foundry container (error-returning variant for TestMain)
func buildFoundryProjectE(projectDir string) (string, error) {
	// Create temp directory for build output with world-writable permissions
	// so the Docker container can write to it regardless of the user it runs as
	builtDir := filepath.Join(os.TempDir(), fmt.Sprintf("foundry-out-%s", uuid.New().String()))

	if err := os.MkdirAll(builtDir, 0777); err != nil {
		return "", fmt.Errorf("failed to create build directory: %w", err)
	}

	// Get absolute path for the project
	absProjectDir, err := filepath.Abs(projectDir)
	if err != nil {
		os.RemoveAll(builtDir)
		return "", fmt.Errorf("failed to get absolute project path: %w", err)
	}

	// Verify the project directory exists
	if _, err := os.Stat(absProjectDir); err != nil {
		os.RemoveAll(builtDir)
		return "", fmt.Errorf("project directory does not exist %s: %w", absProjectDir, err)
	}

	fmt.Printf("Building Foundry project from: %s\n", absProjectDir)

	// Run docker run to build the project in one shot.
	// We mount the project directory as read-only and tell forge to write output
	// directly to /output (which has world-writable permissions). This avoids
	// permission errors in CI where the container user can't write to the
	// bind-mounted project directory.
	// #nosec G204 -- controlled command
	cmd := exec.Command("docker", "run", "--rm",
		"-v", absProjectDir+":/project:ro",
		"-v", builtDir+":/output",
		"-w", "/project",
		"--entrypoint", "/bin/sh",
		"ghcr.io/foundry-rs/foundry:latest",
		"-c", "forge build --build-info --out /output --cache-path /tmp/forge-cache")

	output, err := cmd.CombinedOutput()
	if err != nil {
		os.RemoveAll(builtDir)
		return "", fmt.Errorf("failed to build Foundry project: %w\nOutput: %s", err, string(output))
	}

	fmt.Println("Foundry build completed successfully")
	fmt.Printf("Build output: %s\n", string(output))

	// Verify the build output exists
	entries, err := os.ReadDir(builtDir)
	if err != nil {
		os.RemoveAll(builtDir)
		return "", fmt.Errorf("failed to read build directory: %w", err)
	}
	if len(entries) == 0 {
		os.RemoveAll(builtDir)
		return "", fmt.Errorf("build directory is empty")
	}

	return builtDir, nil
}

// extractTar extracts a tar.gz file to a directory
func extractTar(tarPath, destDir string) error {
	// This is a simple implementation - in production you'd use archive/tar
	// For now, use the system tar command
	// #nosec G204 - we control the input
	cmd := []string{"tar", "-xzf", tarPath, "-C", destDir}
	if err := runCommand(cmd); err != nil {
		return fmt.Errorf("extracting tar: %w", err)
	}
	return nil
}

// runCommand executes a command and returns an error if it fails
func runCommand(args []string) error {
	// #nosec G204 - we control the input
	cmd := exec.Command(args[0], args[1:]...)
	return cmd.Run()
}

// startServer starts the contrafactory server in-process with the given config
func startServer(t *testing.T, connString string) (*httptest.Server, storage.Store) {
	server, store, err := startServerE(connString)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	return server, store
}

// startServerE starts the contrafactory server in-process (error-returning variant for TestMain)
func startServerE(connString string) (*httptest.Server, storage.Store, error) {
	// Create config
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
			Host: "0.0.0.0",
		},
		Storage: config.StorageConfig{
			Type: "postgres",
			Postgres: config.PostgresConfig{
				URL: connString,
			},
		},
		Auth:      config.AuthConfig{Type: "api-key"},
		Cache:     config.CacheConfig{Enabled: false},
		Logging:   config.LoggingConfig{Level: "debug", Format: "text"},
		RateLimit: config.RateLimitConfig{Enabled: false},
		Security:  config.SecurityConfig{FilterEnabled: false, MaxBodySizeMB: 50},
		Proxy:     config.ProxyConfig{TrustProxy: false},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Create store
	store, err := storage.New(cfg.Storage, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create store: %w", err)
	}

	// Run migrations
	err = store.Migrate(context.Background())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	// Create chain registry
	registry := chains.NewRegistry()
	registry.Register(evm.NewChain())

	// Create server
	srv := server.New(cfg, store, logger)

	// Wrap server handler with auth bypass middleware for testing
	handler := srv.Handler()

	testServer := httptest.NewServer(handler)

	return testServer, store, nil
}

// newClient creates a new API client for the test server
func newClient(testServer *httptest.Server, apiKey string) *client.Client {
	return client.New(testServer.URL, apiKey)
}

// createTestAPIKey creates a test API key using the store directly
func createTestAPIKey(t *testing.T, store storage.Store, name string) string {
	key, err := store.CreateAPIKey(context.Background(), name)
	require.NoError(t, err, "Failed to create API key")
	return key
}

// FoundryArtifact represents a parsed Foundry build artifact
type FoundryArtifact struct {
	ABI               json.RawMessage `json:"abi"`
	Bytecode          struct {
		Object string `json:"object"`
	} `json:"bytecode"`
	DeployedBytecode struct {
		Object string `json:"object"`
	} `json:"deployedBytecode"`
	StorageLayout     json.RawMessage `json:"storageLayout"`
	Metadata          json.RawMessage `json:"metadata"`
}

// FoundryBuildInfo represents the build-info output from Foundry
type FoundryBuildInfo struct {
	Output struct {
		Contracts map[string]map[string]FoundryArtifact `json:"contracts"`
	} `json:"output"`
}

// parseFoundryArtifact parses a Foundry artifact JSON file
func parseFoundryArtifact(t *testing.T, artifactPath string) FoundryArtifact {
	data, err := os.ReadFile(artifactPath)
	require.NoError(t, err, "Failed to read artifact file")

	var artifact FoundryArtifact
	err = json.Unmarshal(data, &artifact)
	require.NoError(t, err, "Failed to parse artifact")

	return artifact
}

// getBuildInfo reads and parses the build-info JSON from Foundry
func getBuildInfo(t *testing.T, builtDir string) FoundryBuildInfo {
	buildInfoPath := filepath.Join(builtDir, "build-info")

	// Find the build-info JSON file
	entries, err := os.ReadDir(buildInfoPath)
	require.NoError(t, err, "Failed to read build-info directory")

	require.Len(t, entries, 1, "Expected exactly one build-info file")

	buildInfoPath = filepath.Join(buildInfoPath, entries[0].Name())

	data, err := os.ReadFile(buildInfoPath)
	require.NoError(t, err, "Failed to read build-info file")

	var buildInfo FoundryBuildInfo
	err = json.Unmarshal(data, &buildInfo)
	require.NoError(t, err, "Failed to parse build-info")

	return buildInfo
}

// publishFromBuiltArtifacts publishes a package from built Foundry artifacts
func publishFromBuiltArtifacts(t *testing.T, c *client.Client, builtDir, packageName, version string, contracts ...string) {
	artifacts := make([]client.Artifact, 0, len(contracts))

	for _, contractName := range contracts {
		// Read the actual contract JSON file (not build-info) to get bytecode
		artifactPath := getContractArtifactPath(builtDir, contractName)
		artifact := parseFoundryArtifact(t, artifactPath)

		// Use empty array as default to avoid null values
		storageLayout := json.RawMessage([]byte("[]"))
		if len(artifact.StorageLayout) > 0 {
			storageLayout = artifact.StorageLayout
		}

		// Parse metadata from contract JSON (metadata is an object, not a string like in build-info)
		var metadata struct {
			Compiler struct {
				Version  string `json:"version"`
				Settings struct {
					OptimizerEnabled bool `json:"optimizerEnabled"`
					OptimizerRuns   int  `json:"optimizerRuns"`
					EVMVersion      string `json:"evmVersion"`
					ViaIR           bool `json:"viaIR"`
				} `json:"settings"`
			} `json:"compiler"`
		}
		if len(artifact.Metadata) > 0 {
			require.NoError(t, json.Unmarshal(artifact.Metadata, &metadata), "Failed to parse metadata")
		}

		compiler := &client.CompilerInfo{
			Version: metadata.Compiler.Version,
		}
		if metadata.Compiler.Settings.OptimizerEnabled {
			compiler.Optimizer = &client.OptimizerInfo{
				Enabled: true,
				Runs:    metadata.Compiler.Settings.OptimizerRuns,
			}
		}
		compiler.EVMVersion = metadata.Compiler.Settings.EVMVersion
		compiler.ViaIR = metadata.Compiler.Settings.ViaIR

		// Use empty array as default to avoid null values
		abi := json.RawMessage([]byte("[]"))
		if len(artifact.ABI) > 0 {
			abi = artifact.ABI
		}

		artifacts = append(artifacts, client.Artifact{
			Name:              contractName,
			SourcePath:        fmt.Sprintf("src/%s.sol:%s", contractName, contractName),
			ABI:               abi,
			Bytecode:          artifact.Bytecode.Object,
			DeployedBytecode:  artifact.DeployedBytecode.Object,
			StorageLayout:     storageLayout,
			Compiler:          compiler,
		})
	}

	req := client.PublishRequest{
		Chain:     "evm",
		Builder:   "foundry",
		Artifacts: artifacts,
	}

	err := c.Publish(context.Background(), packageName, version, req)
	require.NoError(t, err, "Failed to publish package")
}

// getContractArtifactPath finds a contract's artifact file in the built output
func getContractArtifactPath(builtDir, contractName string) string {
	// Foundry stores artifacts at out/{contractName}.sol/{contractName}.json
	return filepath.Join(builtDir, fmt.Sprintf("%s.sol/%s.json", contractName, contractName))
}

// assertHTTPError asserts that an error is an APIError with the expected code
func assertHTTPError(t *testing.T, err error, expectedCode string) {
	t.Helper()
	require.Error(t, err, "Expected an error")
	apiErr, ok := err.(*client.APIError)
	require.True(t, ok, "Error should be an APIError")
	require.Equal(t, expectedCode, apiErr.Code, "Error code mismatch")
}
