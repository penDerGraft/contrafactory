//go:build e2e

package e2e

import (
	"context"
	"flag"
	"log"
	"os"
	"testing"
)

var testCtx *TestContext

func TestMain(m *testing.M) {
	// Parse flags
	flag.Parse()

	// Check if Docker is available (testcontainers requirement)
	if os.Getenv("DOCKER_HOST") == "" && os.Getenv("TESTCONTAINERS_DOCKER_SOCKET") == "" {
		// testcontainers will use default docker socket, which should work on most systems
		log.Println("Using default Docker socket for testcontainers")
	}

	ctx := context.Background()

	// Setup test infrastructure
	testCtx = &TestContext{}

	// 1. Start Postgres container
	log.Println("Starting Postgres container...")
	var err error
	testCtx.PostgresContainer, testCtx.ConnString, err = setupPostgresE(ctx)
	if err != nil {
		log.Fatalf("Failed to start postgres: %v", err)
	}
	defer func() {
		if err := testCtx.PostgresContainer.Terminate(ctx); err != nil {
			log.Printf("Failed to terminate postgres container: %v", err)
		}
	}()
	log.Println("Postgres container started")

	// 2. Build Foundry project
	log.Println("Building Foundry project...")
	projectDir := "testdata/sample-foundry-project"
	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		projectDir = "../../testdata/sample-foundry-project"
	}
	testCtx.FoundryBuiltDir, err = buildFoundryProjectE(projectDir)
	if err != nil {
		log.Fatalf("Failed to build Foundry project: %v", err)
	}
	defer os.RemoveAll(testCtx.FoundryBuiltDir)
	log.Println("Foundry project built, artifacts at:", testCtx.FoundryBuiltDir)

	// 3. Start test server
	log.Println("Starting test server...")
	testCtx.TestServer, testCtx.Store, err = startServerE(testCtx.ConnString)
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	defer testCtx.TestServer.Close()
	log.Println("Test server started at:", testCtx.TestServer.URL)

	// Run tests
	log.Println("Running E2E tests...")
	exitCode := m.Run()

	log.Println("E2E tests completed with exit code:", exitCode)
	os.Exit(exitCode)
}
