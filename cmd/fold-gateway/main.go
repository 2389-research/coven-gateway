// ABOUTME: Entry point for fold-gateway control server
// ABOUTME: Manages fold agents and frontend connections

package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/2389/fold-gateway/internal/config"
	"github.com/2389/fold-gateway/internal/gateway"
)

// getConfigPath returns the path to the gateway config file.
// Priority: FOLD_CONFIG env var > XDG_CONFIG_HOME/fold/gateway.yaml > ~/.config/fold/gateway.yaml
func getConfigPath() string {
	if envPath := os.Getenv("FOLD_CONFIG"); envPath != "" {
		return envPath
	}

	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "gateway.yaml" // fallback
		}
		configDir = filepath.Join(homeDir, ".config")
	}

	return filepath.Join(configDir, "fold", "gateway.yaml")
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
	if len(os.Args) < 2 {
		fmt.Println("Usage: fold-gateway <command>")
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("  serve     Start the gateway server")
		fmt.Println("  init      Create a new config file interactively")
		fmt.Println("  health    Check gateway health")
		fmt.Println("  agents    List connected agents")
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	var err error
	switch os.Args[1] {
	case "serve":
		err = runServe(ctx)
	case "init":
		err = runInit()
	case "health":
		err = runHealth(ctx)
	case "agents":
		err = runAgents(ctx)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runServe(ctx context.Context) error {
	configPath := getConfigPath()

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Setup logger
	logger := setupLogger(cfg.Logging)

	logger.Info("starting fold-gateway",
		"config", configPath,
		"grpc_addr", cfg.Server.GRPCAddr,
		"http_addr", cfg.Server.HTTPAddr,
	)

	// Create and run gateway
	gw, err := gateway.New(cfg, logger)
	if err != nil {
		return fmt.Errorf("creating gateway: %w", err)
	}

	return gw.Run(ctx)
}

func setupLogger(cfg config.LoggingConfig) *slog.Logger {
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	var handler slog.Handler
	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}

func runHealth(ctx context.Context) error {
	configPath := getConfigPath()

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Make HTTP request to health endpoint with context
	url := fmt.Sprintf("http://%s/health", cfg.Server.HTTPAddr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unhealthy: status %d", resp.StatusCode)
	}

	fmt.Println("healthy")
	return nil
}

func runAgents(ctx context.Context) error {
	configPath := getConfigPath()

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Make HTTP request to ready endpoint with context
	url := fmt.Sprintf("http://%s/health/ready", cfg.Server.HTTPAddr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("agents check failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	fmt.Println(string(body))
	return nil
}

func runInit() error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("fold-gateway configuration setup")
	fmt.Println("=================================")
	fmt.Println()

	// Default paths
	defaultConfigPath := getConfigPath()
	defaultDataPath := getDataPath()
	defaultDbPath := filepath.Join(defaultDataPath, "gateway.db")

	// Output filename
	outputFile := prompt(reader, "Config file path", defaultConfigPath)

	// Check if file exists
	if _, err := os.Stat(outputFile); err == nil {
		overwrite := prompt(reader, "File exists. Overwrite?", "no")
		if strings.ToLower(overwrite) != "yes" && strings.ToLower(overwrite) != "y" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Server configuration
	fmt.Println("\n--- Server Configuration ---")
	grpcAddr := prompt(reader, "gRPC address", "localhost:50051")
	httpAddr := prompt(reader, "HTTP address", "localhost:8080")

	// Database
	fmt.Println("\n--- Database Configuration ---")
	dbPath := prompt(reader, "SQLite database path", defaultDbPath)

	// Tailscale
	fmt.Println("\n--- Tailscale Configuration ---")
	enableTailscale := prompt(reader, "Enable Tailscale?", "no")
	tailscaleEnabled := strings.ToLower(enableTailscale) == "yes" || strings.ToLower(enableTailscale) == "y"

	var tsHostname, tsAuthKey string
	var tsEphemeral, tsFunnel bool
	if tailscaleEnabled {
		tsHostname = prompt(reader, "Tailscale hostname", "fold-gateway")
		tsAuthKey = prompt(reader, "Tailscale auth key (leave empty for interactive)", "")
		ephemeralStr := prompt(reader, "Ephemeral node?", "no")
		tsEphemeral = strings.ToLower(ephemeralStr) == "yes" || strings.ToLower(ephemeralStr) == "y"
		funnelStr := prompt(reader, "Enable Funnel (public HTTPS)?", "no")
		tsFunnel = strings.ToLower(funnelStr) == "yes" || strings.ToLower(funnelStr) == "y"
	}

	// Logging
	fmt.Println("\n--- Logging Configuration ---")
	logLevel := prompt(reader, "Log level (debug/info/warn/error)", "info")
	logFormat := prompt(reader, "Log format (text/json)", "text")

	// Generate config
	var cfg strings.Builder
	cfg.WriteString("# fold-gateway configuration\n")
	cfg.WriteString("# Generated by fold-gateway init\n\n")

	cfg.WriteString("server:\n")
	cfg.WriteString(fmt.Sprintf("  grpc_addr: \"%s\"\n", grpcAddr))
	cfg.WriteString(fmt.Sprintf("  http_addr: \"%s\"\n", httpAddr))
	cfg.WriteString("\n")

	cfg.WriteString("database:\n")
	cfg.WriteString(fmt.Sprintf("  path: \"%s\"\n", dbPath))
	cfg.WriteString("\n")

	cfg.WriteString("tailscale:\n")
	cfg.WriteString(fmt.Sprintf("  enabled: %t\n", tailscaleEnabled))
	if tailscaleEnabled {
		cfg.WriteString(fmt.Sprintf("  hostname: \"%s\"\n", tsHostname))
		if tsAuthKey != "" {
			cfg.WriteString(fmt.Sprintf("  auth_key: \"%s\"\n", tsAuthKey))
		}
		cfg.WriteString(fmt.Sprintf("  ephemeral: %t\n", tsEphemeral))
		cfg.WriteString(fmt.Sprintf("  funnel: %t\n", tsFunnel))
	}
	cfg.WriteString("\n")

	cfg.WriteString("agents:\n")
	cfg.WriteString("  heartbeat_interval: \"30s\"\n")
	cfg.WriteString("  heartbeat_timeout: \"90s\"\n")
	cfg.WriteString("  reconnect_grace_period: \"5m\"\n")
	cfg.WriteString("\n")

	cfg.WriteString("logging:\n")
	cfg.WriteString(fmt.Sprintf("  level: \"%s\"\n", logLevel))
	cfg.WriteString(fmt.Sprintf("  format: \"%s\"\n", logFormat))
	cfg.WriteString("\n")

	cfg.WriteString("metrics:\n")
	cfg.WriteString("  enabled: false\n")
	cfg.WriteString("  path: \"/metrics\"\n")

	// Ensure config directory exists
	configDir := filepath.Dir(outputFile)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	// Write config file
	if err := os.WriteFile(outputFile, []byte(cfg.String()), 0644); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	// Ensure data directory exists
	dataDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("creating data directory: %w", err)
	}

	fmt.Printf("\nConfig written to %s\n", outputFile)
	fmt.Printf("Data directory: %s\n", dataDir)
	fmt.Println("\nTo start the server:")
	fmt.Printf("  fold-gateway serve\n")

	return nil
}

func prompt(reader *bufio.Reader, question, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", question, defaultVal)
	} else {
		fmt.Printf("%s: ", question)
	}

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return defaultVal
	}
	return input
}
