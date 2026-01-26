// ABOUTME: Entry point for coven-gateway control server
// ABOUTME: Manages coven agents and frontend connections

package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/google/uuid"

	"github.com/2389/coven-gateway/internal/auth"
	"github.com/2389/coven-gateway/internal/config"
	"github.com/2389/coven-gateway/internal/gateway"
	"github.com/2389/coven-gateway/internal/store"
)

// Version is set by goreleaser at build time.
var version = "dev"

const banner = `
                                            _
  ___ _____   _____ _ __        __ _  __ _| |_ _____      ____ _ _   _
 / __/ _ \ \ / / _ \ '_ \ _____/ _' |/ _' | __/ _ \ \ /\ / / _' | | | |
| (_| (_) \ V /  __/ | | |_____| (_| | (_| | ||  __/\ V  V / (_| | |_| |
 \___\___/ \_/ \___|_| |_|      \__, |\__,_|\__\___| \_/\_/ \__,_|\__, |
                                |___/                             |___/
`

// getConfigPath returns the path to the gateway config file.
// Priority: COVEN_CONFIG env var > XDG_CONFIG_HOME/coven/gateway.yaml > ~/.config/coven/gateway.yaml
func getConfigPath() string {
	if envPath := os.Getenv("COVEN_CONFIG"); envPath != "" {
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

	return filepath.Join(configDir, "coven", "gateway.yaml")
}

// getDataPath returns the path to the coven data directory.
// Priority: XDG_DATA_HOME/coven > ~/.local/share/coven
func getDataPath() string {
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "data" // fallback
		}
		dataDir = filepath.Join(homeDir, ".local", "share")
	}

	return filepath.Join(dataDir, "coven")
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: coven-gateway <command>")
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("  serve                  Start the gateway server")
		fmt.Println("  init                   Create a new config file interactively")
		fmt.Println("  bootstrap --name NAME  Create initial owner principal and token")
		fmt.Println("  health                 Check gateway health")
		fmt.Println("  agents                 List connected agents")
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
	case "bootstrap":
		err = runBootstrap(ctx)
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

	// Print banner
	cyan := color.New(color.FgCyan)
	cyan.Print(banner)

	// Version info
	gray := color.New(color.FgHiBlack)
	gray.Printf("    version: %s\n\n", version)

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Setup logger
	logger := setupLogger(cfg.Logging)

	// Startup info
	green := color.New(color.FgGreen)
	yellow := color.New(color.FgYellow)

	green.Print("    ▶ ")
	fmt.Printf("Config:    %s\n", configPath)
	green.Print("    ▶ ")
	fmt.Printf("gRPC:      %s\n", cfg.Server.GRPCAddr)
	green.Print("    ▶ ")
	fmt.Printf("HTTP:      %s\n", cfg.Server.HTTPAddr)

	// Tailscale status
	if cfg.Tailscale.Enabled {
		green.Print("    ▶ ")
		fmt.Printf("Tailscale: ")
		cyan.Print(cfg.Tailscale.Hostname)
		if cfg.Tailscale.Funnel {
			yellow.Print(" [funnel]")
		}
		if cfg.Tailscale.Ephemeral {
			gray.Print(" (ephemeral)")
		}
		fmt.Println()
	}

	fmt.Println()

	logger.Info("starting coven-gateway",
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
		handler = &colorHandler{
			level: level,
		}
	}

	return slog.New(handler)
}

// colorHandler provides colorized log output with thread-safe writes.
type colorHandler struct {
	mu     sync.Mutex
	level  slog.Level
	attrs  []slog.Attr
	groups []string
}

func (h *colorHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *colorHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	var buf strings.Builder

	// Format timestamp
	buf.WriteString(color.HiBlackString(r.Time.Format("15:04:05") + " "))

	// Colorize level
	switch r.Level {
	case slog.LevelDebug:
		buf.WriteString(color.MagentaString("DBG "))
	case slog.LevelInfo:
		buf.WriteString(color.CyanString("INF "))
	case slog.LevelWarn:
		buf.WriteString(color.YellowString("WRN "))
	case slog.LevelError:
		buf.WriteString(color.New(color.FgRed, color.Bold).Sprint("ERR "))
	default:
		buf.WriteString("??? ")
	}

	// Print message
	buf.WriteString(r.Message)

	// Print handler-level attrs first (from WithAttrs)
	for _, a := range h.attrs {
		buf.WriteString(color.HiBlackString(" " + a.Key + "="))
		buf.WriteString(a.Value.String())
	}

	// Print record attrs
	r.Attrs(func(a slog.Attr) bool {
		buf.WriteString(color.HiBlackString(" " + a.Key + "="))
		buf.WriteString(a.Value.String())
		return true
	})

	buf.WriteString("\n")
	fmt.Print(buf.String())
	return nil
}

func (h *colorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs), len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	newAttrs = append(newAttrs, attrs...)
	return &colorHandler{
		level:  h.level,
		attrs:  newAttrs,
		groups: h.groups,
	}
}

func (h *colorHandler) WithGroup(name string) slog.Handler {
	newGroups := make([]string, len(h.groups), len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups = append(newGroups, name)
	return &colorHandler{
		level:  h.level,
		attrs:  h.attrs,
		groups: newGroups,
	}
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

// runBootstrap performs first-time setup of the gateway:
// 1. Creates config file with random JWT secret (if not exists)
// 2. Creates database and owner principal
// 3. Generates JWT token for the owner
//
// This is a one-command setup: coven-gateway bootstrap --name "Your Name"
func runBootstrap(ctx context.Context) error {
	// Parse args with explicit error handling
	// Supports both "--name value" and "--name=value" formats
	var displayName string
	args := os.Args[2:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--name" || arg == "-n":
			if i+1 >= len(args) {
				return fmt.Errorf("--name requires a value")
			}
			displayName = args[i+1]
			i++
		case strings.HasPrefix(arg, "--name="):
			displayName = strings.TrimPrefix(arg, "--name=")
		case strings.HasPrefix(arg, "-n="):
			displayName = strings.TrimPrefix(arg, "-n=")
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag: %s", arg)
		default:
			return fmt.Errorf("unexpected argument: %s", arg)
		}
	}

	if displayName == "" {
		return fmt.Errorf("--name flag is required")
	}

	// Validate display name
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return fmt.Errorf("display name cannot be empty or whitespace only")
	}
	if len(displayName) > 100 {
		return fmt.Errorf("display name exceeds maximum length of 100 characters")
	}

	configPath := getConfigPath()
	dataPath := getDataPath()
	dbPath := filepath.Join(dataPath, "gateway.db")

	green := color.New(color.FgGreen)
	cyan := color.New(color.FgCyan)
	yellow := color.New(color.FgYellow)

	// Check if config exists, create if not
	var cfg *config.Config
	var jwtSecret string

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Generate random JWT secret
		secretBytes := make([]byte, 32)
		if _, err := rand.Read(secretBytes); err != nil {
			return fmt.Errorf("generating JWT secret: %w", err)
		}
		jwtSecret = base64.StdEncoding.EncodeToString(secretBytes)

		// Create config directory
		configDir := filepath.Dir(configPath)
		if err := os.MkdirAll(configDir, 0755); err != nil {
			return fmt.Errorf("creating config directory: %w", err)
		}

		// Create data directory
		if err := os.MkdirAll(dataPath, 0755); err != nil {
			return fmt.Errorf("creating data directory: %w", err)
		}

		// Write config file
		configContent := fmt.Sprintf(`# coven-gateway configuration
# Generated by coven-gateway bootstrap

server:
  grpc_addr: "localhost:50051"
  http_addr: "localhost:8080"

database:
  path: "%s"

auth:
  jwt_secret: "%s"

logging:
  level: "info"
  format: "text"
`, dbPath, jwtSecret)

		if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
			return fmt.Errorf("writing config file: %w", err)
		}

		green.Printf("  ✓ Created config: %s\n", configPath)

		// Load the config we just created
		cfg, err = config.Load(configPath)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
	} else {
		// Config exists, load it
		var err error
		cfg, err = config.Load(configPath)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		// Check JWT secret is configured
		if cfg.Auth.JWTSecret == "" {
			return fmt.Errorf("jwt_secret not configured in %s (required for bootstrap)", configPath)
		}
		jwtSecret = cfg.Auth.JWTSecret

		cyan.Printf("  Using existing config: %s\n", configPath)
	}

	// Open the store directly
	s, err := store.NewSQLiteStore(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer s.Close()

	green.Printf("  ✓ Database: %s\n", cfg.Database.Path)

	// Check if any principals already exist
	count, err := s.CountPrincipals(ctx, store.PrincipalFilter{})
	if err != nil {
		return fmt.Errorf("checking principals: %w", err)
	}

	if count > 0 {
		return fmt.Errorf("bootstrap already complete: %d principal(s) exist", count)
	}

	// Create the owner principal
	principalID := uuid.New().String()
	principal := &store.Principal{
		ID:          principalID,
		Type:        store.PrincipalTypeClient,
		DisplayName: displayName,
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now().UTC(),
	}

	if err := s.CreatePrincipal(ctx, principal); err != nil {
		return fmt.Errorf("creating principal: %w", err)
	}

	// Grant owner role. If this fails, attempt to clean up the principal
	// to avoid leaving the system in a partially bootstrapped state.
	if err := s.AddRole(ctx, store.RoleSubjectPrincipal, principalID, store.RoleOwner); err != nil {
		// Best-effort cleanup - ignore errors since we're already failing
		_ = s.DeletePrincipal(ctx, principalID)
		return fmt.Errorf("granting owner role: %w", err)
	}

	green.Printf("  ✓ Created owner principal: %s\n", displayName)

	// Generate JWT token
	verifier, err := auth.NewJWTVerifier([]byte(jwtSecret))
	if err != nil {
		return fmt.Errorf("creating JWT verifier: %w", err)
	}

	// Default TTL: 30 days
	tokenTTL := 30 * 24 * time.Hour
	expiresAt := time.Now().Add(tokenTTL).UTC()

	token, err := verifier.Generate(principalID, tokenTTL)
	if err != nil {
		return fmt.Errorf("generating token: %w", err)
	}

	// Save token to file for CLI tools to read
	tokenPath := filepath.Join(filepath.Dir(configPath), "token")
	if err := os.WriteFile(tokenPath, []byte(token), 0600); err != nil {
		return fmt.Errorf("writing token file: %w", err)
	}

	green.Printf("  ✓ Saved token: %s\n", tokenPath)

	// Print results
	fmt.Println()
	green.Println("  Bootstrap complete!")
	fmt.Println()
	cyan.Println("  Owner Principal")
	cyan.Println("  ---------------")
	fmt.Printf("  ID:           %s\n", principalID)
	fmt.Printf("  Display Name: %s\n", displayName)
	fmt.Printf("  Type:         client\n")
	fmt.Printf("  Status:       approved\n")
	fmt.Printf("  Roles:        owner\n")
	fmt.Printf("  Token:        %s (expires %s)\n", tokenPath, expiresAt.Format("Jan 02, 2006"))
	fmt.Println()

	yellow.Println("  Ready to go:")
	fmt.Println("    coven-gateway serve    # start the gateway")
	fmt.Println("    coven-admin me         # verify your identity")
	fmt.Println()

	return nil
}

func runInit() error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("coven-gateway configuration setup")
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
		tsHostname = prompt(reader, "Tailscale hostname", "coven-gateway")
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
	cfg.WriteString("# coven-gateway configuration\n")
	cfg.WriteString("# Generated by coven-gateway init\n\n")

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
	fmt.Printf("  coven-gateway serve\n")

	return nil
}

func prompt(reader *bufio.Reader, question, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", question, defaultVal)
	} else {
		fmt.Printf("%s: ", question)
	}

	input, err := reader.ReadString('\n')
	if err != nil {
		// On EOF or error, return default
		fmt.Println()
		return defaultVal
	}
	input = strings.TrimSpace(input)

	if input == "" {
		return defaultVal
	}
	return input
}
