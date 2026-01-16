// ABOUTME: Entry point for fold-matrix bridge
// ABOUTME: Connects Matrix rooms to fold agents via gateway API

package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/fatih/color"
)

const banner = `
  __       _     _                       _        _
 / _| ___ | | __| |      _ __ ___   __ _| |_ _ __(_)_  __
| |_ / _ \| |/ _' |_____| '_ ' _ \ / _' | __| '__| \ \/ /
|  _| (_) | | (_| |_____| | | | | | (_| | |_| |  | |>  <
|_|  \___/|_|\__,_|     |_| |_| |_|\__,_|\__|_|  |_/_/\_\
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
	// Check for init command
	if len(os.Args) > 1 && os.Args[1] == "init" {
		if err := runInit(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

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

	// Setup graceful shutdown context first - all operations should respect it
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Create bridge
	bridge, err := NewBridge(cfg, logger)
	if err != nil {
		return fmt.Errorf("creating bridge: %w", err)
	}

	// Setup encryption
	cryptoMgr, err := SetupCrypto(ctx, bridge.matrix, cfg.Matrix.UserID, cfg.Matrix.RecoveryKey, dataPath, logger)
	if err != nil {
		return fmt.Errorf("setting up encryption: %w", err)
	}
	if cryptoMgr != nil {
		defer cryptoMgr.Close()
	}

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

func runInit() error {
	cyan := color.New(color.FgCyan)
	green := color.New(color.FgGreen)
	yellow := color.New(color.FgYellow)

	cyan.Print(banner)
	fmt.Println("    Interactive Setup")
	fmt.Println("    -----------------")
	fmt.Println()

	configPath := getConfigPath()

	// Check if config already exists
	if _, err := os.Stat(configPath); err == nil {
		yellow.Printf("    Config already exists at %s\n", configPath)
		fmt.Print("    Overwrite? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(answer)) != "y" {
			fmt.Println("    Aborted.")
			return nil
		}
		fmt.Println()
	}

	reader := bufio.NewReader(os.Stdin)

	// Gather config values
	green.Print("    ▶ ")
	fmt.Print("Matrix homeserver URL [https://matrix.org]: ")
	homeserver, _ := reader.ReadString('\n')
	homeserver = strings.TrimSpace(homeserver)
	if homeserver == "" {
		homeserver = "https://matrix.org"
	}

	green.Print("    ▶ ")
	fmt.Print("Matrix user ID [@bot:matrix.org]: ")
	userID, _ := reader.ReadString('\n')
	userID = strings.TrimSpace(userID)
	if userID == "" {
		userID = "@bot:matrix.org"
	}

	green.Print("    ▶ ")
	fmt.Print("Matrix access token: ")
	accessToken, _ := reader.ReadString('\n')
	accessToken = strings.TrimSpace(accessToken)

	green.Print("    ▶ ")
	fmt.Print("Matrix recovery key (optional, for E2EE): ")
	recoveryKey, _ := reader.ReadString('\n')
	recoveryKey = strings.TrimSpace(recoveryKey)

	green.Print("    ▶ ")
	fmt.Print("Gateway URL [http://localhost:8080]: ")
	gatewayURL, _ := reader.ReadString('\n')
	gatewayURL = strings.TrimSpace(gatewayURL)
	if gatewayURL == "" {
		gatewayURL = "http://localhost:8080"
	}

	green.Print("    ▶ ")
	fmt.Print("Command prefix (optional, e.g. '!fold '): ")
	prefix, _ := reader.ReadString('\n')
	prefix = strings.TrimSpace(prefix)

	// Generate config
	config := fmt.Sprintf(`# fold-matrix bridge configuration
# Generated by fold-matrix init

[matrix]
homeserver = "%s"
user_id = "%s"
access_token = "%s"
`, homeserver, userID, accessToken)

	if recoveryKey != "" {
		config += fmt.Sprintf("recovery_key = \"%s\"\n", recoveryKey)
	}

	config += fmt.Sprintf(`
[gateway]
url = "%s"

[bridge]
# Only respond in these rooms (empty = all joined rooms)
allowed_rooms = []
# Require messages start with this prefix (empty = respond to all)
command_prefix = "%s"
# Send typing indicator while streaming
typing_indicator = true

[logging]
level = "info"
`, gatewayURL, prefix)

	// Ensure config directory exists
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	// Write config file
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	fmt.Println()
	green.Printf("    ✓ Config written to %s\n", configPath)
	fmt.Println()
	fmt.Println("    Next steps:")
	fmt.Println("    1. Get an access token from your Matrix account")
	fmt.Println("       (Settings → Help & About → Advanced → Access Token)")
	fmt.Println("    2. Update the config with your real credentials")
	fmt.Println("    3. Run: fold-matrix")
	fmt.Println()

	return nil
}
