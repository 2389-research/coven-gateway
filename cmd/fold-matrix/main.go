// ABOUTME: Entry point for fold-matrix bridge
// ABOUTME: Connects Matrix rooms to fold agents via gateway API

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/fatih/color"
)

const banner = `
    ╭──────────────────────────────────╮
    │                                  │
    │   ┏┳┓┏━┓╺┳╸┏━┓╻╻ ╻   ┏┓ ┏━┓╻╺┳┓  │
    │   ┃┃┃┣━┫ ┃ ┣┳┛┃┏╋┛   ┣┻┓┣┳┛┃ ┃┃  │
    │   ╹ ╹╹ ╹ ╹ ╹┗╸╹╹ ╹   ┗━┛╹┗╸╹╺┻┛  │
    │                                  │
    │         fold-matrix bridge       │
    │                                  │
    ╰──────────────────────────────────╯
`

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
	// Print banner
	cyan := color.New(color.FgCyan)
	cyan.Print(banner)

	configPath := getConfigPath()
	dataPath := getDataPath()

	// Ensure data directory exists
	if err := os.MkdirAll(dataPath, 0755); err != nil {
		return fmt.Errorf("creating data directory: %w", err)
	}

	// Load config
	cfg, err := Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config from %s: %w", configPath, err)
	}

	// Setup logger
	logger := setupLogger(cfg.Logging.Level)

	// Print startup info
	green := color.New(color.FgGreen)
	green.Print("    ▶ ")
	fmt.Printf("Config:     %s\n", configPath)
	green.Print("    ▶ ")
	fmt.Printf("Homeserver: %s\n", cfg.Matrix.Homeserver)
	green.Print("    ▶ ")
	fmt.Printf("User:       %s\n", cfg.Matrix.UserID)
	green.Print("    ▶ ")
	fmt.Printf("Gateway:    %s\n", cfg.Gateway.URL)
	if cfg.Matrix.RecoveryKey != "" {
		green.Print("    ▶ ")
		fmt.Println("Encryption: enabled")
	}
	fmt.Println()

	// Create bridge
	bridge, err := NewBridge(cfg, logger)
	if err != nil {
		return fmt.Errorf("creating bridge: %w", err)
	}

	// Setup encryption
	ctx := context.Background()
	cryptoMgr, err := SetupCrypto(ctx, bridge.matrix, cfg.Matrix.UserID, cfg.Matrix.RecoveryKey, dataPath, logger)
	if err != nil {
		return fmt.Errorf("setting up encryption: %w", err)
	}
	if cryptoMgr != nil {
		defer cryptoMgr.Close()
	}

	// Setup graceful shutdown
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Run bridge
	logger.Info("starting bridge")
	return bridge.Run(ctx)
}

func setupLogger(level string) *slog.Logger {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}
