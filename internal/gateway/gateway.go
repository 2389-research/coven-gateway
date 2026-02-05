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
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"tailscale.com/tsnet"

	"github.com/2389/coven-gateway/internal/admin"
	"github.com/2389/coven-gateway/internal/agent"
	"github.com/2389/coven-gateway/internal/auth"
	"github.com/2389/coven-gateway/internal/builtins"
	"github.com/2389/coven-gateway/internal/client"
	"github.com/2389/coven-gateway/internal/config"
	"github.com/2389/coven-gateway/internal/conversation"
	"github.com/2389/coven-gateway/internal/dedupe"
	"github.com/2389/coven-gateway/internal/mcp"
	"github.com/2389/coven-gateway/internal/packs"
	"github.com/2389/coven-gateway/internal/store"
	"github.com/2389/coven-gateway/internal/webadmin"
	pb "github.com/2389/coven-gateway/proto/coven"
)

// Gateway orchestrates the coven-gateway server components.
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

	// packRegistry tracks connected tool packs
	packRegistry *packs.Registry

	// packRouter routes tool calls to packs
	packRouter *packs.Router

	// mcpTokens maps MCP access tokens to agent capabilities
	mcpTokens *mcp.TokenStore

	// mcpEndpoint is the base URL for MCP endpoint (e.g., "http://localhost:8080/mcp")
	mcpEndpoint string

	// questionRouter handles ask_user tool question routing
	questionRouter *builtins.InMemoryQuestionRouter

	// mockSender is used for testing to inject a mock message sender
	mockSender messageSender
}

// New creates a new Gateway instance with the given configuration.
func New(cfg *config.Config, logger *slog.Logger) (*Gateway, error) {
	// Initialize store
	// COVEN_DB_PATH env var overrides config for Docker deployments
	var s store.Store
	var err error

	dbPath := cfg.Database.Path
	if envPath := os.Getenv("COVEN_DB_PATH"); envPath != "" {
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
			grpc.KeepaliveParams(keepalive.ServerParameters{
				Time:    15 * time.Second, // Ping client if idle for 15s
				Timeout: 5 * time.Second,  // Wait 5s for ping ack
			}),
			grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
				MinTime:             5 * time.Second, // Allow pings as frequent as every 5s
				PermitWithoutStream: true,            // Allow pings even without active RPC
			}),
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
		// No auth - create gRPC server with anonymous auth interceptors
		// These inject a placeholder auth context to prevent panics in handlers
		grpcServer = grpc.NewServer(
			grpc.KeepaliveParams(keepalive.ServerParameters{
				Time:    15 * time.Second, // Ping client if idle for 15s
				Timeout: 5 * time.Second,  // Wait 5s for ping ack
			}),
			grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
				MinTime:             5 * time.Second, // Allow pings as frequent as every 5s
				PermitWithoutStream: true,            // Allow pings even without active RPC
			}),
			grpc.ChainUnaryInterceptor(auth.NoAuthUnaryInterceptor()),
			grpc.ChainStreamInterceptor(auth.NoAuthStreamInterceptor()),
		)
		logger.Warn("auth disabled - no jwt_secret configured")
	}

	// Create conversation service (central message persistence layer)
	convService := conversation.New(
		s.(*store.SQLiteStore),
		agentMgr,
		logger.With("component", "conversation"),
	)

	// Create pack registry and router for tool pack support
	packRegistry := packs.NewRegistry(logger.With("component", "pack-registry"))
	packRouter := packs.NewRouter(packs.RouterConfig{
		Registry: packRegistry,
		Logger:   logger.With("component", "pack-router"),
	})

	// Register built-in packs
	// SQLiteStore implements both store.Store and store.BuiltinStore interfaces
	builtinStore := s.(*store.SQLiteStore)
	if err := packRegistry.RegisterBuiltinPack(builtins.BasePack(builtinStore)); err != nil {
		return nil, fmt.Errorf("registering base pack: %w", err)
	}
	if err := packRegistry.RegisterBuiltinPack(builtins.AdminPack(agentMgr, s)); err != nil {
		return nil, fmt.Errorf("registering admin pack: %w", err)
	}
	if err := packRegistry.RegisterBuiltinPack(builtins.MailPack(builtinStore)); err != nil {
		return nil, fmt.Errorf("registering mail pack: %w", err)
	}
	if err := packRegistry.RegisterBuiltinPack(builtins.NotesPack(builtinStore)); err != nil {
		return nil, fmt.Errorf("registering notes pack: %w", err)
	}

	// Note: UIPack is registered after webAdmin is created since it needs webAdmin as ClientStreamer

	// Create MCP token store for agent capability-scoped access
	mcpTokens := mcp.NewTokenStore()

	// Determine MCP endpoint URL
	// Priority: COVEN_MCP_ENDPOINT env > COVEN_GATEWAY_URL + /mcp > derived from config
	var mcpEndpoint string
	if envEndpoint := os.Getenv("COVEN_MCP_ENDPOINT"); envEndpoint != "" {
		mcpEndpoint = envEndpoint
	} else if envGatewayURL := os.Getenv("COVEN_GATEWAY_URL"); envGatewayURL != "" {
		mcpEndpoint = envGatewayURL + "/mcp"
	} else if cfg.Tailscale.Enabled {
		// Derive from tailscale config
		// Default to HTTPS since Tailscale auto-provisions certs for all nodes
		// Only use HTTP if explicitly disabled via config (not recommended)
		scheme := "https"
		if !cfg.Tailscale.HTTPS && !cfg.Tailscale.Funnel {
			// Check if HTTP-only mode is explicitly configured
			// For now, still default to HTTPS as it's the common case
			// Users can override via COVEN_MCP_ENDPOINT if needed
			logger.Info("Tailscale HTTPS not explicitly enabled, but defaulting to HTTPS for MCP endpoint")
		}
		mcpEndpoint = scheme + "://" + cfg.Tailscale.Hostname + "/mcp"
	} else {
		// Use HTTP address from config
		mcpEndpoint = "http://" + cfg.Server.HTTPAddr + "/mcp"
	}
	logger.Info("MCP endpoint configured", "endpoint", mcpEndpoint)

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
		packRegistry: packRegistry,
		packRouter:   packRouter,
		mcpTokens:    mcpTokens,
		mcpEndpoint:  mcpEndpoint,
	}

	// Register CovenControl service (agent streaming - no auth required for now)
	covenService := newCovenControlServer(gw, logger.With("component", "grpc"))
	pb.RegisterCovenControlServer(grpcServer, covenService)

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

	clientService := client.NewClientServiceWithRouter(sqliteStore, sqliteStore, dedupeCache, agentMgr, agentMgr)
	clientService.SetToolApprover(agentMgr)
	pb.RegisterClientServiceServer(grpcServer, clientService)

	// Register PackService for tool pack support
	packService := packs.NewPackServiceServer(gw.packRegistry, gw.packRouter, logger.With("component", "pack-service"))
	pb.RegisterPackServiceServer(grpcServer, packService)

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
		mux.Handle("/api/threads/", authMiddleware(http.HandlerFunc(gw.handleThreadRoutes)))
		mux.Handle("/api/stats/usage", authMiddleware(http.HandlerFunc(gw.handleUsageStats)))
		mux.Handle("/api/tools/approve", authMiddleware(http.HandlerFunc(gw.handleToolApproval)))
		mux.Handle("/api/questions/answer", authMiddleware(http.HandlerFunc(gw.handleAnswerQuestion)))

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
		mux.HandleFunc("/api/threads/", gw.handleThreadRoutes)
		mux.HandleFunc("/api/stats/usage", gw.handleUsageStats)
		mux.HandleFunc("/api/tools/approve", gw.handleToolApproval)
		mux.HandleFunc("/api/questions/answer", gw.handleAnswerQuestion)
		logger.Warn("HTTP auth disabled - no jwt_secret configured")
	}

	// Register web admin UI routes
	// The admin UI has its own session-based auth (separate from JWT)
	webAdminBaseURL := cfg.WebAdmin.BaseURL
	if webAdminBaseURL == "" {
		// Check COVEN_GATEWAY_URL env var (includes full tailnet DNS name)
		if envURL := os.Getenv("COVEN_GATEWAY_URL"); envURL != "" {
			webAdminBaseURL = envURL
		} else if cfg.Tailscale.Enabled {
			// Auto-detect based on deployment mode
			// With Tailscale HTTPS, user MUST set COVEN_GATEWAY_URL or webadmin.base_url
			// to the full tailnet DNS name for WebAuthn to work
			if cfg.Tailscale.HTTPS || cfg.Tailscale.Funnel {
				logger.Warn("webadmin.base_url/COVEN_GATEWAY_URL not set - WebAuthn/passkeys may fail. Set COVEN_GATEWAY_URL to full tailnet URL (e.g., https://coven-gateway.your-tailnet.ts.net)")
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
	webAdminCfg := webadmin.NewConfig{
		Store:        sqliteStore,
		Manager:      gw.agentManager,
		Conversation: convService,
		Registry:     packRegistry,
		Config: webadmin.Config{
			BaseURL: webAdminBaseURL,
		},
		PrincipalStore: sqliteStore,
		TokenGenerator: jwtVerifier, // May be nil if auth is disabled
	}
	gw.webAdmin = webadmin.NewWithConfig(webAdminCfg)
	gw.webAdmin.RegisterRoutes(mux)
	logger.Info("admin web UI enabled at /admin/", "base_url", webAdminBaseURL)

	// Create question router for ask_user tool (uses webAdmin as ClientStreamer)
	gw.questionRouter = builtins.NewInMemoryQuestionRouter(gw.webAdmin)
	if err := packRegistry.RegisterBuiltinPack(builtins.UIPack(gw.questionRouter)); err != nil {
		return nil, fmt.Errorf("registering UI pack: %w", err)
	}
	// Wire up question answerer to ClientService
	clientService.SetQuestionAnswerer(gw.questionRouter)

	// Register MCP server routes for tool pack access
	// MCP endpoints allow external agents (like Claude Code) to list and execute pack tools
	mcpServer, err := mcp.NewServer(mcp.Config{
		Registry:    packRegistry,
		Router:      packRouter,
		TokenStore:  mcpTokens,
		Logger:      logger.With("component", "mcp"),
		RequireAuth: false, // MCP endpoints don't require auth for now
	})
	if err != nil {
		return nil, fmt.Errorf("creating MCP server: %w", err)
	}
	mcpServer.RegisterRoutes(mux)
	logger.Info("MCP server enabled at /mcp (JSON-RPC 2.0)")

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
		stateDir = filepath.Join(homeDir, ".local", "share", "coven-gateway", "tailscale")
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

	// Update MCP endpoint to use the actual DNS name from Tailscale
	// The short hostname (e.g., "coven") won't resolve from the agent's machine,
	// but the full DNS name (e.g., "coven.porpoise-alkaline.ts.net") will.
	if dnsName != "" {
		// Strip trailing dot from DNS name if present
		cleanDNS := strings.TrimSuffix(dnsName, ".")
		// Always use HTTPS for Tailscale - certs are auto-provisioned
		newEndpoint := "https://" + cleanDNS + "/mcp"
		if newEndpoint != g.mcpEndpoint {
			g.logger.Info("updated MCP endpoint to use Tailscale DNS name",
				"old", g.mcpEndpoint,
				"new", newEndpoint,
			)
			g.mcpEndpoint = newEndpoint
		}
	}

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

	// Close pack router (cancels pending tool requests)
	if g.packRouter != nil {
		g.packRouter.Close()
	}

	// Close pack registry (disconnects all packs)
	if g.packRegistry != nil {
		g.packRegistry.Close()
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
	return fmt.Sprintf("coven-gateway-%d", time.Now().UnixNano()%1000000)
}
