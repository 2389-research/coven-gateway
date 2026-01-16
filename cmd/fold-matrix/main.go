// ABOUTME: Entry point for fold-matrix bridge
// ABOUTME: Connects Matrix rooms to fold agents via gateway API

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// getConfigPath returns the path to the matrix bridge config file.
// Priority: FOLD_MATRIX_CONFIG env var > XDG_CONFIG_HOME/fold/matrix-bridge.toml > ~/.config/fold/matrix-bridge.toml
func getConfigPath() string {
	if envPath := os.Getenv("FOLD_MATRIX_CONFIG"); envPath != "" {
		return envPath
	}

	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "matrix-bridge.toml" // fallback
		}
		configDir = filepath.Join(homeDir, ".config")
	}

	return filepath.Join(configDir, "fold", "matrix-bridge.toml")
}

// getDataPath returns the path to the fold data directory.
// Priority: XDG_DATA_HOME/fold > ~/.local/share/fold
func getDataPath() string {
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "data" // fallback
		}
		dataDir = filepath.Join(homeDir, ".local", "share")
	}

	return filepath.Join(dataDir, "fold")
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := getConfigPath()

	cfg, err := Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config from %s: %w", configPath, err)
	}

	fmt.Printf("Config loaded from %s\n", configPath)
	fmt.Printf("Homeserver: %s\n", cfg.Matrix.Homeserver)
	fmt.Printf("User ID: %s\n", cfg.Matrix.UserID)
	fmt.Printf("Gateway URL: %s\n", cfg.Gateway.URL)

	// TODO: Setup logger, create bridge, run

	return nil
}
