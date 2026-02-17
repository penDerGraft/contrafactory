package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// projectConfigFiles is the search order for project config files
var projectConfigFiles = []string{"contrafactory.toml", "cf.toml"}

// ProjectConfig is the project-level TOML configuration
type ProjectConfig struct {
	Server              string        `toml:"server"`
	Project             string        `toml:"project,omitempty"`
	Chain               string        `toml:"chain,omitempty"`
	Builder             string        `toml:"builder,omitempty"`
	Contracts           []string      `toml:"contracts,omitempty"`
	Exclude             []string      `toml:"exclude,omitempty"`
	ExcludePaths        []string      `toml:"exclude_paths,omitempty"`
	IncludeDependencies []string      `toml:"include_dependencies,omitempty"`
	EVM                 EVMConfigTOML `toml:"evm,omitempty"`
}

// EVMConfigTOML contains EVM-specific configuration for project config
type EVMConfigTOML struct {
	Foundry FoundryConfigTOML `toml:"foundry,omitempty"`
}

// FoundryConfigTOML contains Foundry-specific configuration for project config
type FoundryConfigTOML struct {
	ArtifactsDir string `toml:"artifacts_dir,omitempty"`
}

// ServerConfig is the global server configuration (stored in ~/.contrafactory/config.yaml)
type ServerConfig struct {
	Server string `yaml:"server"`
}

func createConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Configuration commands",
	}

	cmd.AddCommand(createConfigInitCmd())
	cmd.AddCommand(createConfigShowCmd())

	return cmd
}

func createConfigInitCmd() *cobra.Command {
	var serverURL string
	var project string
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create config file",
		Long: `Create a contrafactory.toml configuration file in the current directory.

This file stores project-specific settings like the server URL,
project name, and which contracts to include/exclude.

EXAMPLES:
  # Create config with default server
  contrafactory config init

  # Create config for a specific server
  contrafactory config init --server https://contrafactory.example.com

  # Overwrite existing config
  contrafactory config init --force
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigInit(serverURL, project, force)
		},
	}

	cmd.Flags().StringVar(&serverURL, "server", "http://localhost:8080", "server URL")
	cmd.Flags().StringVar(&project, "project", "", "project name (defaults to directory name)")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing config")

	return cmd
}

func createConfigShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Display current config",
		Long: `Display the current configuration.

Shows both the local project config (contrafactory.toml) and the global config from ~/.contrafactory/config.yaml.

EXAMPLES:
  contrafactory config show
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigShow()
		},
	}

	return cmd
}

func runConfigInit(serverURL, project string, force bool) error {
	configPath := "contrafactory.toml"

	// Check if any config file already exists
	for _, cfgFile := range projectConfigFiles {
		if _, err := os.Stat(cfgFile); err == nil && !force {
			return fmt.Errorf("config file already exists at %s (use --force to overwrite)", cfgFile)
		}
	}

	// Default project name to current directory
	if project == "" {
		cwd, err := os.Getwd()
		if err == nil {
			project = filepath.Base(cwd)
		}
	}

	// Generate TOML config
	content := fmt.Sprintf(`# Contrafactory project configuration
# See https://github.com/pendergraft/contrafactory for documentation

server = "%s"
project = "%s"
chain = "evm"

# Patterns to exclude from publishing
exclude = ["Test", "Script", "Mock", "Deploy", "Setup"]

# Exclude by source path (substring or glob, e.g. "proxy" or "examples/MetaCoin.sol")
# exclude_paths = ["proxy", "examples/MetaCoin.sol"]

# Specific contracts to publish (empty = all from src/)
# contracts = ["MyContract", "OtherContract"]

# Third-party contracts to publish as separate packages
# Useful for proxy patterns that need companion contracts
# include_dependencies = ["TransparentUpgradeableProxy", "ProxyAdmin"]
`, serverURL, project)

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	fmt.Printf("Created %s\n", configPath)
	fmt.Println()
	fmt.Println("Configuration:")
	fmt.Printf("  Server:  %s\n", serverURL)
	fmt.Printf("  Project: %s\n", project)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  1. Edit %s to customize settings\n", configPath)
	fmt.Println("  2. Run 'contrafactory auth login' to authenticate")
	fmt.Println("  3. Run 'contrafactory publish --version 1.0.0' to publish")

	return nil
}

func runConfigShow() error {
	fmt.Println("Configuration sources (in order of precedence):")
	fmt.Println()

	// 1. Command line flags
	fmt.Println("1. Command line flags")
	fmt.Println("   --server, --api-key, --config")
	fmt.Println()

	// 2. Environment variables
	fmt.Println("2. Environment variables")
	serverEnv := os.Getenv("CONTRAFACTORY_SERVER")
	keyEnv := os.Getenv("CONTRAFACTORY_API_KEY")
	if serverEnv != "" {
		fmt.Printf("   CONTRAFACTORY_SERVER=%s\n", serverEnv)
	} else {
		fmt.Println("   CONTRAFACTORY_SERVER=(not set)")
	}
	if keyEnv != "" {
		fmt.Printf("   CONTRAFACTORY_API_KEY=%s\n", maskAPIKey(keyEnv))
	} else {
		fmt.Println("   CONTRAFACTORY_API_KEY=(not set)")
	}
	fmt.Println()

	// 3. Local project config
	fmt.Println("3. Local project config (contrafactory.toml or cf.toml)")
	projectConfig, configPath, err := loadProjectConfig()
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("   (not found)")
		} else {
			fmt.Printf("   Error: %v\n", err)
		}
	} else {
		fmt.Printf("   Loaded from: %s\n", configPath)
		if projectConfig.Server != "" {
			fmt.Printf("   server: %s\n", projectConfig.Server)
		}
		if projectConfig.Project != "" {
			fmt.Printf("   project: %s\n", projectConfig.Project)
		}
		if projectConfig.Chain != "" {
			fmt.Printf("   chain: %s\n", projectConfig.Chain)
		}
		if len(projectConfig.Contracts) > 0 {
			fmt.Printf("   contracts: %v\n", projectConfig.Contracts)
		}
		if len(projectConfig.Exclude) > 0 {
			fmt.Printf("   exclude: %v\n", projectConfig.Exclude)
		}
		if len(projectConfig.ExcludePaths) > 0 {
			fmt.Printf("   exclude_paths: %v\n", projectConfig.ExcludePaths)
		}
		if len(projectConfig.IncludeDependencies) > 0 {
			fmt.Printf("   include_dependencies: %v\n", projectConfig.IncludeDependencies)
		}
	}
	fmt.Println()

	// 4. Global config
	fmt.Println("4. Global config (~/.contrafactory/config.yaml)")
	globalPath := filepath.Join(credentialsDir(), "config.yaml")
	globalData, err := os.ReadFile(globalPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("   (not found)")
		} else {
			fmt.Printf("   Error: %v\n", err)
		}
	} else {
		var globalConfig ServerConfig
		if err := yaml.Unmarshal(globalData, &globalConfig); err == nil {
			if globalConfig.Server != "" {
				fmt.Printf("   server: %s\n", globalConfig.Server)
			}
		}
	}
	fmt.Println()

	// 5. Credentials
	fmt.Println("5. Credentials (~/.contrafactory/credentials)")
	creds, err := loadCredentials()
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("   (not found)")
		} else {
			fmt.Printf("   Error: %v\n", err)
		}
	} else {
		if len(creds.Servers) == 0 {
			fmt.Println("   (no credentials stored)")
		} else {
			for server, cred := range creds.Servers {
				fmt.Printf("   %s: %s\n", server, maskAPIKey(cred.APIKey))
			}
		}
	}
	fmt.Println()

	// Effective config
	fmt.Println("Effective configuration:")
	fmt.Printf("   Server:  %s\n", getServer())
	if key := getAPIKey(); key != "" {
		fmt.Printf("   API Key: %s\n", maskAPIKey(key))
	} else {
		fmt.Println("   API Key: (not set)")
	}

	return nil
}

// loadProjectConfig loads the project config from the first matching config file.
// Returns the config, the path it was loaded from, and an error.
func loadProjectConfig() (*ProjectConfig, string, error) {
	// If --config flag was provided, use that directly
	if cfgFile != "" {
		config, err := loadProjectConfigFromPath(cfgFile)
		if err != nil {
			return nil, cfgFile, err
		}
		return config, cfgFile, nil
	}

	// Search for config files in order
	for _, name := range projectConfigFiles {
		if _, err := os.Stat(name); err == nil {
			config, err := loadProjectConfigFromPath(name)
			if err != nil {
				return nil, name, err
			}
			return config, name, nil
		}
	}
	return nil, "", os.ErrNotExist
}

// loadProjectConfigFromPath loads a project config from a specific path
func loadProjectConfigFromPath(path string) (*ProjectConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config ProjectConfig
	if _, err := toml.Decode(string(data), &config); err != nil {
		return nil, fmt.Errorf("parsing TOML: %w", err)
	}

	return &config, nil
}

// loadProjectConfigSilent loads the project config without returning errors for missing files.
// Returns nil if the file doesn't exist, but returns errors for parse failures.
func loadProjectConfigSilent() *ProjectConfig {
	config, _, err := loadProjectConfig()
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		// Show actionable errors (parse failures)
		fmt.Fprintf(os.Stderr, "Warning: failed to load project config: %v\n", err)
		return nil
	}
	return config
}
