package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	Project   string            `json:"project,omitempty"`
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
	Compiler          *CompilerInfo   `json:"compiler,omitempty"`
}

// CompilerInfo is compiler metadata for verification
type CompilerInfo struct {
	Version    string         `json:"version"`
	Optimizer  *OptimizerInfo `json:"optimizer,omitempty"`
	EVMVersion string         `json:"evmVersion,omitempty"`
	ViaIR      bool           `json:"viaIR,omitempty"`
}

// OptimizerInfo contains optimizer settings
type OptimizerInfo struct {
	Enabled bool `json:"enabled"`
	Runs    int  `json:"runs"`
}

// Default exclude patterns
var defaultExcludePatterns = []string{
	"Test",   // *Test contracts
	"Script", // *Script contracts
	"Mock",   // Mock* contracts
	"Deploy", // Deploy* scripts
	"Setup",  // *Setup test helpers
}

// DiscoveredPackage is a package discovered by the project's discovery logic
type DiscoveredPackage struct {
	Name     string
	Path     string           // path to artifact JSON
	Artifact *chains.Artifact // pre-parsed for reuse (avoids redundant Parse calls)
}

// discoverPackages discovers packages using the same logic as publish.
// Returns package names and artifact paths. Used by both publish and delete.
func discoverPackages(cwd, prefix string, contracts, exclude, excludePaths, includeDeps []string) ([]DiscoveredPackage, error) {
	builder := foundry.New()
	detected, err := builder.Detect(cwd)
	if err != nil {
		return nil, fmt.Errorf("detecting builder: %w", err)
	}
	if !detected {
		return nil, fmt.Errorf("no Foundry project detected (missing foundry.toml) - currently only Foundry projects are supported")
	}

	discoverOpts := chains.DiscoverOptions{
		Contracts:           contracts,
		Exclude:             exclude,
		ExcludePaths:        excludePaths,
		IncludeDependencies: includeDeps,
	}

	artifactPaths, err := builder.Discover(cwd, discoverOpts)
	if err != nil {
		if strings.Contains(err.Error(), "build-info") {
			return nil, fmt.Errorf("%w\n\nTIP: Run 'forge build --build-info' first to generate the required build info files", err)
		}
		return nil, fmt.Errorf("discovering artifacts: %w", err)
	}

	if len(artifactPaths) == 0 {
		return nil, fmt.Errorf("no contract artifacts found\n\nMake sure you've run 'forge build' and have contracts in your src/ directory")
	}

	if len(includeDeps) > 0 {
		if err := validateDependencies(builder, cwd, includeDeps, artifactPaths); err != nil {
			return nil, err
		}
	}

	var packages []DiscoveredPackage
	for _, path := range artifactPaths {
		artifact, err := builder.Parse(path)
		if err != nil {
			if strings.Contains(err.Error(), "no bytecode") {
				continue
			}
			fmt.Printf("Warning: skipping %s: %v\n", filepath.Base(path), err)
			continue
		}

		if artifact.EVM == nil {
			continue
		}

		packageName := normalizePackageName(artifact.Name)
		if prefix != "" {
			packageName = prefix + "-" + packageName
		}

		packages = append(packages, DiscoveredPackage{Name: packageName, Path: path, Artifact: artifact})
	}

	if len(packages) == 0 {
		return nil, fmt.Errorf("no publishable contracts found (all were interfaces or had no bytecode)")
	}

	return packages, nil
}

func createPublishCmd() *cobra.Command {
	var version string
	var contracts []string
	var exclude []string
	var excludePaths []string
	var includeDeps []string
	var prefix string
	var project string
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

  # Publish with dependency contracts from lib/
  contrafactory publish --version 1.0.0 --include-deps TransparentUpgradeableProxy,ProxyAdmin

  # Publish with metadata
  contrafactory publish --version 1.0.0 --metadata audit_status=passed --metadata auditor="Trail of Bits"

  # Dry run (show what would be published)
  contrafactory publish --version 1.0.0 --dry-run
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPublish(version, prefix, project, contracts, exclude, excludePaths, includeDeps, dryRun, metadata)
		},
	}

	cmd.Flags().StringVarP(&version, "version", "v", "", "version to publish (required)")
	cmd.Flags().StringSliceVar(&contracts, "contracts", nil, "specific contracts to publish (default: all from src/)")
	cmd.Flags().StringSliceVar(&exclude, "exclude", nil, "patterns to exclude by contract name (e.g., Test,Mock) - replaces config defaults")
	cmd.Flags().StringSliceVar(&excludePaths, "exclude-path", nil, "patterns to exclude by source path (e.g., proxy, examples/MetaCoin.sol)")
	cmd.Flags().StringSliceVar(&includeDeps, "include-deps", nil, "dependency contracts to publish from lib/")
	cmd.Flags().StringVarP(&prefix, "prefix", "p", "", "prefix for package names (e.g., 'myproject' creates 'myproject-Token')")
	cmd.Flags().StringVar(&project, "project", "", "project scope (overrides contrafactory.toml)")
	cmd.Flags().StringSliceVar(&metadata, "metadata", nil, "package metadata as key=value pairs (repeatable)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be published without publishing")
	_ = cmd.MarkFlagRequired("version")

	return cmd
}

func runPublish(version, prefix, projectFlag string, contracts, exclude, excludePaths, includeDeps []string, dryRun bool, metadataPairs []string) error {
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

	// Load project config (optional)
	projectConfig := loadProjectConfigSilent()

	// Resolve contracts: CLI flag > config > default (all from src/)
	if len(contracts) == 0 && projectConfig != nil {
		contracts = projectConfig.Contracts
	}

	// Resolve exclude: CLI flag > config > hardcoded defaults
	excludePatterns := defaultExcludePatterns
	if len(exclude) > 0 {
		excludePatterns = exclude
	} else if projectConfig != nil && len(projectConfig.Exclude) > 0 {
		excludePatterns = projectConfig.Exclude
	}

	// Resolve exclude_paths: CLI flag > config
	excludePathPatterns := excludePaths
	if len(excludePathPatterns) == 0 && projectConfig != nil {
		excludePathPatterns = projectConfig.ExcludePaths
	}

	// Resolve include_dependencies: CLI flag > config
	if len(includeDeps) == 0 && projectConfig != nil {
		includeDeps = projectConfig.IncludeDependencies
	}

	// Discover packages (same logic used by delete)
	discovered, err := discoverPackages(cwd, prefix, contracts, excludePatterns, excludePathPatterns, includeDeps)
	if err != nil {
		return err
	}

	builder := foundry.New()
	fmt.Printf("Detected Foundry project in %s\n", cwd)

	// Count src vs dependency contracts for output
	srcCount, depCount := 0, 0
	for _, pkg := range discovered {
		if pkg.Artifact == nil || pkg.Artifact.EVM == nil {
			continue
		}
		if strings.HasPrefix(pkg.Artifact.EVM.SourcePath, "src/") {
			srcCount++
		} else {
			depCount++
		}
	}
	if srcCount > 0 {
		fmt.Printf("Found %d contract(s) in src/\n", srcCount)
	}
	if depCount > 0 {
		fmt.Printf("Found %d dependency contract(s) via include_dependencies\n", depCount)
	}

	// Parse artifacts and prepare for publishing
	type packageToPublish struct {
		name       string
		artifact   PublishArtifact
		isDep      bool
		sourcePath string
	}
	var packages []packageToPublish

	for _, pkg := range discovered {
		artifact := pkg.Artifact
		if artifact == nil || artifact.EVM == nil {
			continue
		}

		pa := PublishArtifact{
			Name:             artifact.Name,
			SourcePath:       artifact.EVM.SourcePath,
			ABI:              artifact.EVM.ABI,
			Bytecode:         artifact.EVM.Bytecode,
			DeployedBytecode: artifact.EVM.DeployedBytecode,
		}

		// Compiler info: prefer the full version (with +commit.xxx) from whichever source has it.
		// Artifact metadata (rawMetadata) has the full version from Solidity; build-info may have short "0.8.28".
		compilerVersion := artifact.EVM.Compiler.Version
		if vi, err := builder.GetVerificationInput(cwd, artifact.Name, artifact.EVM.SourcePath); err == nil && vi.SolcLongVersion != "" {
			// Use build-info if it has full version; else keep artifact's if it has full; else use build-info
			if strings.Contains(vi.SolcLongVersion, "+commit.") {
				compilerVersion = vi.SolcLongVersion
			} else if !strings.Contains(compilerVersion, "+commit.") {
				compilerVersion = vi.SolcLongVersion
			}
		}
		pa.Compiler = &CompilerInfo{
			Version:    compilerVersion,
			EVMVersion: artifact.EVM.Compiler.EVMVersion,
			ViaIR:      artifact.EVM.Compiler.ViaIR,
			Optimizer: &OptimizerInfo{
				Enabled: artifact.EVM.Compiler.Optimizer.Enabled,
				Runs:    artifact.EVM.Compiler.Optimizer.Runs,
			},
		}

		// Prefer per-contract minimal standard JSON (matches bytecode metadata hash); fallback to build-info
		if stdJSON, err := builder.GeneratePerContractStandardJSON(cwd, pkg.Path); err == nil {
			pa.StandardJSONInput = stdJSON
		} else if vi, err := builder.GetVerificationInput(cwd, artifact.Name, artifact.EVM.SourcePath); err == nil {
			fmt.Printf("  Warning: could not generate per-contract standard JSON for %s (%v), using build-info\n", artifact.Name, err)
			pa.StandardJSONInput = vi.StandardJSON
		}

		isDep := !strings.HasPrefix(artifact.EVM.SourcePath, "src/")
		packages = append(packages, packageToPublish{
			name:       pkg.Name,
			artifact:   pa,
			isDep:      isDep,
			sourcePath: artifact.EVM.SourcePath,
		})

		if isDep {
			fmt.Printf("  + %s [dep] -> %s@%s\n", artifact.Name, pkg.Name, version)
		} else {
			fmt.Printf("  + %s -> %s@%s\n", artifact.Name, pkg.Name, version)
		}
	}

	// Resolve project: CLI flag > config
	project := projectFlag
	if project == "" && projectConfig != nil {
		project = projectConfig.Project
	}

	if dryRun {
		fmt.Printf("\nDRY RUN - Would publish %d package(s) to %s\n", len(packages), getServer())
		if project != "" {
			fmt.Printf("  Project: %s\n", project)
		}
		for _, pkg := range packages {
			if pkg.isDep {
				fmt.Printf("   - %s@%s [dependency]\n", pkg.name, version)
			} else {
				fmt.Printf("   - %s@%s\n", pkg.name, version)
			}
		}
		return nil
	}

	// Publish each contract as its own package
	serverURL := getServer()
	fmt.Printf("\nPublishing %d package(s) to %s...\n", len(packages), serverURL)

	var successCount, failCount int
	for _, pkg := range packages {
		err := publishPackage(serverURL, pkg.name, version, project, pkg.artifact, metadata)
		if err != nil {
			fmt.Printf("   X %s@%s: %v\n", pkg.name, version, err)
			failCount++
		} else {
			fmt.Printf("   OK %s@%s\n", pkg.name, version)
			successCount++
		}
	}

	fmt.Println()
	if failCount > 0 {
		return fmt.Errorf("published %d package(s), %d failed", successCount, failCount)
	}

	fmt.Printf("Successfully published %d package(s)\n", successCount)
	if len(packages) > 0 {
		fmt.Printf("\n   Example: contrafactory fetch %s@%s\n", packages[0].name, version)
	}

	return nil
}

// validateDependencies checks that all requested dependencies were found
func validateDependencies(builder *foundry.Builder, cwd string, requestedDeps []string, foundPaths []string) error {
	// Build a set of found contract names
	found := make(map[string]bool)
	for _, path := range foundPaths {
		contractName := strings.TrimSuffix(filepath.Base(path), ".json")
		found[strings.ToLower(contractName)] = true
	}

	// Check which requested deps weren't found
	var unmatched []string
	for _, dep := range requestedDeps {
		if !found[strings.ToLower(dep)] {
			unmatched = append(unmatched, dep)
		}
	}

	if len(unmatched) == 0 {
		return nil
	}

	// Get all available deps for suggestions
	availableDeps, err := builder.DiscoverDependencies(cwd)
	if err != nil {
		// If we can't get available deps, just show the error without suggestions
		return fmt.Errorf("dependency %q not found in build artifacts", unmatched[0])
	}

	// Build error message with suggestions
	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("Error: dependency %q not found in build artifacts", unmatched[0]))
	msg.WriteString("\n\nDid you mean one of these?\n")

	// Find close matches using case-insensitive substring matching
	suggestions := findSuggestions(unmatched[0], availableDeps)
	if len(suggestions) > 0 {
		for _, s := range suggestions {
			msg.WriteString(fmt.Sprintf("  - %s (%s)\n", s.Name, s.SourcePath))
		}
	} else {
		// Show all available deps if no close matches
		for _, dep := range availableDeps {
			if len(suggestions) < 10 {
				msg.WriteString(fmt.Sprintf("  - %s (%s)\n", dep.Name, dep.SourcePath))
			}
		}
	}

	msg.WriteString("\nRun 'contrafactory discover --deps' to see all available dependency contracts.")
	msg.WriteString("\nMake sure the contract has bytecode (interfaces are excluded).")

	return errors.New(msg.String())
}

// findSuggestions finds close matches for a requested dependency name
func findSuggestions(requested string, available []chains.DependencyInfo) []chains.DependencyInfo {
	var suggestions []chains.DependencyInfo
	requestedLower := strings.ToLower(requested)

	for _, dep := range available {
		depLower := strings.ToLower(dep.Name)

		// Check if one contains the other (case-insensitive)
		if strings.Contains(depLower, requestedLower) || strings.Contains(requestedLower, depLower) {
			suggestions = append(suggestions, dep)
			continue
		}

		// Check for common prefix
		minLen := len(depLower)
		if len(requestedLower) < minLen {
			minLen = len(requestedLower)
		}
		if minLen > 3 {
			matchCount := 0
			for i := 0; i < minLen; i++ {
				if depLower[i] == requestedLower[i] {
					matchCount++
				}
			}
			if float64(matchCount)/float64(minLen) > 0.7 {
				suggestions = append(suggestions, dep)
			}
		}
	}

	return suggestions
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
func publishPackage(serverURL, packageName, version, project string, artifact PublishArtifact, metadata map[string]string) error {
	req := PublishRequest{
		Chain:     "evm",
		Builder:   "foundry",
		Project:   project,
		Artifacts: []PublishArtifact{artifact},
		Metadata:  metadata,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/packages/%s/%s", serverURL, url.PathEscape(packageName), url.PathEscape(version))
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
