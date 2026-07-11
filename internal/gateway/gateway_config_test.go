// ABOUTME: Tests for gateway.go config/lifecycle functions and pure helpers.
// ABOUTME: Covers Tailscale config resolution, endpoint detection, drain/append helpers, and lifecycle branches.

package gateway

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"

	"tailscale.com/ipn/ipnstate"

	"github.com/2389/coven-gateway/internal/config"
	"github.com/2389/coven-gateway/internal/store"
)

// =============================================================================
// resolveTailscaleAuthKey
// =============================================================================

func TestResolveTailscaleAuthKey_FromConfig(t *testing.T) {
	key, err := resolveTailscaleAuthKey("ts-key-abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "ts-key-abc123" {
		t.Errorf("key = %q, want %q", key, "ts-key-abc123")
	}
}

func TestResolveTailscaleAuthKey_FromEnv(t *testing.T) {
	t.Setenv("TS_AUTHKEY", "ts-env-key-xyz")
	key, err := resolveTailscaleAuthKey("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "ts-env-key-xyz" {
		t.Errorf("key = %q, want %q", key, "ts-env-key-xyz")
	}
}

func TestResolveTailscaleAuthKey_Empty(t *testing.T) {
	t.Setenv("TS_AUTHKEY", "")
	_, err := resolveTailscaleAuthKey("")
	if err == nil {
		t.Error("expected error for empty auth key")
	}
}

// =============================================================================
// resolveTailscaleStateDir
// =============================================================================

func TestResolveTailscaleStateDir_Configured(t *testing.T) {
	dir, err := resolveTailscaleStateDir("/custom/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir != "/custom/path" {
		t.Errorf("dir = %q, want /custom/path", dir)
	}
}

func TestResolveTailscaleStateDir_Default(t *testing.T) {
	dir, err := resolveTailscaleStateDir("")
	if err != nil {
		t.Fatalf("unexpected error getting default dir: %v", err)
	}
	if !strings.Contains(dir, "coven-gateway") {
		t.Errorf("expected default path to contain 'coven-gateway', got %q", dir)
	}
}

// =============================================================================
// determineMCPEndpoint
// =============================================================================

func TestDetermineMCPEndpoint_EnvCOVEN_MCP_ENDPOINT(t *testing.T) {
	t.Setenv("COVEN_MCP_ENDPOINT", "https://custom-mcp.example.com/mcp")
	cfg := &config.Config{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	got := determineMCPEndpoint(cfg, logger)
	if got != "https://custom-mcp.example.com/mcp" {
		t.Errorf("got %q, want %q", got, "https://custom-mcp.example.com/mcp")
	}
}

func TestDetermineMCPEndpoint_EnvCOVEN_GATEWAY_URL(t *testing.T) {
	t.Setenv("COVEN_MCP_ENDPOINT", "")
	t.Setenv("COVEN_GATEWAY_URL", "https://gateway.example.com")
	cfg := &config.Config{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	got := determineMCPEndpoint(cfg, logger)
	if got != "https://gateway.example.com/mcp" {
		t.Errorf("got %q, want %q", got, "https://gateway.example.com/mcp")
	}
}

func TestDetermineMCPEndpoint_Tailscale(t *testing.T) {
	t.Setenv("COVEN_MCP_ENDPOINT", "")
	t.Setenv("COVEN_GATEWAY_URL", "")
	cfg := &config.Config{
		Tailscale: config.TailscaleConfig{
			Enabled:  true,
			Hostname: "my-host.ts.net",
		},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	got := determineMCPEndpoint(cfg, logger)
	if !strings.HasPrefix(got, "https://") || !strings.Contains(got, "my-host.ts.net") {
		t.Errorf("got %q, want https://my-host.ts.net/mcp", got)
	}
}

func TestDetermineMCPEndpoint_DefaultHTTP(t *testing.T) {
	t.Setenv("COVEN_MCP_ENDPOINT", "")
	t.Setenv("COVEN_GATEWAY_URL", "")
	cfg := &config.Config{
		Server: config.ServerConfig{HTTPAddr: "localhost:8080"},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	got := determineMCPEndpoint(cfg, logger)
	if got != "http://localhost:8080/mcp" {
		t.Errorf("got %q, want http://localhost:8080/mcp", got)
	}
}

// =============================================================================
// determineWebAdminBaseURL
// =============================================================================

func TestDetermineWebAdminBaseURL_FromConfig(t *testing.T) {
	cfg := &config.Config{WebAdmin: config.WebAdminConfig{BaseURL: "https://admin.example.com"}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	got := determineWebAdminBaseURL(cfg, logger)
	if got != "https://admin.example.com" {
		t.Errorf("got %q, want https://admin.example.com", got)
	}
}

func TestDetermineWebAdminBaseURL_FromEnv(t *testing.T) {
	t.Setenv("COVEN_GATEWAY_URL", "https://env.example.com")
	cfg := &config.Config{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	got := determineWebAdminBaseURL(cfg, logger)
	if got != "https://env.example.com" {
		t.Errorf("got %q, want https://env.example.com", got)
	}
}

func TestDetermineWebAdminBaseURL_NoTailscale(t *testing.T) {
	t.Setenv("COVEN_GATEWAY_URL", "")
	cfg := &config.Config{
		Server: config.ServerConfig{HTTPAddr: "localhost:9090"},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	got := determineWebAdminBaseURL(cfg, logger)
	if got != "http://localhost:9090" {
		t.Errorf("got %q, want http://localhost:9090", got)
	}
}

func TestDetermineWebAdminBaseURL_TailscaleHTTPS(t *testing.T) {
	t.Setenv("COVEN_GATEWAY_URL", "")
	cfg := &config.Config{
		Tailscale: config.TailscaleConfig{
			Enabled:  true,
			Hostname: "gw.ts.net",
			HTTPS:    true,
		},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	got := determineWebAdminBaseURL(cfg, logger)
	if !strings.HasPrefix(got, "https://") {
		t.Errorf("got %q, want https:// prefix", got)
	}
}

func TestDetermineWebAdminBaseURL_TailscaleHTTP(t *testing.T) {
	t.Setenv("COVEN_GATEWAY_URL", "")
	cfg := &config.Config{
		Tailscale: config.TailscaleConfig{
			Enabled:  true,
			Hostname: "gw.ts.net",
			HTTPS:    false,
			Funnel:   false,
		},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	got := determineWebAdminBaseURL(cfg, logger)
	if !strings.HasPrefix(got, "http://") {
		t.Errorf("got %q, want http:// prefix", got)
	}
}

// =============================================================================
// updateMCPEndpointFromStatus
// =============================================================================

func TestUpdateMCPEndpointFromStatus_NilSelf(t *testing.T) {
	gw := newTestGateway(t)
	gw.mcpEndpoint = "http://original/mcp"
	status := &ipnstate.Status{Self: nil}
	gw.updateMCPEndpointFromStatus(status)
	if gw.mcpEndpoint != "http://original/mcp" {
		t.Errorf("endpoint changed when Self is nil: %q", gw.mcpEndpoint)
	}
}

func TestUpdateMCPEndpointFromStatus_EmptyDNS(t *testing.T) {
	gw := newTestGateway(t)
	gw.mcpEndpoint = "http://original/mcp"
	status := &ipnstate.Status{Self: &ipnstate.PeerStatus{DNSName: ""}}
	gw.updateMCPEndpointFromStatus(status)
	if gw.mcpEndpoint != "http://original/mcp" {
		t.Errorf("endpoint changed when DNSName is empty: %q", gw.mcpEndpoint)
	}
}

func TestUpdateMCPEndpointFromStatus_ValidDNS(t *testing.T) {
	gw := newTestGateway(t)
	gw.mcpEndpoint = "http://old/mcp"
	status := &ipnstate.Status{Self: &ipnstate.PeerStatus{DNSName: "myhost.ts.net."}}
	gw.updateMCPEndpointFromStatus(status)
	want := "https://myhost.ts.net/mcp"
	if gw.mcpEndpoint != want {
		t.Errorf("endpoint = %q, want %q", gw.mcpEndpoint, want)
	}
}

func TestUpdateMCPEndpointFromStatus_SameEndpointNoChange(t *testing.T) {
	gw := newTestGateway(t)
	gw.mcpEndpoint = "https://myhost.ts.net/mcp"
	status := &ipnstate.Status{Self: &ipnstate.PeerStatus{DNSName: "myhost.ts.net."}}
	gw.updateMCPEndpointFromStatus(status)
	if gw.mcpEndpoint != "https://myhost.ts.net/mcp" {
		t.Errorf("endpoint = %q, want unchanged", gw.mcpEndpoint)
	}
}

// =============================================================================
// logTailscaleStatus
// =============================================================================

func TestLogTailscaleStatus_NoIPs(t *testing.T) {
	gw := newTestGateway(t)
	// Should not panic on empty IP list
	status := &ipnstate.Status{}
	gw.logTailscaleStatus("test-host", status)
}

func TestLogTailscaleStatus_WithIPs(t *testing.T) {
	gw := newTestGateway(t)
	ip := netip.MustParseAddr("100.64.0.1")
	status := &ipnstate.Status{
		TailscaleIPs: []netip.Addr{ip},
		Self:         &ipnstate.PeerStatus{DNSName: "myhost.ts.net."},
	}
	gw.logTailscaleStatus("myhost", status)
}

func TestLogTailscaleStatus_NilSelf(t *testing.T) {
	gw := newTestGateway(t)
	ip := netip.MustParseAddr("100.64.0.2")
	status := &ipnstate.Status{
		TailscaleIPs: []netip.Addr{ip},
		Self:         nil,
	}
	gw.logTailscaleStatus("myhost", status)
}

// =============================================================================
// warnIgnoredAddresses
// =============================================================================

func TestWarnIgnoredAddresses_WithAddresses(t *testing.T) {
	gw := newTestGateway(t)
	// Should log a warning without panicking
	gw.config.Server.GRPCAddr = "localhost:50051"
	gw.config.Server.HTTPAddr = "localhost:8080"
	gw.warnIgnoredAddresses()
}

func TestWarnIgnoredAddresses_EmptyAddresses(t *testing.T) {
	gw := newTestGateway(t)
	gw.config.Server.GRPCAddr = ""
	gw.config.Server.HTTPAddr = ""
	gw.warnIgnoredAddresses()
}

// =============================================================================
// drainErrors
// =============================================================================

func TestDrainErrors_WithError(t *testing.T) {
	gw := newTestGateway(t)
	errCh := make(chan error, 2)
	errCh <- errors.New("error1")
	gw.drainErrors(errCh)
	// Channel should now be drained (one error consumed)
	if len(errCh) != 0 {
		t.Errorf("expected channel to be drained, %d items remain", len(errCh))
	}
}

func TestDrainErrors_EmptyChannel(t *testing.T) {
	gw := newTestGateway(t)
	errCh := make(chan error, 2)
	gw.drainErrors(errCh)
	// Should return immediately without blocking
}

// =============================================================================
// appendCloseError
// =============================================================================

func TestAppendCloseError_NonNilError(t *testing.T) {
	errs := appendCloseError(nil, "store", errors.New("close failed"))
	if len(errs) != 1 {
		t.Errorf("expected 1 error, got %d", len(errs))
	}
	if !strings.Contains(errs[0].Error(), "store") {
		t.Errorf("expected label 'store' in error, got: %v", errs[0])
	}
}

func TestAppendCloseError_NilError(t *testing.T) {
	errs := appendCloseError(nil, "store", nil)
	if len(errs) != 0 {
		t.Errorf("expected 0 errors, got %d", len(errs))
	}
}

func TestAppendCloseError_PreservesExisting(t *testing.T) {
	existing := []error{errors.New("first")}
	errs := appendCloseError(existing, "second", errors.New("second-err"))
	if len(errs) != 2 {
		t.Errorf("expected 2 errors, got %d", len(errs))
	}
}

// =============================================================================
// initStore
// =============================================================================

func TestInitStore_InMemory(t *testing.T) {
	cfg := &config.Config{Database: config.DatabaseConfig{Path: ":memory:"}}
	s, err := initStore(cfg)
	if err != nil {
		t.Fatalf("initStore in-memory failed: %v", err)
	}
	s.Close()
}

func TestInitStore_TempPath(t *testing.T) {
	path := t.TempDir() + "/test.db"
	cfg := &config.Config{Database: config.DatabaseConfig{Path: path}}
	s, err := initStore(cfg)
	if err != nil {
		t.Fatalf("initStore with path failed: %v", err)
	}
	s.Close()
}

func TestInitStore_EnvOverride(t *testing.T) {
	path := t.TempDir() + "/env-override.db"
	t.Setenv("COVEN_DB_PATH", path)
	cfg := &config.Config{Database: config.DatabaseConfig{Path: ":memory:"}}
	s, err := initStore(cfg)
	if err != nil {
		t.Fatalf("initStore with env override failed: %v", err)
	}
	s.Close()
}

// =============================================================================
// createGRPCServer
// =============================================================================

func TestCreateGRPCServer_NoAuth(t *testing.T) {
	cfg := &config.Config{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	sqlStore, err := newTestSQLiteStore(t)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer sqlStore.Close()

	result, err := createGRPCServer(cfg, sqlStore, logger)
	if err != nil {
		t.Fatalf("createGRPCServer failed: %v", err)
	}
	if result.server == nil {
		t.Error("expected non-nil server")
	}
	if result.jwtVerifier != nil {
		t.Error("expected nil jwtVerifier for no-auth mode")
	}
}

func TestCreateAuthenticatedGRPCServer_ValidSecret(t *testing.T) {
	cfg := &config.Config{Auth: config.AuthConfig{JWTSecret: "test-secret-that-is-at-least-32-chars-long"}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	sqlStore, err := newTestSQLiteStore(t)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer sqlStore.Close()

	result, err := createAuthenticatedGRPCServer(cfg, sqlStore, logger)
	if err != nil {
		t.Fatalf("createAuthenticatedGRPCServer failed: %v", err)
	}
	if result.server == nil {
		t.Error("expected non-nil server")
	}
	if result.jwtVerifier == nil {
		t.Error("expected non-nil jwtVerifier")
	}
}

// =============================================================================
// registerHTTPAPIRoutes
// =============================================================================

func TestRegisterHTTPAPIRoutes_NoAuth(t *testing.T) {
	gw := newTestGateway(t)
	cfg := &config.Config{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sqlStore, err := newTestSQLiteStore(t)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer sqlStore.Close()

	mux := http.NewServeMux()
	if err := gw.registerHTTPAPIRoutes(mux, cfg, sqlStore, logger); err != nil {
		t.Fatalf("registerHTTPAPIRoutes failed: %v", err)
	}
}

func TestRegisterHTTPAPIRoutes_WithAuth(t *testing.T) {
	gw := newTestGateway(t)
	cfg := &config.Config{Auth: config.AuthConfig{JWTSecret: "test-secret-that-is-at-least-32-chars-long"}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sqlStore, err := newTestSQLiteStore(t)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer sqlStore.Close()

	mux := http.NewServeMux()
	if err := gw.registerHTTPAPIRoutes(mux, cfg, sqlStore, logger); err != nil {
		t.Fatalf("registerHTTPAPIRoutes with auth failed: %v", err)
	}
}

// =============================================================================
// setupTCPListeners
// =============================================================================

func TestSetupTCPListeners_InvalidHTTPAddr(t *testing.T) {
	gw := newTestGateway(t)
	// Bind gRPC to a valid port but give an invalid HTTP address
	grpcLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create grpc listener: %v", err)
	}
	grpcAddr := grpcLn.Addr().String()
	grpcLn.Close()

	gw.config.Server.GRPCAddr = grpcAddr
	gw.config.Server.HTTPAddr = "not-a-valid-addr:99999"
	_, _, err = gw.setupTCPListeners()
	if err == nil {
		t.Error("expected error for invalid HTTP address")
	}
}

func TestSetupTCPListeners_Success(t *testing.T) {
	gw := newTestGateway(t)
	gw.config.Server.GRPCAddr = "127.0.0.1:0"
	gw.config.Server.HTTPAddr = "127.0.0.1:0"
	grpcLn, httpLn, err := gw.setupTCPListeners()
	if err != nil {
		t.Fatalf("setupTCPListeners: %v", err)
	}
	defer grpcLn.Close()
	defer httpLn.Close()
	if grpcLn.Addr() == nil {
		t.Error("expected non-nil gRPC listener address")
	}
	if httpLn.Addr() == nil {
		t.Error("expected non-nil HTTP listener address")
	}
}

func TestSetupListeners_NonTailscale(t *testing.T) {
	gw := newTestGateway(t)
	gw.config.Server.GRPCAddr = "127.0.0.1:0"
	gw.config.Server.HTTPAddr = "127.0.0.1:0"
	gw.config.Tailscale.Enabled = false
	grpcLn, httpLn, err := gw.setupListeners(context.Background())
	if err != nil {
		t.Fatalf("setupListeners: %v", err)
	}
	defer grpcLn.Close()
	defer httpLn.Close()
}

// =============================================================================
// waitForShutdownSignal
// =============================================================================

func TestWaitForShutdownSignal_ContextCancel(t *testing.T) {
	gw := newTestGateway(t)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	cancel()
	err := gw.waitForShutdownSignal(ctx, errCh)
	if err != nil {
		t.Errorf("expected nil on context cancel, got %v", err)
	}
}

func TestWaitForShutdownSignal_ErrorFromChannel(t *testing.T) {
	gw := newTestGateway(t)
	ctx := context.Background()
	errCh := make(chan error, 2)
	errCh <- errors.New("server died")

	err := gw.waitForShutdownSignal(ctx, errCh)
	if err == nil {
		t.Error("expected error from channel, got nil")
	}
}

// =============================================================================
// Shutdown arms
// =============================================================================

func TestShutdown_AlreadyShutdown(t *testing.T) {
	gw := newTestGateway(t)

	ctx := context.Background()
	// First shutdown
	if err := gw.Shutdown(ctx); err != nil {
		t.Fatalf("first shutdown failed: %v", err)
	}
	// Second shutdown should not panic (idempotent-ish)
	// The grpc server may error but it should not crash
	_ = gw.Shutdown(ctx)
}

func TestShutdown_WithTimeout(t *testing.T) {
	gw := newTestGateway(t)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	// Should complete without hanging
	_ = gw.Shutdown(ctx)
}

// =============================================================================
// Helper: newTestSQLiteStore creates an in-memory SQLiteStore for tests.
// =============================================================================

func newTestSQLiteStore(t *testing.T) (*store.SQLiteStore, error) {
	t.Helper()
	s, err := store.NewSQLiteStore(":memory:")
	if err != nil {
		return nil, err
	}
	return s, nil
}
