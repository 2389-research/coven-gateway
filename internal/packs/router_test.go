// ABOUTME: Tests for the tool router including routing, timeout, and error handling.
// ABOUTME: Validates request correlation and concurrent access patterns.

package packs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	pb "github.com/2389/coven-gateway/proto/coven"
)

// setupRouterTest creates a registry and router for testing.
func setupRouterTest(t *testing.T) (*Registry, *Router) {
	t.Helper()
	registry := NewRegistry(slog.Default())
	router := NewRouter(RouterConfig{
		Registry: registry,
		Logger:   slog.Default(),
		Timeout:  5 * time.Second,
	})
	return registry, router
}

// registerTestPack registers a test pack with the given tools.
func registerTestPack(t *testing.T, registry *Registry, packID string, tools ...*pb.ToolDefinition) *Pack {
	t.Helper()
	manifest := &pb.PackManifest{
		PackId:  packID,
		Version: "1.0.0",
		Tools:   tools,
	}
	err := registry.RegisterPack(packID, manifest)
	if err != nil {
		t.Fatalf("failed to register pack: %v", err)
	}
	return registry.GetPack(packID)
}

func TestRouterRouteToolCall(t *testing.T) {
	t.Run("routes tool call to pack and receives response", func(t *testing.T) {
		registry, router := setupRouterTest(t)

		// Register a pack with a tool
		pack := registerTestPack(t, registry, "test-pack",
			createTestTool("my-tool", "A test tool"),
		)

		// Simulate pack handling requests in background
		go func() {
			for req := range pack.Channel {
				router.HandleToolResponse(&pb.ExecuteToolResponse{
					RequestId: req.RequestId,
					Result: &pb.ExecuteToolResponse_OutputJson{
						OutputJson: `{"result": "success"}`,
					},
				})
			}
		}()

		// Route a tool call
		ctx := context.Background()
		resp, err := router.RouteToolCall(ctx, "my-tool", `{"input": "test"}`, "req-123", "test-agent")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp == nil {
			t.Fatal("expected response, got nil")
		}
		if resp.GetRequestId() != "req-123" {
			t.Errorf("expected request_id 'req-123', got '%s'", resp.GetRequestId())
		}
		if resp.GetOutputJson() != `{"result": "success"}` {
			t.Errorf("expected output_json, got %v", resp.GetResult())
		}
	})

	t.Run("returns error response from pack", func(t *testing.T) {
		registry, router := setupRouterTest(t)

		pack := registerTestPack(t, registry, "test-pack",
			createTestTool("failing-tool", "A tool that fails"),
		)

		// Simulate pack returning an error
		go func() {
			for req := range pack.Channel {
				router.HandleToolResponse(&pb.ExecuteToolResponse{
					RequestId: req.RequestId,
					Result: &pb.ExecuteToolResponse_Error{
						Error: "tool execution failed",
					},
				})
			}
		}()

		ctx := context.Background()
		resp, err := router.RouteToolCall(ctx, "failing-tool", `{}`, "req-456", "test-agent")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.GetError() != "tool execution failed" {
			t.Errorf("expected error in response, got %v", resp.GetResult())
		}
	})

	t.Run("returns ErrToolNotFound for unknown tool", func(t *testing.T) {
		_, router := setupRouterTest(t)

		ctx := context.Background()
		resp, err := router.RouteToolCall(ctx, "nonexistent-tool", `{}`, "req-789", "test-agent")

		if !errors.Is(err, ErrToolNotFound) {
			t.Errorf("expected ErrToolNotFound, got %v", err)
		}
		if resp != nil {
			t.Error("expected nil response")
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		registry, router := setupRouterTest(t)

		// Register a pack but don't handle requests (to test timeout)
		registerTestPack(t, registry, "slow-pack",
			createTestTool("slow-tool", "A slow tool"),
		)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		resp, err := router.RouteToolCall(ctx, "slow-tool", `{}`, "req-cancelled", "test-agent")

		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
		if resp != nil {
			t.Error("expected nil response")
		}
	})

	t.Run("times out when pack does not respond", func(t *testing.T) {
		registry := NewRegistry(slog.Default())
		router := NewRouter(RouterConfig{
			Registry: registry,
			Logger:   slog.Default(),
			Timeout:  100 * time.Millisecond, // Short timeout for test
		})

		// Register a pack but don't handle requests
		registerTestPack(t, registry, "unresponsive-pack",
			createTestTool("unresponsive-tool", "A tool that never responds"),
		)

		ctx := context.Background()
		resp, err := router.RouteToolCall(ctx, "unresponsive-tool", `{}`, "req-timeout", "test-agent")

		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("expected context.DeadlineExceeded, got %v", err)
		}
		if resp != nil {
			t.Error("expected nil response")
		}
	})

	t.Run("uses tool-specific timeout when defined", func(t *testing.T) {
		registry := NewRegistry(slog.Default())
		router := NewRouter(RouterConfig{
			Registry: registry,
			Logger:   slog.Default(),
			Timeout:  10 * time.Second, // Long default
		})

		// Register a tool with short timeout
		manifest := &pb.PackManifest{
			PackId:  "quick-pack",
			Version: "1.0.0",
			Tools: []*pb.ToolDefinition{
				{
					Name:           "quick-tool",
					Description:    "A tool with short timeout",
					TimeoutSeconds: 1, // 1 second timeout
				},
			},
		}
		registry.RegisterPack("quick-pack", manifest)

		start := time.Now()
		ctx := context.Background()
		_, err := router.RouteToolCall(ctx, "quick-tool", `{}`, "req-quick", "test-agent")
		elapsed := time.Since(start)

		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("expected context.DeadlineExceeded, got %v", err)
		}
		// Should timeout in ~1 second, not 10 seconds
		if elapsed > 2*time.Second {
			t.Errorf("expected timeout around 1s, took %v", elapsed)
		}
	})
}

func TestRouterHandleToolResponse(t *testing.T) {
	t.Run("routes response to waiting caller", func(t *testing.T) {
		registry, router := setupRouterTest(t)

		pack := registerTestPack(t, registry, "test-pack",
			createTestTool("my-tool", "A test tool"),
		)

		// Start the tool call in background
		done := make(chan struct{})
		var resp *pb.ExecuteToolResponse
		var err error

		go func() {
			ctx := context.Background()
			resp, err = router.RouteToolCall(ctx, "my-tool", `{}`, "req-abc", "test-agent")
			close(done)
		}()

		// Wait for request to be sent to pack
		select {
		case req := <-pack.Channel:
			// Send response
			router.HandleToolResponse(&pb.ExecuteToolResponse{
				RequestId: req.RequestId,
				Result: &pb.ExecuteToolResponse_OutputJson{
					OutputJson: `{"handled": true}`,
				},
			})
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for request")
		}

		// Wait for call to complete
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for response")
		}

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.GetOutputJson() != `{"handled": true}` {
			t.Errorf("unexpected response: %v", resp)
		}
	})

	t.Run("ignores response for unknown request", func(t *testing.T) {
		_, router := setupRouterTest(t)

		// This should not panic or error
		router.HandleToolResponse(&pb.ExecuteToolResponse{
			RequestId: "unknown-request",
			Result: &pb.ExecuteToolResponse_OutputJson{
				OutputJson: `{}`,
			},
		})
	})
}

func TestRouterHasTool(t *testing.T) {
	t.Run("returns true for registered tool", func(t *testing.T) {
		registry, router := setupRouterTest(t)

		registerTestPack(t, registry, "test-pack",
			createTestTool("existing-tool", "A tool"),
		)

		if !router.HasTool("existing-tool") {
			t.Error("expected HasTool to return true")
		}
	})

	t.Run("returns false for unregistered tool", func(t *testing.T) {
		_, router := setupRouterTest(t)

		if router.HasTool("nonexistent-tool") {
			t.Error("expected HasTool to return false")
		}
	})
}

func TestRouterGetToolDefinition(t *testing.T) {
	t.Run("returns definition for registered tool", func(t *testing.T) {
		registry, router := setupRouterTest(t)

		registerTestPack(t, registry, "test-pack",
			createTestTool("my-tool", "My tool description"),
		)

		def := router.GetToolDefinition("my-tool")
		if def == nil {
			t.Fatal("expected tool definition")
		}
		if def.GetName() != "my-tool" {
			t.Errorf("expected name 'my-tool', got '%s'", def.GetName())
		}
		if def.GetDescription() != "My tool description" {
			t.Errorf("expected description 'My tool description', got '%s'", def.GetDescription())
		}
	})

	t.Run("returns nil for unregistered tool", func(t *testing.T) {
		_, router := setupRouterTest(t)

		def := router.GetToolDefinition("nonexistent-tool")
		if def != nil {
			t.Error("expected nil definition")
		}
	})
}

func TestRouterPendingCount(t *testing.T) {
	t.Run("tracks pending requests", func(t *testing.T) {
		registry, router := setupRouterTest(t)

		pack := registerTestPack(t, registry, "test-pack",
			createTestTool("my-tool", "A test tool"),
		)

		if router.PendingCount() != 0 {
			t.Errorf("expected 0 pending, got %d", router.PendingCount())
		}

		// Start multiple requests
		var wg sync.WaitGroup
		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				ctx := context.Background()
				router.RouteToolCall(ctx, "my-tool", `{}`, fmt.Sprintf("req-%d", id), "test-agent")
			}(i)
		}

		// Poll until all 3 requests are registered (more reliable than sleep)
		deadline := time.Now().Add(time.Second)
		for router.PendingCount() < 3 {
			if time.Now().After(deadline) {
				t.Fatalf("only %d requests registered in time", router.PendingCount())
			}
			time.Sleep(time.Millisecond)
		}

		pending := router.PendingCount()
		if pending != 3 {
			t.Errorf("expected 3 pending, got %d", pending)
		}

		// Complete the requests
		for i := 0; i < 3; i++ {
			select {
			case req := <-pack.Channel:
				router.HandleToolResponse(&pb.ExecuteToolResponse{
					RequestId: req.RequestId,
					Result: &pb.ExecuteToolResponse_OutputJson{
						OutputJson: `{}`,
					},
				})
			case <-time.After(time.Second):
				t.Fatal("timeout waiting for request")
			}
		}

		wg.Wait()

		if router.PendingCount() != 0 {
			t.Errorf("expected 0 pending after completion, got %d", router.PendingCount())
		}
	})
}

func TestRouterDuplicateRequestID(t *testing.T) {
	t.Run("returns ErrDuplicateRequestID for duplicate request", func(t *testing.T) {
		registry, router := setupRouterTest(t)

		pack := registerTestPack(t, registry, "test-pack",
			createTestTool("my-tool", "A test tool"),
		)

		// Start a goroutine to consume from pack channel so first request doesn't block
		stopConsumer := make(chan struct{})
		go func() {
			for {
				select {
				case <-stopConsumer:
					return
				case <-pack.Channel:
					// Consume but don't respond - let the request wait
				}
			}
		}()
		defer close(stopConsumer)

		// Start first request (will block waiting for response)
		firstRequestDone := make(chan struct{})
		go func() {
			defer close(firstRequestDone)
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			// This will block until timeout since no one responds
			router.RouteToolCall(ctx, "my-tool", `{}`, "duplicate-id", "test-agent")
		}()

		// Poll until the first request is registered (more reliable than sleep)
		deadline := time.Now().Add(time.Second)
		for router.PendingCount() == 0 {
			if time.Now().After(deadline) {
				t.Fatal("first request did not register in time")
			}
			time.Sleep(time.Millisecond)
		}

		// Try second request with same ID - should fail immediately
		ctx := context.Background()
		_, err := router.RouteToolCall(ctx, "my-tool", `{}`, "duplicate-id", "test-agent")

		if !errors.Is(err, ErrDuplicateRequestID) {
			t.Errorf("expected ErrDuplicateRequestID, got %v", err)
		}

		// Wait for first request to complete (via timeout)
		<-firstRequestDone
	})
}

func TestRouterConcurrentAccess(t *testing.T) {
	t.Run("handles concurrent tool calls", func(t *testing.T) {
		registry, router := setupRouterTest(t)

		pack := registerTestPack(t, registry, "test-pack",
			createTestTool("concurrent-tool", "A concurrent tool"),
		)

		// Handle requests in background (multiple goroutines to handle concurrency)
		for i := 0; i < 4; i++ {
			go func() {
				for req := range pack.Channel {
					// Simulate some processing time
					time.Sleep(5 * time.Millisecond)
					router.HandleToolResponse(&pb.ExecuteToolResponse{
						RequestId: req.RequestId,
						Result: &pb.ExecuteToolResponse_OutputJson{
							OutputJson: `{"id": "` + req.RequestId + `"}`,
						},
					})
				}
			}()
		}

		// Launch concurrent requests (within channel buffer size)
		var wg sync.WaitGroup
		numRequests := 10
		errCh := make(chan error, numRequests)

		for i := 0; i < numRequests; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				ctx := context.Background()
				reqID := fmt.Sprintf("req-%d", id)
				resp, err := router.RouteToolCall(ctx, "concurrent-tool", `{}`, reqID, "test-agent")
				if err != nil {
					errCh <- err
					return
				}
				if resp.GetRequestId() != reqID {
					errCh <- errors.New("request ID mismatch")
				}
			}(i)
		}

		wg.Wait()
		close(errCh)

		for err := range errCh {
			t.Errorf("concurrent call error: %v", err)
		}
	})

	t.Run("handles pack unregistration during call", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping test with inherent race in short mode")
		}
		registry := NewRegistry(slog.Default())
		router := NewRouter(RouterConfig{
			Registry: registry,
			Logger:   slog.Default(),
			Timeout:  500 * time.Millisecond, // Short timeout for test
		})

		registerTestPack(t, registry, "ephemeral-pack",
			createTestTool("ephemeral-tool", "A tool that disappears"),
		)

		// Start a call
		done := make(chan error)
		go func() {
			ctx := context.Background()
			_, err := router.RouteToolCall(ctx, "ephemeral-tool", `{}`, "req-ephemeral", "test-agent")
			done <- err
		}()

		// Poll until the request is registered (more reliable than sleep)
		deadline := time.Now().Add(time.Second)
		for router.PendingCount() == 0 {
			if time.Now().After(deadline) {
				t.Fatal("request did not register in time")
			}
			time.Sleep(time.Millisecond)
		}

		// Unregister the pack (closes channel)
		// Note: This is inherently racy - the router may be sending to the channel
		// when we close it. The router handles this via panic recovery.
		registry.UnregisterPack("ephemeral-pack")

		// The call should eventually fail or timeout
		select {
		case err := <-done:
			// Either ErrPackDisconnected or context deadline exceeded is acceptable
			if err == nil {
				t.Error("expected error when pack disconnected")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("test timed out waiting for call to fail")
		}
	})
}

func TestRouterPackDisconnected(t *testing.T) {
	t.Run("detects closed pack channel via unregister", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping test with inherent race in short mode")
		}
		registry := NewRegistry(slog.Default())
		router := NewRouter(RouterConfig{
			Registry: registry,
			Logger:   slog.Default(),
			Timeout:  500 * time.Millisecond, // Short timeout for test
		})

		// Register pack
		registerTestPack(t, registry, "disconnect-pack",
			createTestTool("disconnect-tool", "A tool"),
		)

		// Start a tool call in background
		done := make(chan error, 1)
		go func() {
			ctx := context.Background()
			_, err := router.RouteToolCall(ctx, "disconnect-tool", `{}`, "req-disconnect", "test-agent")
			done <- err
		}()

		// Poll until the request is registered (more reliable than sleep)
		deadline := time.Now().Add(time.Second)
		for router.PendingCount() == 0 {
			if time.Now().After(deadline) {
				t.Fatal("request did not register in time")
			}
			time.Sleep(time.Millisecond)
		}

		// Unregister pack (closes channel, should cause disconnect error or timeout)
		// Note: This is inherently racy - the router may be sending to the channel
		// when we close it. The router handles this via panic recovery.
		registry.UnregisterPack("disconnect-pack")

		// The call should fail with either ErrPackDisconnected or timeout
		select {
		case err := <-done:
			// Either error is acceptable since timing can vary
			if err == nil {
				t.Error("expected error when pack disconnected")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("test timed out waiting for call to fail")
		}
	})

	t.Run("returns ErrPackDisconnected for closed channel on send", func(t *testing.T) {
		registry := NewRegistry(slog.Default())
		router := NewRouter(RouterConfig{
			Registry: registry,
			Logger:   slog.Default(),
			Timeout:  100 * time.Millisecond,
		})

		// Manually create a pack with a closed channel to test the panic recovery
		manifest := &pb.PackManifest{
			PackId:  "closed-pack",
			Version: "1.0.0",
			Tools: []*pb.ToolDefinition{
				createTestTool("closed-tool", "A tool with closed channel"),
			},
		}
		registry.RegisterPack("closed-pack", manifest)
		pack := registry.GetPack("closed-pack")

		// Close the pack safely to simulate disconnection
		pack.Close()

		ctx := context.Background()
		_, err := router.RouteToolCall(ctx, "closed-tool", `{}`, "req-closed", "test-agent")

		if !errors.Is(err, ErrPackDisconnected) {
			t.Errorf("expected ErrPackDisconnected, got %v", err)
		}
	})
}

func TestNewRouter(t *testing.T) {
	t.Run("uses default timeout when not specified", func(t *testing.T) {
		registry := NewRegistry(slog.Default())
		router := NewRouter(RouterConfig{
			Registry: registry,
			Logger:   slog.Default(),
			// Timeout not specified
		})

		if router.timeout != DefaultTimeout {
			t.Errorf("expected default timeout %v, got %v", DefaultTimeout, router.timeout)
		}
	})

	t.Run("uses custom timeout when specified", func(t *testing.T) {
		registry := NewRegistry(slog.Default())
		router := NewRouter(RouterConfig{
			Registry: registry,
			Logger:   slog.Default(),
			Timeout:  60 * time.Second,
		})

		if router.timeout != 60*time.Second {
			t.Errorf("expected timeout 60s, got %v", router.timeout)
		}
	})
}

func TestRouteBuiltinTool(t *testing.T) {
	t.Run("dispatches builtin tool in-process", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
		reg := NewRegistry(logger)

		// Register a builtin pack
		pack := &BuiltinPack{
			ID: "builtin:test",
			Tools: []*BuiltinTool{
				{
					Definition: &pb.ToolDefinition{Name: "echo"},
					Handler: func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
						return input, nil // echo back the input
					},
				},
			},
		}
		if err := reg.RegisterBuiltinPack(pack); err != nil {
			t.Fatalf("RegisterBuiltinPack: %v", err)
		}

		router := NewRouter(RouterConfig{
			Registry: reg,
			Logger:   logger,
		})

		resp, err := router.RouteToolCall(context.Background(), "echo", `{"hello": "world"}`, "req-1", "agent-1")
		if err != nil {
			t.Fatalf("RouteToolCall: %v", err)
		}

		if resp.GetError() != "" {
			t.Errorf("unexpected error: %s", resp.GetError())
		}
		if resp.GetOutputJson() != `{"hello": "world"}` {
			t.Errorf("unexpected output: %s", resp.GetOutputJson())
		}
	})

	t.Run("builtin handler error returns error response", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
		reg := NewRegistry(logger)

		// Register a builtin that errors
		pack := &BuiltinPack{
			ID: "builtin:error-test",
			Tools: []*BuiltinTool{
				{
					Definition: &pb.ToolDefinition{Name: "fail"},
					Handler: func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
						return nil, errors.New("intentional failure")
					},
				},
			},
		}
		if err := reg.RegisterBuiltinPack(pack); err != nil {
			t.Fatalf("RegisterBuiltinPack: %v", err)
		}

		router := NewRouter(RouterConfig{
			Registry: reg,
			Logger:   logger,
		})

		resp, err := router.RouteToolCall(context.Background(), "fail", `{}`, "req-fail", "agent-1")
		if err != nil {
			t.Fatalf("RouteToolCall should not return error for builtin errors: %v", err)
		}

		if resp.GetError() != "intentional failure" {
			t.Errorf("expected error 'intentional failure', got '%s'", resp.GetError())
		}
	})

	t.Run("builtin receives agentID", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
		reg := NewRegistry(logger)

		var receivedAgentID string
		pack := &BuiltinPack{
			ID: "builtin:agent-test",
			Tools: []*BuiltinTool{
				{
					Definition: &pb.ToolDefinition{Name: "check-agent"},
					Handler: func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
						receivedAgentID = agentID
						return []byte(`{"ok": true}`), nil
					},
				},
			},
		}
		if err := reg.RegisterBuiltinPack(pack); err != nil {
			t.Fatalf("RegisterBuiltinPack: %v", err)
		}

		router := NewRouter(RouterConfig{
			Registry: reg,
			Logger:   logger,
		})

		_, err := router.RouteToolCall(context.Background(), "check-agent", `{}`, "req-agent", "test-agent-42")
		if err != nil {
			t.Fatalf("RouteToolCall: %v", err)
		}

		if receivedAgentID != "test-agent-42" {
			t.Errorf("expected agentID 'test-agent-42', got '%s'", receivedAgentID)
		}
	})
}

func TestRouterHasToolWithBuiltins(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	reg := NewRegistry(logger)

	// Register a builtin
	pack := &BuiltinPack{
		ID: "builtin:test",
		Tools: []*BuiltinTool{
			{
				Definition: &pb.ToolDefinition{Name: "builtin-tool"},
				Handler: func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
					return []byte(`{}`), nil
				},
			},
		},
	}
	if err := reg.RegisterBuiltinPack(pack); err != nil {
		t.Fatalf("RegisterBuiltinPack: %v", err)
	}

	router := NewRouter(RouterConfig{
		Registry: reg,
		Logger:   logger,
	})

	t.Run("HasTool returns true for builtin tool", func(t *testing.T) {
		if !router.HasTool("builtin-tool") {
			t.Error("expected HasTool to return true for builtin tool")
		}
	})

	t.Run("HasTool returns false for non-existent tool", func(t *testing.T) {
		if router.HasTool("non-existent") {
			t.Error("expected HasTool to return false for non-existent tool")
		}
	})
}

func TestRouterGetToolDefinitionWithBuiltins(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	reg := NewRegistry(logger)

	// Register a builtin with a description
	pack := &BuiltinPack{
		ID: "builtin:test",
		Tools: []*BuiltinTool{
			{
				Definition: &pb.ToolDefinition{
					Name:        "builtin-tool",
					Description: "A builtin test tool",
				},
				Handler: func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
					return []byte(`{}`), nil
				},
			},
		},
	}
	if err := reg.RegisterBuiltinPack(pack); err != nil {
		t.Fatalf("RegisterBuiltinPack: %v", err)
	}

	router := NewRouter(RouterConfig{
		Registry: reg,
		Logger:   logger,
	})

	t.Run("GetToolDefinition returns definition for builtin tool", func(t *testing.T) {
		def := router.GetToolDefinition("builtin-tool")
		if def == nil {
			t.Fatal("expected tool definition for builtin tool")
		}
		if def.GetName() != "builtin-tool" {
			t.Errorf("expected name 'builtin-tool', got '%s'", def.GetName())
		}
		if def.GetDescription() != "A builtin test tool" {
			t.Errorf("expected description 'A builtin test tool', got '%s'", def.GetDescription())
		}
	})

	t.Run("GetToolDefinition returns nil for non-existent tool", func(t *testing.T) {
		def := router.GetToolDefinition("non-existent")
		if def != nil {
			t.Error("expected nil definition for non-existent tool")
		}
	})
}
