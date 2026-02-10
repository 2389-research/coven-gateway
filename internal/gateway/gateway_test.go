// ABOUTME: Tests for Gateway orchestrator and CovenControl gRPC service
// ABOUTME: Uses real gRPC streaming to test bidirectional agent communication

package gateway

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/2389/coven-gateway/internal/agent"
	"github.com/2389/coven-gateway/internal/config"
	pb "github.com/2389/coven-gateway/proto/coven"
)

// testConfig creates a minimal config for testing with available ports.
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
		Agents: config.AgentsConfig{
			HeartbeatInterval:    30 * time.Second,
			HeartbeatTimeout:     90 * time.Second,
			ReconnectGracePeriod: 5 * time.Minute,
		},
	}
}

// testLogger creates a silent logger for tests.
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
		if err != nil && !errors.Is(err, context.Canceled) {
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

	ctx := t.Context()

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

	ctx := t.Context()

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

	ctx := t.Context()

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

	client := pb.NewCovenControlClient(conn)

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

	ctx := t.Context()

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

	client := pb.NewCovenControlClient(conn)
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

	ctx := t.Context()

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

	client := pb.NewCovenControlClient(conn)

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

	// Disconnect by canceling context and closing connection
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

	ctx := t.Context()

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

	client := pb.NewCovenControlClient(conn)
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

	ctx := t.Context()

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

	client1 := pb.NewCovenControlClient(conn1)
	client2 := pb.NewCovenControlClient(conn2)

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

// TestFullMessageRoundTrip tests the complete flow:
// 1. Agent connects and registers
// 2. Gateway sends message to agent
// 3. Agent responds with streaming events
// 4. Gateway receives and transforms responses
// This is the critical integration test that catches cross-layer issues.
func TestFullMessageRoundTrip(t *testing.T) {
	cfg := testConfig(t)
	logger := testLogger()

	gw, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx := t.Context()

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

	client := pb.NewCovenControlClient(conn)
	stream, err := client.AgentStream(ctx)
	if err != nil {
		t.Fatalf("AgentStream() failed: %v", err)
	}

	// Register agent
	agentID := uuid.New().String()
	err = stream.Send(&pb.AgentMessage{
		Payload: &pb.AgentMessage_Register{
			Register: &pb.RegisterAgent{
				AgentId:      agentID,
				Name:         "roundtrip-test-agent",
				Capabilities: []string{"chat"},
			},
		},
	})
	if err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	// Receive welcome
	welcomeMsg, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv() welcome failed: %v", err)
	}
	if welcomeMsg.GetWelcome() == nil {
		t.Fatal("expected Welcome message")
	}

	// Start a goroutine to simulate agent behavior:
	// Wait for SendMessage, then respond with streaming events
	agentDone := make(chan struct{})
	go func() {
		defer close(agentDone)

		// Wait for SendMessage from gateway
		msg, err := stream.Recv()
		if err != nil {
			t.Errorf("agent recv failed: %v", err)
			return
		}

		sendMsg := msg.GetSendMessage()
		if sendMsg == nil {
			t.Error("expected SendMessage, got something else")
			return
		}

		requestID := sendMsg.GetRequestId()
		if requestID == "" {
			t.Error("request_id should not be empty")
			return
		}

		// Verify message content
		if sendMsg.GetContent() != "Hello from test!" {
			t.Errorf("expected 'Hello from test!', got '%s'", sendMsg.GetContent())
		}

		// Respond with streaming events: Thinking → Text → Text → Done
		responses := []*pb.AgentMessage{
			{
				Payload: &pb.AgentMessage_Response{
					Response: &pb.MessageResponse{
						RequestId: requestID,
						Event: &pb.MessageResponse_Thinking{
							Thinking: "Let me think about this...",
						},
					},
				},
			},
			{
				Payload: &pb.AgentMessage_Response{
					Response: &pb.MessageResponse{
						RequestId: requestID,
						Event: &pb.MessageResponse_Text{
							Text: "Hello ",
						},
					},
				},
			},
			{
				Payload: &pb.AgentMessage_Response{
					Response: &pb.MessageResponse{
						RequestId: requestID,
						Event: &pb.MessageResponse_Text{
							Text: "back!",
						},
					},
				},
			},
			{
				Payload: &pb.AgentMessage_Response{
					Response: &pb.MessageResponse{
						RequestId: requestID,
						Event: &pb.MessageResponse_Done{
							Done: &pb.Done{
								FullResponse: "Hello back!",
							},
						},
					},
				},
			},
		}

		for _, resp := range responses {
			if err := stream.Send(resp); err != nil {
				t.Errorf("agent send response failed: %v", err)
				return
			}
			// Small delay to simulate streaming
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// Give agent goroutine time to start listening
	time.Sleep(50 * time.Millisecond)

	// Send message through the manager
	respChan, err := gw.agentManager.SendMessage(ctx, &agent.SendRequest{
		ThreadID: "thread-roundtrip",
		Sender:   "test@example.com",
		Content:  "Hello from test!",
		AgentID:  agentID,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	// Collect all responses
	var responses []*agent.Response
	timeout := time.After(5 * time.Second)

collectLoop:
	for {
		select {
		case resp, ok := <-respChan:
			if !ok {
				break collectLoop
			}
			responses = append(responses, resp)
			if resp.Done {
				break collectLoop
			}
		case <-timeout:
			t.Fatal("timeout waiting for responses")
		}
	}

	// Wait for agent goroutine to complete
	select {
	case <-agentDone:
	case <-time.After(2 * time.Second):
		t.Error("agent goroutine did not complete")
	}

	// Verify we received all expected events
	if len(responses) < 4 {
		t.Fatalf("expected at least 4 responses, got %d", len(responses))
	}

	// Check event sequence
	expectedEvents := []agent.ResponseEvent{
		agent.EventThinking,
		agent.EventText,
		agent.EventText,
		agent.EventDone,
	}

	for i, expected := range expectedEvents {
		if i >= len(responses) {
			t.Errorf("missing response at index %d", i)
			continue
		}
		if responses[i].Event != expected {
			t.Errorf("response[%d].Event = %v, want %v", i, responses[i].Event, expected)
		}
	}

	// Verify content
	if responses[0].Text != "Let me think about this..." {
		t.Errorf("thinking text = %q, want %q", responses[0].Text, "Let me think about this...")
	}
	if responses[1].Text != "Hello " {
		t.Errorf("text[0] = %q, want %q", responses[1].Text, "Hello ")
	}
	if responses[2].Text != "back!" {
		t.Errorf("text[1] = %q, want %q", responses[2].Text, "back!")
	}
	if responses[3].Text != "Hello back!" {
		t.Errorf("done full_response = %q, want %q", responses[3].Text, "Hello back!")
	}
	if !responses[3].Done {
		t.Error("last response should have Done=true")
	}
}

// TestMessageRoundTrip_ToolUse tests tool use/result flow through the system.
func TestMessageRoundTrip_ToolUse(t *testing.T) {
	cfg := testConfig(t)
	logger := testLogger()

	gw, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx := t.Context()

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

	client := pb.NewCovenControlClient(conn)
	stream, err := client.AgentStream(ctx)
	if err != nil {
		t.Fatalf("AgentStream() failed: %v", err)
	}

	agentID := uuid.New().String()
	err = stream.Send(&pb.AgentMessage{
		Payload: &pb.AgentMessage_Register{
			Register: &pb.RegisterAgent{
				AgentId: agentID,
				Name:    "tool-test-agent",
			},
		},
	})
	if err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	_, err = stream.Recv() // welcome
	if err != nil {
		t.Fatalf("welcome failed: %v", err)
	}

	// Agent goroutine: respond with tool_use → tool_result → done
	agentDone := make(chan struct{})
	go func() {
		defer close(agentDone)

		msg, err := stream.Recv()
		if err != nil {
			t.Errorf("agent recv failed: %v", err)
			return
		}

		requestID := msg.GetSendMessage().GetRequestId()

		responses := []*pb.AgentMessage{
			{
				Payload: &pb.AgentMessage_Response{
					Response: &pb.MessageResponse{
						RequestId: requestID,
						Event: &pb.MessageResponse_ToolUse{
							ToolUse: &pb.ToolUse{
								Id:        "tool-123",
								Name:      "read_file",
								InputJson: `{"path": "/tmp/test.txt"}`,
							},
						},
					},
				},
			},
			{
				Payload: &pb.AgentMessage_Response{
					Response: &pb.MessageResponse{
						RequestId: requestID,
						Event: &pb.MessageResponse_ToolResult{
							ToolResult: &pb.ToolResult{
								Id:      "tool-123",
								Output:  "file contents here",
								IsError: false,
							},
						},
					},
				},
			},
			{
				Payload: &pb.AgentMessage_Response{
					Response: &pb.MessageResponse{
						RequestId: requestID,
						Event: &pb.MessageResponse_Done{
							Done: &pb.Done{
								FullResponse: "I read the file for you.",
							},
						},
					},
				},
			},
		}

		for _, resp := range responses {
			if err := stream.Send(resp); err != nil {
				t.Errorf("agent send failed: %v", err)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	time.Sleep(50 * time.Millisecond)

	respChan, err := gw.agentManager.SendMessage(ctx, &agent.SendRequest{
		ThreadID: "thread-tool",
		Sender:   "test@example.com",
		Content:  "Read a file for me",
		AgentID:  agentID,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	var responses []*agent.Response
	timeout := time.After(5 * time.Second)

collectLoop:
	for {
		select {
		case resp, ok := <-respChan:
			if !ok {
				break collectLoop
			}
			responses = append(responses, resp)
			if resp.Done {
				break collectLoop
			}
		case <-timeout:
			t.Fatal("timeout waiting for responses")
		}
	}

	<-agentDone

	if len(responses) != 3 {
		t.Fatalf("expected 3 responses, got %d", len(responses))
	}

	// Verify tool_use
	if responses[0].Event != agent.EventToolUse {
		t.Errorf("expected EventToolUse, got %v", responses[0].Event)
	}
	if responses[0].ToolUse == nil {
		t.Fatal("ToolUse should not be nil")
	}
	if responses[0].ToolUse.Name != "read_file" {
		t.Errorf("tool name = %q, want %q", responses[0].ToolUse.Name, "read_file")
	}

	// Verify tool_result
	if responses[1].Event != agent.EventToolResult {
		t.Errorf("expected EventToolResult, got %v", responses[1].Event)
	}
	if responses[1].ToolResult == nil {
		t.Fatal("ToolResult should not be nil")
	}
	if responses[1].ToolResult.Output != "file contents here" {
		t.Errorf("tool output = %q, want %q", responses[1].ToolResult.Output, "file contents here")
	}

	// Verify done
	if responses[2].Event != agent.EventDone {
		t.Errorf("expected EventDone, got %v", responses[2].Event)
	}
}

// TestMessageRoundTrip_Error tests error handling through the system.
func TestMessageRoundTrip_Error(t *testing.T) {
	cfg := testConfig(t)
	logger := testLogger()

	gw, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx := t.Context()

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

	client := pb.NewCovenControlClient(conn)
	stream, err := client.AgentStream(ctx)
	if err != nil {
		t.Fatalf("AgentStream() failed: %v", err)
	}

	agentID := uuid.New().String()
	err = stream.Send(&pb.AgentMessage{
		Payload: &pb.AgentMessage_Register{
			Register: &pb.RegisterAgent{
				AgentId: agentID,
				Name:    "error-test-agent",
			},
		},
	})
	if err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	_, err = stream.Recv() // welcome
	if err != nil {
		t.Fatalf("welcome failed: %v", err)
	}

	// Agent goroutine: respond with error
	agentDone := make(chan struct{})
	go func() {
		defer close(agentDone)

		msg, err := stream.Recv()
		if err != nil {
			t.Errorf("agent recv failed: %v", err)
			return
		}

		requestID := msg.GetSendMessage().GetRequestId()

		err = stream.Send(&pb.AgentMessage{
			Payload: &pb.AgentMessage_Response{
				Response: &pb.MessageResponse{
					RequestId: requestID,
					Event: &pb.MessageResponse_Error{
						Error: "something went wrong",
					},
				},
			},
		})
		if err != nil {
			t.Errorf("agent send error failed: %v", err)
		}
	}()

	time.Sleep(50 * time.Millisecond)

	respChan, err := gw.agentManager.SendMessage(ctx, &agent.SendRequest{
		ThreadID: "thread-error",
		Sender:   "test@example.com",
		Content:  "Cause an error",
		AgentID:  agentID,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	var responses []*agent.Response
	timeout := time.After(5 * time.Second)

collectLoop:
	for {
		select {
		case resp, ok := <-respChan:
			if !ok {
				break collectLoop
			}
			responses = append(responses, resp)
			if resp.Done {
				break collectLoop
			}
		case <-timeout:
			t.Fatal("timeout waiting for responses")
		}
	}

	<-agentDone

	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}

	if responses[0].Event != agent.EventError {
		t.Errorf("expected EventError, got %v", responses[0].Event)
	}
	if responses[0].Error != "something went wrong" {
		t.Errorf("error = %q, want %q", responses[0].Error, "something went wrong")
	}
	if !responses[0].Done {
		t.Error("error response should have Done=true")
	}
}
