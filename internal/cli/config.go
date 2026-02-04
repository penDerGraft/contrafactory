package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// CLIConfig is the CLI configuration file structure
type CLIConfig struct {
	Server    string    `yaml:"server"`
	Project   string    `yaml:"project,omitempty"`
	Chain     string    `yaml:"chain,omitempty"`
	Builder   string    `yaml:"builder,omitempty"`
	EVM       EVMConfig `yaml:"evm,omitempty"`
	Contracts []string  `yaml:"contracts,omitempty"`
	Exclude   []string  `yaml:"exclude,omitempty"`
}

// EVMConfig contains EVM-specific configuration
type EVMConfig struct {
	Foundry FoundryConfig `yaml:"foundry,omitempty"`
	Hardhat HardhatConfig `yaml:"hardhat,omitempty"`
}

// FoundryConfig contains Foundry-specific configuration
type FoundryConfig struct {
	ArtifactsDir string `yaml:"artifacts_dir,omitempty"`
}

// HardhatConfig contains Hardhat-specific configuration
type HardhatConfig struct {
	ArtifactsDir string `yaml:"artifacts_dir,omitempty"`
	BuildInfoDir string `yaml:"build_info_dir,omitempty"`
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
		Long: `Create a contrafactory.yaml configuration file in the current directory.

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
		Long: `Display the current configuration from contrafactory.yaml.

Shows both the local project config and the global config from ~/.contrafactory/config.yaml.

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
	configPath := "contrafactory.yaml"

	// Check if config already exists
	if _, err := os.Stat(configPath); err == nil && !force {
		return fmt.Errorf("config file already exists at %s (use --force to overwrite)", configPath)
	}

	// Default project name to current directory
	if project == "" {
		cwd, err := os.Getwd()
		if err == nil {
			project = filepath.Base(cwd)
		}
	}

	config := CLIConfig{
		Server:  serverURL,
		Project: project,
		Chain:   "evm",
		Exclude: []string{"Test*", "Mock*", "Script*"},
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Add helpful comments
	content := fmt.Sprintf(`# Contrafactory CLI configuration
# See https://github.com/pendergraft/contrafactory for documentation

%s
# Optional: specify which contracts to include
# contracts:
#   - MyContract
#   - OtherContract
`, string(data))

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	fmt.Printf("✅ Created %s\n", configPath)
	fmt.Println()
	fmt.Println("Configuration:")
	fmt.Printf("  Server:  %s\n", serverURL)
	fmt.Printf("  Project: %s\n", project)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Edit contrafactory.yaml to customize settings")
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
	fmt.Println("3. Local project config (./contrafactory.yaml)")
	localConfig, err := loadLocalConfig()
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("   (not found)")
		} else {
			fmt.Printf("   Error: %v\n", err)
		}
	} else {
		if localConfig.Server != "" {
			fmt.Printf("   server: %s\n", localConfig.Server)
		}
		if localConfig.Project != "" {
			fmt.Printf("   project: %s\n", localConfig.Project)
		}
		if localConfig.Chain != "" {
			fmt.Printf("   chain: %s\n", localConfig.Chain)
		}
		if len(localConfig.Contracts) > 0 {
			fmt.Printf("   contracts: %v\n", localConfig.Contracts)
		}
		if len(localConfig.Exclude) > 0 {
			fmt.Printf("   exclude: %v\n", localConfig.Exclude)
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
		var globalConfig CLIConfig
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

func loadLocalConfig() (*CLIConfig, error) {
	data, err := os.ReadFile("contrafactory.yaml")
	if err != nil {
		return nil, err
	}

	var config CLIConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}
