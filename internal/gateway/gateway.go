// ABOUTME: Gateway orchestrator that coordinates GRPC and HTTP servers
// ABOUTME: Manages agent connections, store, and health endpoints lifecycle

package gateway

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/grpc"
	"tailscale.com/tsnet"

	"github.com/2389/fold-gateway/internal/admin"
	"github.com/2389/fold-gateway/internal/agent"
	"github.com/2389/fold-gateway/internal/auth"
	"github.com/2389/fold-gateway/internal/client"
	"github.com/2389/fold-gateway/internal/config"
	"github.com/2389/fold-gateway/internal/conversation"
	"github.com/2389/fold-gateway/internal/dedupe"
	"github.com/2389/fold-gateway/internal/store"
	"github.com/2389/fold-gateway/internal/webadmin"
	pb "github.com/2389/fold-gateway/proto/fold"
)

// Gateway orchestrates the fold-gateway server components.
// It manages the GRPC server for agent connections and HTTP server for health checks.
type Gateway struct {
	config       *config.Config
	agentManager *agent.Manager
	store        store.Store
	conversation *conversation.Service
	grpcServer   *grpc.Server
	httpServer   *http.Server
	tsnetServer  *tsnet.Server
	webAdmin     *webadmin.Admin
	logger       *slog.Logger

	// serverID identifies this gateway instance
	serverID string

	// dedupe is used to prevent duplicate bridge message processing
	dedupe *dedupe.Cache

	// mockSender is used for testing to inject a mock message sender
	mockSender messageSender
}

// New creates a new Gateway instance with the given configuration.
func New(cfg *config.Config, logger *slog.Logger) (*Gateway, error) {
	// Initialize store
	// FOLD_DB_PATH env var overrides config for Docker deployments
	var s store.Store
	var err error

	dbPath := cfg.Database.Path
	if envPath := os.Getenv("FOLD_DB_PATH"); envPath != "" {
		dbPath = envPath
	}

	if dbPath == ":memory:" {
		// For testing, use in-memory store
		s, err = store.NewSQLiteStore(":memory:")
	} else {
		s, err = store.NewSQLiteStore(dbPath)
	}
	if err != nil {
		return nil, fmt.Errorf("initializing store: %w", err)
	}

	// Create agent manager
	agentMgr := agent.NewManager(logger.With("component", "agent-manager"))

	// Create dedupe cache for bridge message deduplication
	// TTL of 5 minutes, max size 100,000 entries
	dedupeCache := dedupe.New(5*time.Minute, 100_000)

	// Create gRPC server with auth interceptors if JWT secret is configured
	var grpcServer *grpc.Server
	var jwtVerifier *auth.JWTVerifier // Stored for token generation
	if cfg.Auth.JWTSecret != "" {
		// Create JWT verifier for client auth
		var err error
		jwtVerifier, err = auth.NewJWTVerifier([]byte(cfg.Auth.JWTSecret))
		if err != nil {
			return nil, fmt.Errorf("creating JWT verifier: %w", err)
		}

		// Create SSH verifier for agent auth
		sshVerifier := auth.NewSSHVerifier()

		// Create store adapter for auth interceptors
		sqlStore := s.(*store.SQLiteStore)

		// Create auth config for auto-registration
		authConfig := &auth.AuthConfig{
			AgentAutoRegistration: cfg.Auth.AgentAutoRegistration,
		}
		// Default to "disabled" if not set (secure by default)
		if authConfig.AgentAutoRegistration == "" {
			authConfig.AgentAutoRegistration = "disabled"
		}

		// Create gRPC server with auth interceptors
		// Supports both JWT (clients) and SSH key (agents) authentication
		grpcServer = grpc.NewServer(
			grpc.ChainUnaryInterceptor(
				auth.UnaryInterceptor(sqlStore, sqlStore, jwtVerifier, sshVerifier, authConfig, sqlStore),
				auth.RequireAdmin(),
			),
			grpc.ChainStreamInterceptor(
				auth.StreamInterceptor(sqlStore, sqlStore, jwtVerifier, sshVerifier, authConfig, sqlStore),
				auth.RequireAdminStream(),
			),
		)
		logger.Info("auth interceptors enabled (JWT + SSH)")
	} else {
		// No auth - create plain gRPC server
		grpcServer = grpc.NewServer()
		logger.Warn("auth disabled - no jwt_secret configured")
	}

	// Create conversation service (central message persistence layer)
	convService := conversation.New(
		s.(*store.SQLiteStore),
		agentMgr,
		logger.With("component", "conversation"),
	)

	// Create gateway
	gw := &Gateway{
		config:       cfg,
		agentManager: agentMgr,
		store:        s,
		conversation: convService,
		grpcServer:   grpcServer,
		logger:       logger.With("component", "gateway"),
		serverID:     generateServerID(),
		dedupe:       dedupeCache,
	}

	// Register FoldControl service (agent streaming - no auth required for now)
	foldService := newFoldControlServer(gw, logger.With("component", "grpc"))
	pb.RegisterFoldControlServer(grpcServer, foldService)

	// Register AdminService and ClientService
	sqliteStore := s.(*store.SQLiteStore)
	if jwtVerifier != nil {
		// Use PrincipalService which supports token and principal management
		principalService := admin.NewPrincipalService(sqliteStore, jwtVerifier)
		pb.RegisterAdminServiceServer(grpcServer, principalService)
	} else {
		// Use basic AdminService without token/principal management
		adminService := admin.NewAdminService(sqliteStore)
		pb.RegisterAdminServiceServer(grpcServer, adminService)
	}

	clientService := client.NewClientServiceWithDedupe(sqliteStore, sqliteStore, dedupeCache)
	pb.RegisterClientServiceServer(grpcServer, clientService)

	// Create HTTP server for health checks and API
	mux := http.NewServeMux()

	// Health endpoints - no auth required
	mux.HandleFunc("/health", gw.handleHealth)
	mux.HandleFunc("/health/ready", gw.handleReady)

	// API endpoints - auth required if JWT secret is configured
	if cfg.Auth.JWTSecret != "" {
		// Create JWT verifier for HTTP (reuse logic from gRPC)
		httpVerifier, err := auth.NewJWTVerifier([]byte(cfg.Auth.JWTSecret))
		if err != nil {
			return nil, fmt.Errorf("creating HTTP JWT verifier: %w", err)
		}

		// Create auth middleware
		authMiddleware := auth.HTTPAuthMiddleware(sqliteStore, sqliteStore, httpVerifier)
		adminMiddleware := auth.RequireAdminHTTP()

		// Protected endpoints - any authenticated user
		mux.Handle("/api/agents", authMiddleware(http.HandlerFunc(gw.handleListAgents)))
		mux.Handle("/api/agents/", authMiddleware(http.HandlerFunc(gw.handleAgentHistory)))
		mux.Handle("/api/send", authMiddleware(http.HandlerFunc(gw.handleSendMessage)))
		mux.Handle("/api/threads/", authMiddleware(http.HandlerFunc(gw.handleThreadMessages)))

		// Admin endpoints - requires admin role for mutations
		// GET is allowed for any authenticated user, POST/DELETE require admin
		mux.Handle("/api/bindings", authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost || r.Method == http.MethodDelete {
				adminMiddleware(http.HandlerFunc(gw.handleBindings)).ServeHTTP(w, r)
			} else {
				gw.handleBindings(w, r)
			}
		})))

		logger.Info("HTTP auth middleware enabled")
	} else {
		// No auth - register handlers directly
		mux.HandleFunc("/api/agents", gw.handleListAgents)
		mux.HandleFunc("/api/agents/", gw.handleAgentHistory)
		mux.HandleFunc("/api/send", gw.handleSendMessage)
		mux.HandleFunc("/api/bindings", gw.handleBindings)
		mux.HandleFunc("/api/threads/", gw.handleThreadMessages)
		logger.Warn("HTTP auth disabled - no jwt_secret configured")
	}

	// Register web admin UI routes
	// The admin UI has its own session-based auth (separate from JWT)
	webAdminBaseURL := cfg.WebAdmin.BaseURL
	if webAdminBaseURL == "" {
		// Check FOLD_GATEWAY_URL env var (includes full tailnet DNS name)
		if envURL := os.Getenv("FOLD_GATEWAY_URL"); envURL != "" {
			webAdminBaseURL = envURL
		} else if cfg.Tailscale.Enabled {
			// Auto-detect based on deployment mode
			// With Tailscale HTTPS, user MUST set FOLD_GATEWAY_URL or webadmin.base_url
			// to the full tailnet DNS name for WebAuthn to work
			if cfg.Tailscale.HTTPS || cfg.Tailscale.Funnel {
				logger.Warn("webadmin.base_url/FOLD_GATEWAY_URL not set - WebAuthn/passkeys may fail. Set FOLD_GATEWAY_URL to full tailnet URL (e.g., https://fold-gateway.your-tailnet.ts.net)")
			}
			scheme := "http"
			if cfg.Tailscale.HTTPS || cfg.Tailscale.Funnel {
				scheme = "https"
			}
			webAdminBaseURL = scheme + "://" + cfg.Tailscale.Hostname
		} else {
			webAdminBaseURL = "http://" + cfg.Server.HTTPAddr
		}
	}
	webAdminCfg := webadmin.Config{
		BaseURL: webAdminBaseURL,
	}
	gw.webAdmin = webadmin.New(sqliteStore, gw.agentManager, convService, webAdminCfg)
	gw.webAdmin.RegisterRoutes(mux)
	logger.Info("admin web UI enabled at /admin/", "base_url", webAdminBaseURL)

	gw.httpServer = &http.Server{
		Addr:    cfg.Server.HTTPAddr,
		Handler: mux,
	}

	return gw, nil
}

// Run starts the gateway servers and blocks until the context is cancelled.
// It manages graceful shutdown of both GRPC and HTTP servers.
// Returns nil on graceful shutdown (context cancelled), or an error if a server fails.
func (g *Gateway) Run(ctx context.Context) error {
	// Channel to collect errors from servers
	errCh := make(chan error, 2)

	var grpcListener, httpListener net.Listener
	var err error

	// Setup listeners - either via Tailscale or regular TCP
	if g.config.Tailscale.Enabled {
		// Warn if server addresses are configured but will be ignored
		if g.config.Server.GRPCAddr != "" || g.config.Server.HTTPAddr != "" {
			g.logger.Warn("server.grpc_addr and server.http_addr are ignored when tailscale is enabled",
				"grpc_addr", g.config.Server.GRPCAddr,
				"http_addr", g.config.Server.HTTPAddr,
			)
		}
		grpcListener, httpListener, err = g.setupTailscaleListeners(ctx)
		if err != nil {
			return fmt.Errorf("setting up tailscale: %w", err)
		}
	} else {
		g.logger.Info("starting gateway",
			"grpc_addr", g.config.Server.GRPCAddr,
			"http_addr", g.config.Server.HTTPAddr,
		)

		grpcListener, err = net.Listen("tcp", g.config.Server.GRPCAddr)
		if err != nil {
			return fmt.Errorf("listening on gRPC address: %w", err)
		}

		httpListener, err = net.Listen("tcp", g.config.Server.HTTPAddr)
		if err != nil {
			grpcListener.Close()
			return fmt.Errorf("listening on HTTP address: %w", err)
		}
	}

	// Start GRPC server
	go func() {
		g.logger.Info("gRPC server listening", "addr", grpcListener.Addr().String())
		if err := g.grpcServer.Serve(grpcListener); err != nil {
			errCh <- fmt.Errorf("gRPC server: %w", err)
		}
	}()

	// Start HTTP server
	go func() {
		g.logger.Info("HTTP server listening", "addr", httpListener.Addr().String())
		if err := g.httpServer.Serve(httpListener); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("HTTP server: %w", err)
		}
	}()

	// Wait for context cancellation or server error
	var serverErr error
	select {
	case <-ctx.Done():
		g.logger.Info("context cancelled, initiating shutdown")
	case serverErr = <-errCh:
		g.logger.Error("server error", "error", serverErr)
		// Drain any additional errors (buffer size 2)
		select {
		case additionalErr := <-errCh:
			g.logger.Error("additional server error", "error", additionalErr)
		default:
		}
	}

	// Graceful shutdown - use short timeout since agent streams won't close gracefully
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	shutdownErr := g.Shutdown(shutdownCtx)

	// Return server error if one occurred, otherwise return shutdown error
	if serverErr != nil {
		return serverErr
	}
	return shutdownErr
}

// setupTailscaleListeners creates a tsnet server and returns listeners for gRPC and HTTP.
func (g *Gateway) setupTailscaleListeners(ctx context.Context) (grpcLn, httpLn net.Listener, err error) {
	tsCfg := g.config.Tailscale

	// Determine state directory
	stateDir := tsCfg.StateDir
	if stateDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, nil, fmt.Errorf("cannot determine home directory for tailscale state (set tailscale.state_dir explicitly): %w", err)
		}
		stateDir = filepath.Join(homeDir, ".local", "share", "fold-gateway", "tailscale")
	}

	// Ensure state directory exists
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return nil, nil, fmt.Errorf("creating tailscale state dir: %w", err)
	}

	// Create tsnet server
	g.tsnetServer = &tsnet.Server{
		Hostname:  tsCfg.Hostname,
		Dir:       stateDir,
		Ephemeral: tsCfg.Ephemeral,
	}

	// Set auth key - required for non-interactive (container) deployments.
	// Precedence: config value > TS_AUTHKEY env var.
	authKey := tsCfg.AuthKey
	if authKey == "" {
		authKey = os.Getenv("TS_AUTHKEY")
	}
	if authKey == "" {
		return nil, nil, fmt.Errorf("tailscale auth key required: set auth_key in config or TS_AUTHKEY environment variable (get one at https://login.tailscale.com/admin/settings/keys)")
	}
	g.tsnetServer.AuthKey = authKey

	g.logger.Info("starting tailscale node",
		"hostname", tsCfg.Hostname,
		"state_dir", stateDir,
		"ephemeral", tsCfg.Ephemeral,
	)

	// Start and wait for tailscale to be ready
	status, err := g.tsnetServer.Up(ctx)
	if err != nil {
		_ = g.tsnetServer.Close() // Cleanup partially initialized server
		return nil, nil, fmt.Errorf("starting tailscale: %w", err)
	}

	// Log tailscale address info
	var tsAddr, dnsName string
	if len(status.TailscaleIPs) > 0 {
		tsAddr = status.TailscaleIPs[0].String()
	} else {
		g.logger.Warn("tailscale node has no IP addresses assigned")
	}
	if status.Self != nil {
		dnsName = status.Self.DNSName
	}

	g.logger.Info("tailscale node ready",
		"hostname", tsCfg.Hostname,
		"tailscale_ip", tsAddr,
		"dns_name", dnsName,
	)

	// Create gRPC listener on port 50051
	grpcLn, err = g.tsnetServer.Listen("tcp", ":50051")
	if err != nil {
		g.tsnetServer.Close()
		return nil, nil, fmt.Errorf("listening on tailscale gRPC port: %w", err)
	}

	// Create HTTP listener based on config:
	// - Funnel: public HTTPS on :443 (Tailscale terminates TLS)
	// - HTTPS: private HTTPS on :443 (auto-provisioned Tailscale certs)
	// - Neither: HTTP on :80 (tailnet only, no passkey support)
	if tsCfg.Funnel {
		g.logger.Info("enabling tailscale funnel (public HTTPS) on :443")
		httpLn, err = g.tsnetServer.ListenFunnel("tcp", ":443")
	} else if tsCfg.HTTPS {
		// Use Tailscale's auto-provisioned HTTPS certs via LocalClient
		g.logger.Info("enabling HTTPS with Tailscale certs on :443")
		ln, listenErr := g.tsnetServer.Listen("tcp", ":443")
		if listenErr != nil {
			grpcLn.Close()
			g.tsnetServer.Close()
			return nil, nil, fmt.Errorf("listening on tailscale HTTPS port: %w", listenErr)
		}
		lc, lcErr := g.tsnetServer.LocalClient()
		if lcErr != nil {
			ln.Close()
			grpcLn.Close()
			g.tsnetServer.Close()
			return nil, nil, fmt.Errorf("getting tailscale local client: %w", lcErr)
		}
		httpLn = tls.NewListener(ln, &tls.Config{
			GetCertificate: lc.GetCertificate,
		})
	} else {
		httpLn, err = g.tsnetServer.Listen("tcp", ":80")
	}
	if err != nil {
		grpcLn.Close()
		g.tsnetServer.Close()
		return nil, nil, fmt.Errorf("listening on tailscale HTTP port: %w", err)
	}

	return grpcLn, httpLn, nil
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

	// Close tsnet server if running
	if g.tsnetServer != nil {
		if err := g.tsnetServer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("tailscale shutdown: %w", err))
		}
	}

	// Close store
	if err := g.store.Close(); err != nil {
		errs = append(errs, fmt.Errorf("store close: %w", err))
	}

	// Close dedupe cache (stops background cleanup goroutine)
	if g.dedupe != nil {
		g.dedupe.Close()
	}

	// Close web admin (cleans up chat hub and sessions)
	if g.webAdmin != nil {
		g.webAdmin.Close()
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
