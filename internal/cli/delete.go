package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/spf13/cobra"
)

func createDeleteCmd() *cobra.Command {
	var version string
	var contracts []string
	var exclude []string
	var excludePaths []string
	var includeDeps []string
	var prefix string
	var project string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete packages from the registry",
		Long: `Delete packages from the Contrafactory registry using the same discovery logic as publish.

Discovers packages from your Foundry project (contracts, exclude, include_dependencies
from contrafactory.toml) and deletes each package at the specified version.

EXAMPLES:
  # Delete all packages for version 1.0.0 (same set that would be published)
  contrafactory delete --version 1.0.0

  # Delete with prefix (must match what was used when publishing)
  contrafactory delete --version 1.0.0 --prefix myproject

  # Dry run (show what would be deleted)
  contrafactory delete --version 1.0.0 --dry-run
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDelete(version, prefix, project, contracts, exclude, excludePaths, includeDeps, dryRun)
		},
	}

	cmd.Flags().StringVarP(&version, "version", "v", "", "version to delete (required)")
	cmd.Flags().StringSliceVar(&contracts, "contracts", nil, "specific contracts to delete (default: all from config)")
	cmd.Flags().StringSliceVar(&exclude, "exclude", nil, "patterns to exclude by contract name (e.g., Test,Mock)")
	cmd.Flags().StringSliceVar(&excludePaths, "exclude-path", nil, "patterns to exclude by source path (e.g., proxy, examples/MetaCoin.sol)")
	cmd.Flags().StringSliceVar(&includeDeps, "include-deps", nil, "dependency contracts to include")
	cmd.Flags().StringVarP(&prefix, "prefix", "p", "", "prefix for package names (must match publish)")
	cmd.Flags().StringVar(&project, "project", "", "project scope (overrides contrafactory.toml)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be deleted without deleting")
	_ = cmd.MarkFlagRequired("version")

	return cmd
}

func runDelete(version, prefix, projectFlag string, contracts, exclude, excludePaths, includeDeps []string, dryRun bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	// Load project config and resolve params (same as publish)
	projectConfig := loadProjectConfigSilent()

	if len(contracts) == 0 && projectConfig != nil {
		contracts = projectConfig.Contracts
	}

	excludePatterns := defaultExcludePatterns
	if len(exclude) > 0 {
		excludePatterns = exclude
	} else if projectConfig != nil && len(projectConfig.Exclude) > 0 {
		excludePatterns = projectConfig.Exclude
	}

	excludePathPatterns := excludePaths
	if len(excludePathPatterns) == 0 && projectConfig != nil {
		excludePathPatterns = projectConfig.ExcludePaths
	}

	if len(includeDeps) == 0 && projectConfig != nil {
		includeDeps = projectConfig.IncludeDependencies
	}

	// Discover packages (same logic as publish)
	discovered, err := discoverPackages(cwd, prefix, contracts, excludePatterns, excludePathPatterns, includeDeps)
	if err != nil {
		return err
	}

	serverURL := getServer()

	// Resolve project: CLI flag > config (display-only for now; delete API uses name+version only)
	project := projectFlag
	if project == "" && projectConfig != nil {
		project = projectConfig.Project
	}

	if dryRun {
		fmt.Printf("DRY RUN - Would delete %d package(s) from %s\n", len(discovered), serverURL)
		if project != "" {
			fmt.Printf("  Project scope: %s\n", project)
		}
		for _, pkg := range discovered {
			fmt.Printf("   - %s@%s\n", pkg.Name, version)
		}
		return nil
	}

	apiKey := getAPIKey()
	if apiKey == "" {
		return fmt.Errorf("API key required for delete (use --api-key, CONTRAFACTORY_API_KEY, or contrafactory auth login)")
	}

	fmt.Printf("Deleting %d package(s) from %s...\n", len(discovered), serverURL)

	var successCount, failCount int
	for _, pkg := range discovered {
		err := deletePackage(serverURL, apiKey, pkg.Name, version)
		if err != nil {
			fmt.Printf("   X %s@%s: %v\n", pkg.Name, version, err)
			failCount++
		} else {
			fmt.Printf("   OK %s@%s\n", pkg.Name, version)
			successCount++
		}
	}

	fmt.Println()
	if failCount > 0 {
		return fmt.Errorf("deleted %d package(s), %d failed", successCount, failCount)
	}

	fmt.Printf("Successfully deleted %d package(s)\n", successCount)
	return nil
}

func deletePackage(serverURL, apiKey, packageName, version string) error {
	path := fmt.Sprintf("%s/api/v1/packages/%s/%s", serverURL, url.PathEscape(packageName), url.PathEscape(version))
	req, err := http.NewRequest("DELETE", path, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("X-API-Key", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusNoContent {
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
