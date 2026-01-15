// ABOUTME: Entry point for fold-gateway control server
// ABOUTME: Manages fold agents and frontend connections

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
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
	// TODO: Implement
	// 1. Load config
	// 2. Initialize store
	// 3. Initialize agent manager
	// 4. Initialize frontends
	// 5. Start GRPC server
	// 6. Wait for shutdown signal
	fmt.Println("fold-gateway serve: not yet implemented")
	fmt.Println("See SPEC.md for implementation plan")
	return nil
}

func runHealth(ctx context.Context) error {
	// TODO: Implement - HTTP health check
	fmt.Println("fold-gateway health: not yet implemented")
	return nil
}

func runAgents(ctx context.Context) error {
	// TODO: Implement - list connected agents
	fmt.Println("fold-gateway agents: not yet implemented")
	return nil
}
