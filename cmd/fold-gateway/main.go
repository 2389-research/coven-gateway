// ABOUTME: Entry point for fold-gateway control server
// ABOUTME: Manages fold agents and frontend connections

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/2389/fold-gateway/internal/config"
	"github.com/2389/fold-gateway/internal/gateway"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: fold-gateway <command>")
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("  serve     Start the gateway server")
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
	// Determine config path
	configPath := os.Getenv("FOLD_CONFIG")
	if configPath == "" {
		configPath = "config.yaml"
	}

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
	// Get HTTP address from config or use default
	configPath := os.Getenv("FOLD_CONFIG")
	if configPath == "" {
		configPath = "config.yaml"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Make HTTP request to health endpoint
	url := fmt.Sprintf("http://%s/health", cfg.Server.HTTPAddr)
	resp, err := http.Get(url)
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
	// Get HTTP address from config or use default
	configPath := os.Getenv("FOLD_CONFIG")
	if configPath == "" {
		configPath = "config.yaml"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Make HTTP request to ready endpoint
	url := fmt.Sprintf("http://%s/health/ready", cfg.Server.HTTPAddr)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("agents check failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	buf := make([]byte, 1024)
	n, _ := resp.Body.Read(buf)

	fmt.Println(string(buf[:n]))
	return nil
}
