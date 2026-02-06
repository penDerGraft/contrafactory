package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/pendergraft/contrafactory/internal/chains"
	"github.com/pendergraft/contrafactory/internal/chains/evm/foundry"
)

// PublishRequest matches the server's expected format
type PublishRequest struct {
	Chain     string            `json:"chain"`
	Builder   string            `json:"builder"`
	Artifacts []PublishArtifact `json:"artifacts"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// PublishArtifact represents a contract artifact to publish
type PublishArtifact struct {
	Name              string          `json:"name"`
	SourcePath        string          `json:"sourcePath"`
	ABI               json.RawMessage `json:"abi,omitempty"`
	Bytecode          string          `json:"bytecode,omitempty"`
	DeployedBytecode  string          `json:"deployedBytecode,omitempty"`
	StandardJSONInput json.RawMessage `json:"standardJsonInput,omitempty"`
}

func createPublishCmd() *cobra.Command {
	var version string
	var contracts []string
	var prefix string
	var dryRun bool
	var metadata []string

	cmd := &cobra.Command{
		Use:   "publish",
		Short: "Publish packages to the registry",
		Long: `Publish packages from a Foundry project to the Contrafactory registry.

Each contract becomes its own package, containing the contract's ABI, bytecode,
and verification artifacts. Package names default to the contract name.

REQUIREMENTS:
  Run 'forge build --build-info' before publishing to generate the
  Standard JSON Input needed for block explorer verification.

EXAMPLES:
  # Publish all contracts (one package per contract)
  contrafactory publish --version 1.0.0
  # Creates packages: Token@1.0.0, Registry@1.0.0, Factory@1.0.0

  # Publish with a prefix (for namespacing)
  contrafactory publish --version 1.0.0 --prefix myproject
  # Creates: myproject-Token@1.0.0, myproject-Registry@1.0.0, ...

  # Publish specific contracts only
  contrafactory publish --version 1.0.0 --contracts Token,Registry

  # Publish with metadata
  contrafactory publish --version 1.0.0 --metadata audit_status=passed --metadata auditor="Trail of Bits"

  # Dry run (show what would be published)
  contrafactory publish --version 1.0.0 --dry-run
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPublish(version, prefix, contracts, dryRun, metadata)
		},
	}

	cmd.Flags().StringVarP(&version, "version", "v", "", "version to publish (required)")
	cmd.Flags().StringSliceVar(&contracts, "contracts", nil, "specific contracts to publish (default: all)")
	cmd.Flags().StringVarP(&prefix, "prefix", "p", "", "prefix for package names (e.g., 'myproject' creates 'myproject-Token')")
	cmd.Flags().StringSliceVar(&metadata, "metadata", nil, "package metadata as key=value pairs (repeatable)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be published without publishing")
	_ = cmd.MarkFlagRequired("version")

	return cmd
}

func runPublish(version, prefix string, contracts []string, dryRun bool, metadataPairs []string) error {
	// Parse metadata key=value pairs
	metadata, err := parseMetadata(metadataPairs)
	if err != nil {
		return fmt.Errorf("parsing metadata: %w", err)
	}
	// Get current directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	// Detect builder
	builder := foundry.New()
	detected, err := builder.Detect(cwd)
	if err != nil {
		return fmt.Errorf("detecting builder: %w", err)
	}
	if !detected {
		return fmt.Errorf("no Foundry project detected (missing foundry.toml) - currently only Foundry projects are supported")
	}

	fmt.Printf("📦 Detected Foundry project in %s\n", cwd)

	// Discover artifacts
	// Exclude common test/script/mock patterns
	discoverOpts := chains.DiscoverOptions{
		Contracts: contracts,
		Exclude: []string{
			"Test",   // *Test contracts
			"Script", // *Script contracts
			"Mock",   // Mock* contracts
			"Deploy", // Deploy* scripts
			"Setup",  // *Setup test helpers
		},
	}

	artifactPaths, err := builder.Discover(cwd, discoverOpts)
	if err != nil {
		if strings.Contains(err.Error(), "build-info") {
			return fmt.Errorf("%w\n\n💡 TIP: Run 'forge build --build-info' first to generate the required build info files", err)
		}
		return fmt.Errorf("discovering artifacts: %w", err)
	}

	if len(artifactPaths) == 0 {
		return fmt.Errorf("no contract artifacts found\n\nMake sure you've run 'forge build' and have contracts in your src/ directory")
	}

	fmt.Printf("🔍 Found %d contract(s) in src/\n", len(artifactPaths))

	// Parse artifacts and prepare for publishing
	type packageToPublish struct {
		name     string
		artifact PublishArtifact
	}
	var packages []packageToPublish
	var skippedInterfaces []string

	for _, path := range artifactPaths {
		artifact, err := builder.Parse(path)
		if err != nil {
			// Skip interfaces and abstract contracts (no bytecode)
			if strings.Contains(err.Error(), "no bytecode") {
				contractName := strings.TrimSuffix(filepath.Base(path), ".json")
				skippedInterfaces = append(skippedInterfaces, contractName)
				continue
			}
			fmt.Printf("⚠️  Warning: skipping %s: %v\n", filepath.Base(path), err)
			continue
		}

		if artifact.EVM == nil {
			continue
		}

		pa := PublishArtifact{
			Name:             artifact.Name,
			SourcePath:       artifact.EVM.SourcePath,
			ABI:              artifact.EVM.ABI,
			Bytecode:         artifact.EVM.Bytecode,
			DeployedBytecode: artifact.EVM.DeployedBytecode,
		}

		// Try to get Standard JSON Input
		if stdJSON, err := builder.GenerateVerificationInput(cwd, artifact.Name); err == nil {
			pa.StandardJSONInput = stdJSON
		}

		// Package name = normalized contract name (with optional prefix)
		// PascalCase -> lowercase-with-hyphens (e.g., PredicateRegistry -> predicate-registry)
		packageName := normalizePackageName(artifact.Name)
		if prefix != "" {
			packageName = prefix + "-" + packageName
		}

		packages = append(packages, packageToPublish{
			name:     packageName,
			artifact: pa,
		})
		fmt.Printf("  ✓ %s → %s@%s\n", artifact.Name, packageName, version)
	}

	if len(packages) == 0 {
		return fmt.Errorf("no publishable contracts found (all were interfaces or had no bytecode)")
	}

	// Show skipped interfaces if any
	if len(skippedInterfaces) > 0 {
		fmt.Printf("  ⏭️  Skipped %d interface(s): %s\n", len(skippedInterfaces), strings.Join(skippedInterfaces, ", "))
	}

	if dryRun {
		fmt.Printf("\n🔍 DRY RUN - Would publish %d package(s) to %s\n", len(packages), getServer())
		for _, pkg := range packages {
			fmt.Printf("   - %s@%s\n", pkg.name, version)
		}
		return nil
	}

	// Publish each contract as its own package
	serverURL := getServer()
	fmt.Printf("\n📤 Publishing %d package(s) to %s...\n", len(packages), serverURL)

	var successCount, failCount int
	for _, pkg := range packages {
		err := publishPackage(serverURL, pkg.name, version, pkg.artifact, metadata)
		if err != nil {
			fmt.Printf("   ❌ %s@%s: %v\n", pkg.name, version, err)
			failCount++
		} else {
			fmt.Printf("   ✅ %s@%s\n", pkg.name, version)
			successCount++
		}
	}

	fmt.Println()
	if failCount > 0 {
		return fmt.Errorf("published %d package(s), %d failed", successCount, failCount)
	}

	fmt.Printf("✅ Successfully published %d package(s)\n", successCount)
	if len(packages) > 0 {
		fmt.Printf("\n   Example: contrafactory fetch %s@%s\n", packages[0].name, version)
	}

	return nil
}

// normalizePackageName converts a contract name to a valid package name.
// PascalCase/camelCase is converted to lowercase-with-hyphens.
// Example: PredicateRegistry -> predicate-registry
func normalizePackageName(name string) string {
	// Insert hyphens before uppercase letters (except at start)
	var result strings.Builder
	for i, r := range name {
		if i > 0 && unicode.IsUpper(r) {
			// Check if previous char was lowercase or next char is lowercase
			// This handles cases like "ERC20" -> "erc20" not "e-r-c-20"
			prev := rune(name[i-1])
			if unicode.IsLower(prev) {
				result.WriteRune('-')
			} else if i+1 < len(name) && unicode.IsLower(rune(name[i+1])) {
				result.WriteRune('-')
			}
		}
		result.WriteRune(unicode.ToLower(r))
	}

	// Replace any remaining invalid characters
	normalized := result.String()

	// Replace underscores with hyphens
	normalized = strings.ReplaceAll(normalized, "_", "-")

	// Remove consecutive hyphens
	re := regexp.MustCompile(`-+`)
	normalized = re.ReplaceAllString(normalized, "-")

	// Trim leading/trailing hyphens
	normalized = strings.Trim(normalized, "-")

	return normalized
}

// publishPackage publishes a single contract as its own package
func publishPackage(serverURL, packageName, version string, artifact PublishArtifact, metadata map[string]string) error {
	req := PublishRequest{
		Chain:     "evm",
		Builder:   "foundry",
		Artifacts: []PublishArtifact{artifact},
		Metadata:  metadata,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/packages/%s/%s", serverURL, packageName, version)
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if key := getAPIKey(); key != "" {
		httpReq.Header.Set("X-API-Key", key)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusCreated {
		var errResp map[string]any
		if json.Unmarshal(body, &errResp) == nil {
			if errObj, ok := errResp["error"].(map[string]any); ok {
				return fmt.Errorf("%s - %s", errObj["code"], errObj["message"])
			}
		}
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// parseMetadata parses key=value pairs into a map
func parseMetadata(pairs []string) (map[string]string, error) {
	metadata := make(map[string]string)
	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid metadata format: %s (expected key=value)", pair)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			return nil, fmt.Errorf("empty key in metadata: %s", pair)
		}
		metadata[key] = value
	}
	return metadata, nil
}
