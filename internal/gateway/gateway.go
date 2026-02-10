// ABOUTME: Gateway orchestrator that coordinates GRPC and HTTP servers
// ABOUTME: Manages agent connections, store, and health endpoints lifecycle

package gateway

import (
	"context"
	"crypto/tls"
	"errors"
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
	"tailscale.com/ipn/ipnstate"
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

	// mcpServer is the MCP-compatible HTTP server for external agents
	mcpServer *mcp.Server

	// mcpEndpoint is the base URL for MCP endpoint (e.g., "http://localhost:8080/mcp")
	mcpEndpoint string

	// questionRouter handles ask_user tool question routing
	questionRouter *builtins.InMemoryQuestionRouter

	// eventBroadcaster handles cross-client event push
	eventBroadcaster *conversation.EventBroadcaster

	// mockSender is used for testing to inject a mock message sender
	mockSender messageSender
}

// determineWebAdminBaseURL resolves the web admin base URL from config or environment.
func determineWebAdminBaseURL(cfg *config.Config, logger *slog.Logger) string {
	// Use explicit config first
	if cfg.WebAdmin.BaseURL != "" {
		return cfg.WebAdmin.BaseURL
	}

	// Check COVEN_GATEWAY_URL env var (includes full tailnet DNS name)
	if envURL := os.Getenv("COVEN_GATEWAY_URL"); envURL != "" {
		return envURL
	}

	// Auto-detect based on deployment mode
	if !cfg.Tailscale.Enabled {
		return "http://" + cfg.Server.HTTPAddr
	}

	// Tailscale enabled
	if cfg.Tailscale.HTTPS || cfg.Tailscale.Funnel {
		logger.Warn("webadmin.base_url/COVEN_GATEWAY_URL not set - WebAuthn/passkeys may fail. Set COVEN_GATEWAY_URL to full tailnet URL (e.g., https://coven-gateway.your-tailnet.ts.net)")
		return "https://" + cfg.Tailscale.Hostname
	}
	return "http://" + cfg.Tailscale.Hostname
}

// initStore creates and returns a store based on config and environment.
func initStore(cfg *config.Config) (store.Store, error) {
	dbPath := cfg.Database.Path
	if envPath := os.Getenv("COVEN_DB_PATH"); envPath != "" {
		dbPath = envPath
	}

	var s store.Store
	var err error
	if dbPath == ":memory:" {
		s, err = store.NewSQLiteStore(":memory:")
	} else {
		s, err = store.NewSQLiteStore(dbPath)
	}
	if err != nil {
		return nil, fmt.Errorf("initializing store: %w", err)
	}
	return s, nil
}

// grpcServerResult holds the result of creating a gRPC server.
type grpcServerResult struct {
	server      *grpc.Server
	jwtVerifier *auth.JWTVerifier
}

// createAuthenticatedGRPCServer creates a gRPC server with JWT and SSH auth interceptors.
func createAuthenticatedGRPCServer(cfg *config.Config, sqlStore *store.SQLiteStore, logger *slog.Logger) (*grpcServerResult, error) {
	jwtVerifier, err := auth.NewJWTVerifier([]byte(cfg.Auth.JWTSecret))
	if err != nil {
		return nil, fmt.Errorf("creating JWT verifier: %w", err)
	}

	sshVerifier := auth.NewSSHVerifier()

	authConfig := &auth.AuthConfig{
		AgentAutoRegistration: cfg.Auth.AgentAutoRegistration,
	}
	if authConfig.AgentAutoRegistration == "" {
		authConfig.AgentAutoRegistration = "disabled"
	}

	server := grpc.NewServer(
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    15 * time.Second,
			Timeout: 5 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             5 * time.Second,
			PermitWithoutStream: true,
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
	return &grpcServerResult{server: server, jwtVerifier: jwtVerifier}, nil
}

// createUnauthenticatedGRPCServer creates a gRPC server without auth (anonymous mode).
func createUnauthenticatedGRPCServer(logger *slog.Logger) *grpc.Server {
	server := grpc.NewServer(
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    15 * time.Second,
			Timeout: 5 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             5 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.ChainUnaryInterceptor(auth.NoAuthUnaryInterceptor()),
		grpc.ChainStreamInterceptor(auth.NoAuthStreamInterceptor()),
	)
	logger.Warn("auth disabled - no jwt_secret configured")
	return server
}

// createGRPCServer creates a gRPC server with or without auth based on config.
func createGRPCServer(cfg *config.Config, sqlStore *store.SQLiteStore, logger *slog.Logger) (*grpcServerResult, error) {
	if cfg.Auth.JWTSecret != "" {
		return createAuthenticatedGRPCServer(cfg, sqlStore, logger)
	}
	return &grpcServerResult{server: createUnauthenticatedGRPCServer(logger)}, nil
}

// registerBuiltinPacks registers all builtin packs with the registry.
func registerBuiltinPacks(registry *packs.Registry, agentMgr *agent.Manager, s store.Store, builtinStore *store.SQLiteStore) error {
	if err := registry.RegisterBuiltinPack(builtins.BasePack(builtinStore)); err != nil {
		return fmt.Errorf("registering base pack: %w", err)
	}
	if err := registry.RegisterBuiltinPack(builtins.AdminPack(agentMgr, s, builtinStore)); err != nil {
		return fmt.Errorf("registering admin pack: %w", err)
	}
	if err := registry.RegisterBuiltinPack(builtins.MailPack(builtinStore)); err != nil {
		return fmt.Errorf("registering mail pack: %w", err)
	}
	if err := registry.RegisterBuiltinPack(builtins.NotesPack(builtinStore)); err != nil {
		return fmt.Errorf("registering notes pack: %w", err)
	}
	return nil
}

// determineMCPEndpoint resolves the MCP endpoint URL from env or config.
// Priority: COVEN_MCP_ENDPOINT env > COVEN_GATEWAY_URL + /mcp > derived from config.
func determineMCPEndpoint(cfg *config.Config, logger *slog.Logger) string {
	if envEndpoint := os.Getenv("COVEN_MCP_ENDPOINT"); envEndpoint != "" {
		return envEndpoint
	}
	if envGatewayURL := os.Getenv("COVEN_GATEWAY_URL"); envGatewayURL != "" {
		return envGatewayURL + "/mcp"
	}
	if cfg.Tailscale.Enabled {
		scheme := "https"
		if !cfg.Tailscale.HTTPS && !cfg.Tailscale.Funnel {
			logger.Info("Tailscale HTTPS not explicitly enabled, but defaulting to HTTPS for MCP endpoint")
		}
		return scheme + "://" + cfg.Tailscale.Hostname + "/mcp"
	}
	return "http://" + cfg.Server.HTTPAddr + "/mcp"
}

// registerGRPCServices registers all gRPC services on the server.
// Returns the clientService for additional configuration.
func registerGRPCServices(gw *Gateway, grpcServer *grpc.Server, jwtVerifier *auth.JWTVerifier, sqlStore *store.SQLiteStore, dedupeCache *dedupe.Cache, agentMgr *agent.Manager, eventBroadcaster *conversation.EventBroadcaster, logger *slog.Logger) *client.ClientService {
	// Register CovenControl service (agent streaming)
	covenService := newCovenControlServer(gw, logger.With("component", "grpc"))
	pb.RegisterCovenControlServer(grpcServer, covenService)

	// Register AdminService - PrincipalService if auth enabled, basic otherwise
	if jwtVerifier != nil {
		principalService := admin.NewPrincipalService(sqlStore, jwtVerifier)
		pb.RegisterAdminServiceServer(grpcServer, principalService)
	} else {
		adminService := admin.NewAdminService(sqlStore)
		pb.RegisterAdminServiceServer(grpcServer, adminService)
	}

	// Register ClientService
	clientService := client.NewClientServiceWithRouter(sqlStore, sqlStore, dedupeCache, agentMgr, agentMgr)
	clientService.SetToolApprover(agentMgr)
	clientService.SetBroadcaster(eventBroadcaster)
	pb.RegisterClientServiceServer(grpcServer, clientService)

	// Register PackService for tool pack support
	packService := packs.NewPackServiceServer(gw.packRegistry, gw.packRouter, logger.With("component", "pack-service"))
	pb.RegisterPackServiceServer(grpcServer, packService)

	return clientService
}

// registerHTTPAPIRoutes registers API routes on the mux with or without auth middleware.
func (g *Gateway) registerHTTPAPIRoutes(mux *http.ServeMux, cfg *config.Config, sqlStore *store.SQLiteStore, logger *slog.Logger) error {
	if cfg.Auth.JWTSecret != "" {
		httpVerifier, err := auth.NewJWTVerifier([]byte(cfg.Auth.JWTSecret))
		if err != nil {
			return fmt.Errorf("creating HTTP JWT verifier: %w", err)
		}
		authMiddleware := auth.HTTPAuthMiddleware(sqlStore, sqlStore, httpVerifier)
		adminMiddleware := auth.RequireAdminHTTP()
		mux.Handle("/api/agents", authMiddleware(http.HandlerFunc(g.handleListAgents)))
		mux.Handle("/api/agents/", authMiddleware(http.HandlerFunc(g.handleAgentHistory)))
		mux.Handle("/api/send", authMiddleware(http.HandlerFunc(g.handleSendMessage)))
		mux.Handle("/api/threads/", authMiddleware(http.HandlerFunc(g.handleThreadRoutes)))
		mux.Handle("/api/stats/usage", authMiddleware(http.HandlerFunc(g.handleUsageStats)))
		mux.Handle("/api/tools/approve", authMiddleware(http.HandlerFunc(g.handleToolApproval)))
		mux.Handle("/api/questions/answer", authMiddleware(http.HandlerFunc(g.handleAnswerQuestion)))
		mux.Handle("/api/bindings", authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost || r.Method == http.MethodDelete {
				adminMiddleware(http.HandlerFunc(g.handleBindings)).ServeHTTP(w, r)
			} else {
				g.handleBindings(w, r)
			}
		})))
		logger.Info("HTTP auth middleware enabled")
	} else {
		mux.HandleFunc("/api/agents", g.handleListAgents)
		mux.HandleFunc("/api/agents/", g.handleAgentHistory)
		mux.HandleFunc("/api/send", g.handleSendMessage)
		mux.HandleFunc("/api/bindings", g.handleBindings)
		mux.HandleFunc("/api/threads/", g.handleThreadRoutes)
		mux.HandleFunc("/api/stats/usage", g.handleUsageStats)
		mux.HandleFunc("/api/tools/approve", g.handleToolApproval)
		mux.HandleFunc("/api/questions/answer", g.handleAnswerQuestion)
		logger.Warn("HTTP auth disabled - no jwt_secret configured")
	}
	return nil
}

// New creates a new Gateway instance with the given configuration.
func New(cfg *config.Config, logger *slog.Logger) (*Gateway, error) {
	s, err := initStore(cfg)
	if err != nil {
		return nil, err
	}

	agentMgr := agent.NewManager(logger.With("component", "agent-manager"))
	dedupeCache := dedupe.New(5*time.Minute, 100_000) // TTL 5min, max 100k entries

	sqlStore, ok := s.(*store.SQLiteStore)
	if !ok {
		return nil, errors.New("unexpected store type: expected SQLiteStore")
	}

	grpcResult, err := createGRPCServer(cfg, sqlStore, logger)
	if err != nil {
		return nil, err
	}

	eventBroadcaster := conversation.NewEventBroadcaster(logger.With("component", "broadcaster"))
	convService := conversation.New(sqlStore, agentMgr, logger.With("component", "conversation"), eventBroadcaster)

	packRegistry := packs.NewRegistry(logger.With("component", "pack-registry"))
	packRouter := packs.NewRouter(packs.RouterConfig{
		Registry: packRegistry,
		Logger:   logger.With("component", "pack-router"),
	})
	if err := registerBuiltinPacks(packRegistry, agentMgr, s, sqlStore); err != nil {
		return nil, err
	}

	mcpTokens := mcp.NewTokenStore()
	mcpEndpoint := determineMCPEndpoint(cfg, logger)
	grpcServer := grpcResult.server
	gw := &Gateway{
		config:           cfg,
		agentManager:     agentMgr,
		store:            s,
		conversation:     convService,
		grpcServer:       grpcServer,
		logger:           logger.With("component", "gateway"),
		serverID:         generateServerID(),
		dedupe:           dedupeCache,
		packRegistry:     packRegistry,
		packRouter:       packRouter,
		mcpTokens:        mcpTokens,
		mcpEndpoint:      mcpEndpoint,
		eventBroadcaster: eventBroadcaster,
	}

	// Register gRPC services
	clientService := registerGRPCServices(gw, grpcServer, grpcResult.jwtVerifier, sqlStore, dedupeCache, agentMgr, eventBroadcaster, logger)

	// Create HTTP server for health checks and API
	mux := http.NewServeMux()

	// Health endpoints - no auth required
	mux.HandleFunc("/health", gw.handleHealth)
	mux.HandleFunc("/health/ready", gw.handleReady)

	// API endpoints - auth required if JWT secret is configured
	if err := gw.registerHTTPAPIRoutes(mux, cfg, sqlStore, logger); err != nil {
		return nil, err
	}

	// Register web admin UI routes
	// The admin UI has its own session-based auth (separate from JWT)
	webAdminBaseURL := determineWebAdminBaseURL(cfg, logger)
	webAdminCfg := webadmin.NewConfig{
		Store:        sqlStore,
		Manager:      gw.agentManager,
		Conversation: convService,
		Broadcaster:  eventBroadcaster,
		Registry:     packRegistry,
		Config: webadmin.Config{
			BaseURL: webAdminBaseURL,
		},
		PrincipalStore: sqlStore,
		TokenGenerator: grpcResult.jwtVerifier, // May be nil if auth is disabled
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
	gw.mcpServer = mcpServer
	gw.mcpServer.RegisterRoutes(mux)

	gw.httpServer = &http.Server{
		Addr:              cfg.Server.HTTPAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return gw, nil
}

// setupTCPListeners creates standard TCP listeners for gRPC and HTTP.
func (g *Gateway) setupTCPListeners() (grpcLn, httpLn net.Listener, err error) {
	g.logger.Info("starting gateway",
		"grpc_addr", g.config.Server.GRPCAddr,
		"http_addr", g.config.Server.HTTPAddr,
	)

	grpcLn, err = net.Listen("tcp", g.config.Server.GRPCAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("listening on gRPC address: %w", err)
	}

	httpLn, err = net.Listen("tcp", g.config.Server.HTTPAddr)
	if err != nil {
		_ = grpcLn.Close()
		return nil, nil, fmt.Errorf("listening on HTTP address: %w", err)
	}

	return grpcLn, httpLn, nil
}

// Run starts the gateway servers and blocks until the context is canceled.
// It manages graceful shutdown of both GRPC and HTTP servers.
// Returns nil on graceful shutdown (context canceled), or an error if a server fails.
// warnIgnoredAddresses logs a warning if server addresses are configured but Tailscale is enabled.
func (g *Gateway) warnIgnoredAddresses() {
	if g.config.Server.GRPCAddr != "" || g.config.Server.HTTPAddr != "" {
		g.logger.Warn("server.grpc_addr and server.http_addr are ignored when tailscale is enabled",
			"grpc_addr", g.config.Server.GRPCAddr,
			"http_addr", g.config.Server.HTTPAddr,
		)
	}
}

// setupListeners creates listeners based on configuration (Tailscale or TCP).
func (g *Gateway) setupListeners(ctx context.Context) (grpcLn, httpLn net.Listener, err error) {
	if g.config.Tailscale.Enabled {
		g.warnIgnoredAddresses()
		return g.setupTailscaleListeners(ctx)
	}
	return g.setupTCPListeners()
}

// startServers starts gRPC and HTTP servers in goroutines, returning error channel.
func (g *Gateway) startServers(grpcLn, httpLn net.Listener) chan error {
	errCh := make(chan error, 2)

	go func() {
		g.logger.Info("gRPC server listening", "addr", grpcLn.Addr().String())
		if err := g.grpcServer.Serve(grpcLn); err != nil {
			errCh <- fmt.Errorf("gRPC server: %w", err)
		}
	}()

	go func() {
		g.logger.Info("HTTP server listening", "addr", httpLn.Addr().String())
		if err := g.httpServer.Serve(httpLn); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("HTTP server: %w", err)
		}
	}()

	return errCh
}

// waitForShutdownSignal waits for context cancellation or server error.
func (g *Gateway) waitForShutdownSignal(ctx context.Context, errCh chan error) error {
	select {
	case <-ctx.Done():
		g.logger.Info("context canceled, initiating shutdown")
		return nil
	case err := <-errCh:
		g.logger.Error("server error", "error", err)
		g.drainErrors(errCh)
		return err
	}
}

// drainErrors drains any remaining errors from the channel.
func (g *Gateway) drainErrors(errCh chan error) {
	select {
	case additionalErr := <-errCh:
		g.logger.Error("additional server error", "error", additionalErr)
	default:
	}
}

func (g *Gateway) Run(ctx context.Context) error {
	grpcListener, httpListener, err := g.setupListeners(ctx)
	if err != nil {
		return err
	}

	errCh := g.startServers(grpcListener, httpListener)
	serverErr := g.waitForShutdownSignal(ctx, errCh)

	shutdownErr := g.gracefulShutdown()

	if serverErr != nil {
		return serverErr
	}
	return shutdownErr
}

// gracefulShutdown performs shutdown with a fresh context and timeout.
// Uses context.Background() intentionally since the original context is already canceled.
func (g *Gateway) gracefulShutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return g.Shutdown(ctx)
}

// resolveTailscaleStateDir returns the state directory, using default if not configured.
func resolveTailscaleStateDir(configured string) (string, error) {
	if configured != "" {
		return configured, nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory for tailscale state (set tailscale.state_dir explicitly): %w", err)
	}
	return filepath.Join(homeDir, ".local", "share", "coven-gateway", "tailscale"), nil
}

// resolveTailscaleAuthKey returns the auth key from config or environment.
func resolveTailscaleAuthKey(configured string) (string, error) {
	authKey := configured
	if authKey == "" {
		authKey = os.Getenv("TS_AUTHKEY")
	}
	if authKey == "" {
		return "", errors.New("tailscale auth key required: set auth_key in config or TS_AUTHKEY environment variable (get one at https://login.tailscale.com/admin/settings/keys)")
	}
	return authKey, nil
}

// setupTailscaleListeners creates a tsnet server and returns listeners for gRPC and HTTP.
func (g *Gateway) setupTailscaleListeners(ctx context.Context) (grpcLn, httpLn net.Listener, err error) {
	tsCfg := g.config.Tailscale

	stateDir, err := resolveTailscaleStateDir(tsCfg.StateDir)
	if err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return nil, nil, fmt.Errorf("creating tailscale state dir: %w", err)
	}

	authKey, err := resolveTailscaleAuthKey(tsCfg.AuthKey)
	if err != nil {
		return nil, nil, err
	}

	g.tsnetServer = &tsnet.Server{
		Hostname:  tsCfg.Hostname,
		Dir:       stateDir,
		Ephemeral: tsCfg.Ephemeral,
		AuthKey:   authKey,
	}

	g.logger.Info("starting tailscale node", "hostname", tsCfg.Hostname, "state_dir", stateDir, "ephemeral", tsCfg.Ephemeral)
	status, err := g.tsnetServer.Up(ctx)
	if err != nil {
		_ = g.tsnetServer.Close()
		return nil, nil, fmt.Errorf("starting tailscale: %w", err)
	}

	g.logTailscaleStatus(tsCfg.Hostname, status)
	g.updateMCPEndpointFromStatus(status)

	grpcLn, err = g.tsnetServer.Listen("tcp", ":50051")
	if err != nil {
		_ = g.tsnetServer.Close()
		return nil, nil, fmt.Errorf("listening on tailscale gRPC port: %w", err)
	}

	httpLn, err = g.createTailscaleHTTPListener(tsCfg, grpcLn)
	if err != nil {
		return nil, nil, err
	}
	return grpcLn, httpLn, nil
}

// logTailscaleStatus logs info about the tailscale node status.
func (g *Gateway) logTailscaleStatus(hostname string, status *ipnstate.Status) {
	var tsAddr, dnsName string
	if len(status.TailscaleIPs) > 0 {
		tsAddr = status.TailscaleIPs[0].String()
	} else {
		g.logger.Warn("tailscale node has no IP addresses assigned")
	}
	if status.Self != nil {
		dnsName = status.Self.DNSName
	}
	g.logger.Info("tailscale node ready", "hostname", hostname, "tailscale_ip", tsAddr, "dns_name", dnsName)
}

// updateMCPEndpointFromStatus updates MCP endpoint to use Tailscale DNS name.
func (g *Gateway) updateMCPEndpointFromStatus(status *ipnstate.Status) {
	if status.Self == nil || status.Self.DNSName == "" {
		return
	}
	cleanDNS := strings.TrimSuffix(status.Self.DNSName, ".")
	newEndpoint := "https://" + cleanDNS + "/mcp"
	if newEndpoint != g.mcpEndpoint {
		g.logger.Info("updated MCP endpoint to use Tailscale DNS name", "old", g.mcpEndpoint, "new", newEndpoint)
		g.mcpEndpoint = newEndpoint
	}
}

// createTailscaleHTTPListener creates the appropriate HTTP listener based on config.
func (g *Gateway) createTailscaleHTTPListener(tsCfg config.TailscaleConfig, grpcLn net.Listener) (net.Listener, error) {
	switch {
	case tsCfg.Funnel:
		g.logger.Info("enabling tailscale funnel (public HTTPS) on :443")
		ln, err := g.tsnetServer.ListenFunnel("tcp", ":443")
		if err != nil {
			_ = grpcLn.Close()
			_ = g.tsnetServer.Close()
			return nil, fmt.Errorf("listening on tailscale HTTP port: %w", err)
		}
		return ln, nil
	case tsCfg.HTTPS:
		return g.createTailscaleTLSListener(grpcLn)
	default:
		ln, err := g.tsnetServer.Listen("tcp", ":80")
		if err != nil {
			_ = grpcLn.Close()
			_ = g.tsnetServer.Close()
			return nil, fmt.Errorf("listening on tailscale HTTP port: %w", err)
		}
		return ln, nil
	}
}

// createTailscaleTLSListener creates a TLS listener using Tailscale's auto-provisioned certs.
func (g *Gateway) createTailscaleTLSListener(grpcLn net.Listener) (net.Listener, error) {
	g.logger.Info("enabling HTTPS with Tailscale certs on :443")
	ln, err := g.tsnetServer.Listen("tcp", ":443")
	if err != nil {
		_ = grpcLn.Close()
		_ = g.tsnetServer.Close()
		return nil, fmt.Errorf("listening on tailscale HTTPS port: %w", err)
	}
	lc, err := g.tsnetServer.LocalClient()
	if err != nil {
		_ = ln.Close()
		_ = grpcLn.Close()
		_ = g.tsnetServer.Close()
		return nil, fmt.Errorf("getting tailscale local client: %w", err)
	}
	return tls.NewListener(ln, &tls.Config{
		GetCertificate: lc.GetCertificate,
		MinVersion:     tls.VersionTLS12,
	}), nil
}

// Shutdown gracefully stops all gateway servers and releases resources.
// shutdownGRPCServer gracefully stops the gRPC server or force-stops on context cancel.
func (g *Gateway) shutdownGRPCServer(ctx context.Context) {
	stopped := make(chan struct{})
	go func() {
		g.grpcServer.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-ctx.Done():
		g.grpcServer.Stop()
	}
}

// appendCloseError appends an error with label if err is non-nil.
func appendCloseError(errs []error, label string, err error) []error {
	if err != nil {
		return append(errs, fmt.Errorf("%s: %w", label, err))
	}
	return errs
}

// closeOptionalComponents closes optional components that may be nil.
func (g *Gateway) closeOptionalComponents() {
	if g.dedupe != nil {
		g.dedupe.Close()
	}
	if g.eventBroadcaster != nil {
		g.eventBroadcaster.Close()
	}
	if g.webAdmin != nil {
		g.webAdmin.Close()
	}
	if g.mcpServer != nil {
		g.mcpServer.Close()
	}
	if g.packRouter != nil {
		g.packRouter.Close()
	}
	if g.packRegistry != nil {
		g.packRegistry.Close()
	}
}

func (g *Gateway) Shutdown(ctx context.Context) error {
	g.logger.Info("shutting down gateway")

	var errs []error
	errs = appendCloseError(errs, "HTTP shutdown", g.httpServer.Shutdown(ctx))

	g.shutdownGRPCServer(ctx)

	if g.tsnetServer != nil {
		errs = appendCloseError(errs, "tailscale shutdown", g.tsnetServer.Close())
	}
	errs = appendCloseError(errs, "store close", g.store.Close())

	g.closeOptionalComponents()

	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}
	return nil
}

// handleHealth returns 200 OK if the server is alive.
func (g *Gateway) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

// handleReady returns 200 OK if the server has at least one agent connected.
func (g *Gateway) handleReady(w http.ResponseWriter, r *http.Request) {
	agents := g.agentManager.ListAgents()
	if len(agents) == 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("no agents connected"))
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, "ready (%d agents)", len(agents))
}

// generateServerID creates a unique identifier for this gateway instance.
func generateServerID() string {
	return fmt.Sprintf("coven-gateway-%d", time.Now().UnixNano()%1000000)
}
