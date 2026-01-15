// ABOUTME: Tests for Gateway orchestrator and FoldControl gRPC service
// ABOUTME: Uses real gRPC streaming to test bidirectional agent communication

package gateway

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/2389/fold-gateway/internal/config"
	pb "github.com/2389/fold-gateway/proto/fold"
)

// testConfig creates a minimal config for testing with available ports
func testConfig(t *testing.T) *config.Config {
	t.Helper()

	// Find available ports
	grpcListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find available gRPC port: %v", err)
	}
	grpcAddr := grpcListener.Addr().String()
	grpcListener.Close()

	httpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find available HTTP port: %v", err)
	}
	httpAddr := httpListener.Addr().String()
	httpListener.Close()

	return &config.Config{
		Server: config.ServerConfig{
			GRPCAddr: grpcAddr,
			HTTPAddr: httpAddr,
		},
		Database: config.DatabaseConfig{
			Path: ":memory:",
		},
		Routing: config.RoutingConfig{
			Strategy: "round_robin",
		},
		Agents: config.AgentsConfig{
			HeartbeatInterval:    30 * time.Second,
			HeartbeatTimeout:     90 * time.Second,
			ReconnectGracePeriod: 5 * time.Minute,
		},
	}
}

// testLogger creates a silent logger for tests
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestGatewayNew(t *testing.T) {
	cfg := testConfig(t)
	logger := testLogger()

	gw, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer gw.Shutdown(context.Background())

	if gw.config != cfg {
		t.Error("gateway config mismatch")
	}

	if gw.agentManager == nil {
		t.Error("agentManager should not be nil")
	}

	if gw.store == nil {
		t.Error("store should not be nil")
	}
}

func TestGatewayRunAndShutdown(t *testing.T) {
	cfg := testConfig(t)
	logger := testLogger()

	gw, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Run gateway in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- gw.Run(ctx)
	}()

	// Give it time to start
	time.Sleep(100 * time.Millisecond)

	// Shutdown via context cancel
	cancel()

	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Errorf("Run() returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("gateway did not shutdown in time")
	}
}

func TestHealthEndpoint(t *testing.T) {
	cfg := testConfig(t)
	logger := testLogger()

	gw, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run gateway
	go func() {
		_ = gw.Run(ctx)
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Check health endpoint
	resp, err := http.Get("http://" + cfg.Server.HTTPAddr + "/health")
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("health status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestReadyEndpoint_NoAgents(t *testing.T) {
	cfg := testConfig(t)
	logger := testLogger()

	gw, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = gw.Run(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	// With no agents, ready should return 503
	resp, err := http.Get("http://" + cfg.Server.HTTPAddr + "/health/ready")
	if err != nil {
		t.Fatalf("ready request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("ready status = %d, want %d (no agents)", resp.StatusCode, http.StatusServiceUnavailable)
	}
}

func TestAgentStreamRegistration(t *testing.T) {
	cfg := testConfig(t)
	logger := testLogger()

	gw, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = gw.Run(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	// Connect as an agent
	conn, err := grpc.NewClient(
		cfg.Server.GRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewFoldControlClient(conn)

	stream, err := client.AgentStream(ctx)
	if err != nil {
		t.Fatalf("AgentStream() failed: %v", err)
	}

	// Send registration
	agentID := uuid.New().String()
	err = stream.Send(&pb.AgentMessage{
		Payload: &pb.AgentMessage_Register{
			Register: &pb.RegisterAgent{
				AgentId:      agentID,
				Name:         "test-agent",
				Capabilities: []string{"chat", "code"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Send() registration failed: %v", err)
	}

	// Receive welcome
	msg, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv() failed: %v", err)
	}

	welcome := msg.GetWelcome()
	if welcome == nil {
		t.Fatal("expected Welcome message, got something else")
	}

	if welcome.GetAgentId() != agentID {
		t.Errorf("welcome.agent_id = %s, want %s", welcome.GetAgentId(), agentID)
	}

	if welcome.GetServerId() == "" {
		t.Error("welcome.server_id should not be empty")
	}

	// Agent should now be listed
	agents := gw.agentManager.ListAgents()
	if len(agents) != 1 {
		t.Errorf("agent count = %d, want 1", len(agents))
	}

	// Verify ready endpoint now returns 200
	resp, err := http.Get("http://" + cfg.Server.HTTPAddr + "/health/ready")
	if err != nil {
		t.Fatalf("ready request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("ready status = %d, want %d (with agent)", resp.StatusCode, http.StatusOK)
	}
}

func TestAgentStreamHeartbeat(t *testing.T) {
	cfg := testConfig(t)
	logger := testLogger()

	gw, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = gw.Run(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	conn, err := grpc.NewClient(
		cfg.Server.GRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewFoldControlClient(conn)
	stream, err := client.AgentStream(ctx)
	if err != nil {
		t.Fatalf("AgentStream() failed: %v", err)
	}

	// Register agent
	agentID := uuid.New().String()
	err = stream.Send(&pb.AgentMessage{
		Payload: &pb.AgentMessage_Register{
			Register: &pb.RegisterAgent{
				AgentId: agentID,
				Name:    "heartbeat-test",
			},
		},
	})
	if err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	// Receive welcome
	_, err = stream.Recv()
	if err != nil {
		t.Fatalf("Recv() welcome failed: %v", err)
	}

	// Send heartbeat
	err = stream.Send(&pb.AgentMessage{
		Payload: &pb.AgentMessage_Heartbeat{
			Heartbeat: &pb.Heartbeat{
				TimestampMs: time.Now().UnixMilli(),
			},
		},
	})
	if err != nil {
		t.Fatalf("heartbeat send failed: %v", err)
	}

	// Heartbeat should be accepted without error (server just logs)
	// Agent should still be registered
	time.Sleep(50 * time.Millisecond)

	agents := gw.agentManager.ListAgents()
	if len(agents) != 1 {
		t.Errorf("agent count after heartbeat = %d, want 1", len(agents))
	}
}

func TestAgentStreamDisconnect(t *testing.T) {
	cfg := testConfig(t)
	logger := testLogger()

	gw, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = gw.Run(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	conn, err := grpc.NewClient(
		cfg.Server.GRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	client := pb.NewFoldControlClient(conn)

	agentCtx, agentCancel := context.WithCancel(ctx)
	stream, err := client.AgentStream(agentCtx)
	if err != nil {
		t.Fatalf("AgentStream() failed: %v", err)
	}

	// Register agent
	agentID := uuid.New().String()
	err = stream.Send(&pb.AgentMessage{
		Payload: &pb.AgentMessage_Register{
			Register: &pb.RegisterAgent{
				AgentId: agentID,
				Name:    "disconnect-test",
			},
		},
	})
	if err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	_, err = stream.Recv()
	if err != nil {
		t.Fatalf("Recv() welcome failed: %v", err)
	}

	// Verify agent is registered
	if len(gw.agentManager.ListAgents()) != 1 {
		t.Error("agent should be registered")
	}

	// Disconnect by cancelling context and closing connection
	agentCancel()
	conn.Close()

	// Wait for server to detect disconnect
	time.Sleep(200 * time.Millisecond)

	// Agent should be unregistered
	if len(gw.agentManager.ListAgents()) != 0 {
		t.Error("agent should be unregistered after disconnect")
	}
}

func TestAgentStream_NoRegistrationFirst(t *testing.T) {
	cfg := testConfig(t)
	logger := testLogger()

	gw, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = gw.Run(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	conn, err := grpc.NewClient(
		cfg.Server.GRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewFoldControlClient(conn)
	stream, err := client.AgentStream(ctx)
	if err != nil {
		t.Fatalf("AgentStream() failed: %v", err)
	}

	// Send heartbeat without registering first (invalid)
	err = stream.Send(&pb.AgentMessage{
		Payload: &pb.AgentMessage_Heartbeat{
			Heartbeat: &pb.Heartbeat{
				TimestampMs: time.Now().UnixMilli(),
			},
		},
	})
	if err != nil {
		t.Fatalf("Send() failed: %v", err)
	}

	// Server should close stream with error
	_, err = stream.Recv()
	if err == nil {
		t.Error("expected error when sending message before registration")
	}
}

func TestAgentStream_DuplicateRegistration(t *testing.T) {
	cfg := testConfig(t)
	logger := testLogger()

	gw, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = gw.Run(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	conn1, err := grpc.NewClient(
		cfg.Server.GRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to connect conn1: %v", err)
	}
	defer conn1.Close()

	conn2, err := grpc.NewClient(
		cfg.Server.GRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to connect conn2: %v", err)
	}
	defer conn2.Close()

	client1 := pb.NewFoldControlClient(conn1)
	client2 := pb.NewFoldControlClient(conn2)

	// First agent registers
	stream1, err := client1.AgentStream(ctx)
	if err != nil {
		t.Fatalf("AgentStream() 1 failed: %v", err)
	}

	agentID := uuid.New().String()
	err = stream1.Send(&pb.AgentMessage{
		Payload: &pb.AgentMessage_Register{
			Register: &pb.RegisterAgent{
				AgentId: agentID,
				Name:    "first-agent",
			},
		},
	})
	if err != nil {
		t.Fatalf("registration 1 failed: %v", err)
	}

	_, err = stream1.Recv()
	if err != nil {
		t.Fatalf("welcome 1 failed: %v", err)
	}

	// Second agent tries to register with same ID
	stream2, err := client2.AgentStream(ctx)
	if err != nil {
		t.Fatalf("AgentStream() 2 failed: %v", err)
	}

	err = stream2.Send(&pb.AgentMessage{
		Payload: &pb.AgentMessage_Register{
			Register: &pb.RegisterAgent{
				AgentId: agentID, // same ID
				Name:    "second-agent",
			},
		},
	})
	if err != nil {
		t.Fatalf("registration 2 failed: %v", err)
	}

	// Second stream should receive error
	_, err = stream2.Recv()
	if err == nil {
		t.Error("expected error for duplicate agent ID")
	}
}
