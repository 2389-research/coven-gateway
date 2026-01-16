// ABOUTME: Gateway orchestrator that coordinates GRPC and HTTP servers
// ABOUTME: Manages agent connections, store, and health endpoints lifecycle

package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"google.golang.org/grpc"

	"github.com/2389/fold-gateway/internal/agent"
	"github.com/2389/fold-gateway/internal/config"
	"github.com/2389/fold-gateway/internal/store"
	pb "github.com/2389/fold-gateway/proto/fold"
)

// Gateway orchestrates the fold-gateway server components.
// It manages the GRPC server for agent connections and HTTP server for health checks.
type Gateway struct {
	config       *config.Config
	agentManager *agent.Manager
	store        store.Store
	grpcServer   *grpc.Server
	httpServer   *http.Server
	logger       *slog.Logger

	// serverID identifies this gateway instance
	serverID string

	// mockSender is used for testing to inject a mock message sender
	mockSender messageSender
}

// New creates a new Gateway instance with the given configuration.
func New(cfg *config.Config, logger *slog.Logger) (*Gateway, error) {
	// Initialize store
	var s store.Store
	var err error

	if cfg.Database.Path == ":memory:" {
		// For testing, use in-memory store
		s, err = store.NewSQLiteStore(":memory:")
	} else {
		s, err = store.NewSQLiteStore(cfg.Database.Path)
	}
	if err != nil {
		return nil, fmt.Errorf("initializing store: %w", err)
	}

	// Create agent manager
	agentMgr := agent.NewManager(logger.With("component", "agent-manager"))

	// Create gRPC server
	grpcServer := grpc.NewServer()

	// Create gateway
	gw := &Gateway{
		config:       cfg,
		agentManager: agentMgr,
		store:        s,
		grpcServer:   grpcServer,
		logger:       logger.With("component", "gateway"),
		serverID:     generateServerID(),
	}

	// Register FoldControl service
	foldService := newFoldControlServer(gw, logger.With("component", "grpc"))
	pb.RegisterFoldControlServer(grpcServer, foldService)

	// Create HTTP server for health checks and API
	mux := http.NewServeMux()
	mux.HandleFunc("/health", gw.handleHealth)
	mux.HandleFunc("/health/ready", gw.handleReady)
	mux.HandleFunc("/api/agents", gw.handleListAgents)
	mux.HandleFunc("/api/send", gw.handleSendMessage)

	gw.httpServer = &http.Server{
		Addr:    cfg.Server.HTTPAddr,
		Handler: mux,
	}

	return gw, nil
}

// Run starts the gateway servers and blocks until the context is cancelled.
// It manages graceful shutdown of both GRPC and HTTP servers.
func (g *Gateway) Run(ctx context.Context) error {
	g.logger.Info("starting gateway",
		"grpc_addr", g.config.Server.GRPCAddr,
		"http_addr", g.config.Server.HTTPAddr,
	)

	// Channel to collect errors from servers
	errCh := make(chan error, 2)

	// Start GRPC server
	grpcListener, err := net.Listen("tcp", g.config.Server.GRPCAddr)
	if err != nil {
		return fmt.Errorf("listening on gRPC address: %w", err)
	}

	go func() {
		g.logger.Info("gRPC server listening", "addr", g.config.Server.GRPCAddr)
		if err := g.grpcServer.Serve(grpcListener); err != nil {
			errCh <- fmt.Errorf("gRPC server: %w", err)
		}
	}()

	// Start HTTP server
	httpListener, err := net.Listen("tcp", g.config.Server.HTTPAddr)
	if err != nil {
		g.grpcServer.Stop()
		return fmt.Errorf("listening on HTTP address: %w", err)
	}

	go func() {
		g.logger.Info("HTTP server listening", "addr", g.config.Server.HTTPAddr)
		if err := g.httpServer.Serve(httpListener); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("HTTP server: %w", err)
		}
	}()

	// Wait for context cancellation or server error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			g.logger.Info("context cancelled, initiating shutdown")
		case err := <-errCh:
			g.logger.Error("server error", "error", err)
		}

		// Graceful shutdown - use short timeout since agent streams won't close gracefully
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_ = g.Shutdown(shutdownCtx)
	}()

	wg.Wait()
	return nil
}

// Shutdown gracefully stops all gateway servers and releases resources.
func (g *Gateway) Shutdown(ctx context.Context) error {
	g.logger.Info("shutting down gateway")

	var errs []error

	// Stop HTTP server gracefully
	if err := g.httpServer.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("HTTP shutdown: %w", err))
	}

	// Stop GRPC server gracefully
	stopped := make(chan struct{})
	go func() {
		g.grpcServer.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
		// Graceful stop completed
	case <-ctx.Done():
		// Force stop
		g.grpcServer.Stop()
	}

	// Close store
	if err := g.store.Close(); err != nil {
		errs = append(errs, fmt.Errorf("store close: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}
	return nil
}

// handleHealth returns 200 OK if the server is alive
func (g *Gateway) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// handleReady returns 200 OK if the server has at least one agent connected
func (g *Gateway) handleReady(w http.ResponseWriter, r *http.Request) {
	agents := g.agentManager.ListAgents()
	if len(agents) == 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("no agents connected"))
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf("ready (%d agents)", len(agents))))
}

// generateServerID creates a unique identifier for this gateway instance
func generateServerID() string {
	return fmt.Sprintf("fold-gateway-%d", time.Now().UnixNano()%1000000)
}
