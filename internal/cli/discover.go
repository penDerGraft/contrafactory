package cli

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/pendergraft/contrafactory/internal/chains"
	"github.com/pendergraft/contrafactory/internal/chains/evm/foundry"
)

func createDiscoverCmd() *cobra.Command {
	var showDeps bool
	var showAll bool

	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Discover contracts in the project",
		Long: `Discover contracts that would be published or are available as dependencies.

This command examines your Foundry build artifacts to show:
- Contracts from src/ that would be published (default)
- Dependency contracts from lib/ that could be included (--deps)
- Both (--all)

EXAMPLES:
  # Show contracts that would be published
  contrafactory discover

  # Show available dependency contracts from lib/
  contrafactory discover --deps

  # Show everything
  contrafactory discover --all
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDiscover(showDeps, showAll)
		},
	}

	cmd.Flags().BoolVar(&showDeps, "deps", false, "show dependency contracts from lib/")
	cmd.Flags().BoolVar(&showAll, "all", false, "show both src and dependency contracts")

	return cmd
}

func runDiscover(showDeps, showAll bool) error {
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
		return fmt.Errorf("no Foundry project detected (missing foundry.toml)")
	}

	// Determine what to show
	showSrc := !showDeps || showAll
	showLib := showDeps || showAll

	// Load project config for exclude patterns
	projectConfig := loadProjectConfigSilent()
	excludePatterns := defaultExcludePatterns
	if projectConfig != nil && len(projectConfig.Exclude) > 0 {
		excludePatterns = projectConfig.Exclude
	}

	// Discover src contracts
	if showSrc {
		discoverOpts := chains.DiscoverOptions{
			Exclude: excludePatterns,
		}

		artifactPaths, err := builder.Discover(cwd, discoverOpts)
		if err != nil {
			if strings.Contains(err.Error(), "build-info") {
				return fmt.Errorf("%w\n\nTIP: Run 'forge build --build-info' first", err)
			}
			return fmt.Errorf("discovering contracts: %w", err)
		}

		if len(artifactPaths) == 0 {
			fmt.Println("No contracts found in src/")
			fmt.Println("\nMake sure you've run 'forge build' and have contracts in your src/ directory.")
		} else {
			fmt.Printf("Contracts in src/ (%d):\n\n", len(artifactPaths))

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			for _, path := range artifactPaths {
				artifact, err := builder.Parse(path)
				if err != nil {
					continue
				}
				if artifact.EVM != nil {
					fmt.Fprintf(w, "  %s\t%s\n", artifact.Name, artifact.EVM.SourcePath)
				}
			}
			w.Flush()
		}
	}

	// Discover lib dependencies
	if showLib {
		if showSrc {
			fmt.Println()
		}

		deps, err := builder.DiscoverDependencies(cwd)
		if err != nil {
			if strings.Contains(err.Error(), "out directory not found") {
				return fmt.Errorf("out directory not found - run 'forge build' first")
			}
			return fmt.Errorf("discovering dependencies: %w", err)
		}

		if len(deps) == 0 {
			fmt.Println("No dependency contracts found in lib/")
			fmt.Println("\nDependencies are contracts from lib/ that have bytecode.")
			fmt.Println("Run 'forge install' to add dependencies to your project.")
		} else {
			fmt.Printf("Available dependency contracts from lib/ (%d):\n\n", len(deps))

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "  NAME\tSOURCE\n")
			for _, dep := range deps {
				fmt.Fprintf(w, "  %s\t%s\n", dep.Name, dep.SourcePath)
			}
			w.Flush()

			fmt.Println()
			fmt.Println("Tip: add names to include_dependencies in contrafactory.toml to publish them")
		}
	}

	return nil
}
