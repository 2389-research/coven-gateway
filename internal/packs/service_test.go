// ABOUTME: Tests for the PackService gRPC handlers including registration, tool dispatch, and result handling.
// ABOUTME: Validates thread-safe pending request tracking, timeout behavior, and concurrent pack connections.

package packs

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	pb "github.com/2389/fold-gateway/proto/fold"
)

// mockRegisterStream implements grpc.ServerStreamingServer[pb.ExecuteToolRequest] for testing.
type mockRegisterStream struct {
	grpc.ServerStream
	ctx      context.Context
	sent     []*pb.ExecuteToolRequest
	sendErr  error
	mu       sync.Mutex
	sendChan chan *pb.ExecuteToolRequest
}

func newMockRegisterStream(ctx context.Context) *mockRegisterStream {
	return &mockRegisterStream{
		ctx:      ctx,
		sent:     make([]*pb.ExecuteToolRequest, 0),
		sendChan: make(chan *pb.ExecuteToolRequest, 16),
	}
}

func (m *mockRegisterStream) Send(req *pb.ExecuteToolRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sent = append(m.sent, req)
	select {
	case m.sendChan <- req:
	default:
	}
	return nil
}

func (m *mockRegisterStream) SetSendError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendErr = err
}

func (m *mockRegisterStream) GetSent() []*pb.ExecuteToolRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*pb.ExecuteToolRequest, len(m.sent))
	copy(result, m.sent)
	return result
}

func (m *mockRegisterStream) Context() context.Context {
	return m.ctx
}

func (m *mockRegisterStream) SetHeader(metadata.MD) error {
	return nil
}

func (m *mockRegisterStream) SendHeader(metadata.MD) error {
	return nil
}

func (m *mockRegisterStream) SetTrailer(metadata.MD) {
}

func (m *mockRegisterStream) SendMsg(msg any) error {
	return m.Send(msg.(*pb.ExecuteToolRequest))
}

func (m *mockRegisterStream) RecvMsg(msg any) error {
	return io.EOF
}

// createTestService creates a PackServiceServer with a fresh registry for testing.
func createTestService() (*PackServiceServer, *Registry) {
	logger := slog.Default()
	registry := NewRegistry(logger)
	service := NewPackServiceServer(registry, logger)
	return service, registry
}

func TestPackServiceRegister(t *testing.T) {
	t.Run("registers pack and keeps stream open", func(t *testing.T) {
		service, registry := createTestService()

		manifest := &pb.PackManifest{
			PackId:  "test-pack",
			Version: "1.0.0",
			Tools: []*pb.ToolDefinition{
				{Name: "tool-a", Description: "Tool A"},
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		stream := newMockRegisterStream(ctx)

		// Run Register in goroutine since it blocks
		errCh := make(chan error, 1)
		go func() {
			errCh <- service.Register(manifest, stream)
		}()

		// Give time for registration
		time.Sleep(50 * time.Millisecond)

		// Verify pack is registered
		pack := registry.GetPack("test-pack")
		if pack == nil {
			t.Error("expected pack to be registered")
		}

		// Cancel context to close stream
		cancel()

		// Wait for Register to return
		select {
		case err := <-errCh:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("unexpected error: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("Register did not return after context cancel")
		}

		// Verify pack is unregistered after stream close
		time.Sleep(50 * time.Millisecond)
		pack = registry.GetPack("test-pack")
		if pack != nil {
			t.Error("expected pack to be unregistered after stream close")
		}
	})

	t.Run("forwards tool requests to stream", func(t *testing.T) {
		service, registry := createTestService()

		manifest := &pb.PackManifest{
			PackId:  "test-pack",
			Version: "1.0.0",
			Tools: []*pb.ToolDefinition{
				{Name: "my-tool", Description: "My Tool"},
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		stream := newMockRegisterStream(ctx)

		// Run Register in goroutine
		go func() {
			service.Register(manifest, stream)
		}()

		// Wait for registration
		time.Sleep(50 * time.Millisecond)

		// Get pack channel and send a tool request
		pack := registry.GetPack("test-pack")
		if pack == nil {
			t.Fatal("expected pack to be registered")
		}

		toolReq := &pb.ExecuteToolRequest{
			ToolName:  "my-tool",
			InputJson: `{"arg": "value"}`,
			RequestId: "req-123",
		}

		select {
		case pack.Channel <- toolReq:
		case <-time.After(time.Second):
			t.Fatal("failed to send tool request to channel")
		}

		// Wait for request to be sent to stream
		select {
		case sent := <-stream.sendChan:
			if sent.GetToolName() != "my-tool" {
				t.Errorf("expected tool name 'my-tool', got '%s'", sent.GetToolName())
			}
			if sent.GetRequestId() != "req-123" {
				t.Errorf("expected request ID 'req-123', got '%s'", sent.GetRequestId())
			}
		case <-time.After(time.Second):
			t.Error("tool request was not forwarded to stream")
		}
	})

	t.Run("handles duplicate pack registration", func(t *testing.T) {
		service, registry := createTestService()

		manifest := &pb.PackManifest{
			PackId:  "dup-pack",
			Version: "1.0.0",
			Tools: []*pb.ToolDefinition{
				{Name: "tool-a", Description: "Tool A"},
			},
		}

		// Pre-register the pack
		err := registry.RegisterPack("dup-pack", manifest)
		if err != nil {
			t.Fatalf("failed to pre-register pack: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		stream := newMockRegisterStream(ctx)

		// Try to register again via service
		err = service.Register(manifest, stream)
		if err == nil {
			t.Error("expected error for duplicate pack registration")
		}
	})
}

func TestPackServiceToolResult(t *testing.T) {
	t.Run("delivers result to waiting request", func(t *testing.T) {
		service, _ := createTestService()

		// Create a pending request
		requestID := "test-req-001"
		resultCh := service.CreatePendingRequest(requestID)

		// Send result in goroutine
		go func() {
			time.Sleep(10 * time.Millisecond)
			resp := &pb.ExecuteToolResponse{
				RequestId: requestID,
				Result:    &pb.ExecuteToolResponse_OutputJson{OutputJson: `{"result": "success"}`},
			}
			_, err := service.ToolResult(context.Background(), resp)
			if err != nil {
				t.Errorf("ToolResult returned error: %v", err)
			}
		}()

		// Wait for result
		select {
		case result := <-resultCh:
			if result.GetOutputJson() != `{"result": "success"}` {
				t.Errorf("unexpected result: %v", result)
			}
		case <-time.After(time.Second):
			t.Error("did not receive result")
		}
	})

	t.Run("handles error result", func(t *testing.T) {
		service, _ := createTestService()

		requestID := "test-req-002"
		resultCh := service.CreatePendingRequest(requestID)

		go func() {
			time.Sleep(10 * time.Millisecond)
			resp := &pb.ExecuteToolResponse{
				RequestId: requestID,
				Result:    &pb.ExecuteToolResponse_Error{Error: "tool execution failed"},
			}
			service.ToolResult(context.Background(), resp)
		}()

		select {
		case result := <-resultCh:
			if result.GetError() != "tool execution failed" {
				t.Errorf("unexpected error: %v", result.GetError())
			}
		case <-time.After(time.Second):
			t.Error("did not receive result")
		}
	})

	t.Run("returns empty for unknown request ID", func(t *testing.T) {
		service, _ := createTestService()

		resp := &pb.ExecuteToolResponse{
			RequestId: "unknown-req",
			Result:    &pb.ExecuteToolResponse_OutputJson{OutputJson: `{}`},
		}

		result, err := service.ToolResult(context.Background(), resp)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result == nil {
			t.Error("expected empty response, got nil")
		}
	})
}

func TestPackServiceDispatchTool(t *testing.T) {
	t.Run("dispatches tool and receives result", func(t *testing.T) {
		service, registry := createTestService()

		// Register a pack
		manifest := &pb.PackManifest{
			PackId:  "dispatch-pack",
			Version: "1.0.0",
			Tools: []*pb.ToolDefinition{
				{Name: "echo", Description: "Echo tool", TimeoutSeconds: 5},
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		stream := newMockRegisterStream(ctx)

		// Run Register in goroutine
		go func() {
			service.Register(manifest, stream)
		}()

		time.Sleep(50 * time.Millisecond)

		// Simulate pack responding to tool requests
		go func() {
			for req := range stream.sendChan {
				// Simulate pack processing and returning result
				time.Sleep(10 * time.Millisecond)
				resp := &pb.ExecuteToolResponse{
					RequestId: req.GetRequestId(),
					Result:    &pb.ExecuteToolResponse_OutputJson{OutputJson: `{"echoed": true}`},
				}
				service.ToolResult(context.Background(), resp)
			}
		}()

		// Dispatch a tool call
		pack := registry.GetPack("dispatch-pack")
		if pack == nil {
			t.Fatal("pack not registered")
		}

		result, err := service.DispatchTool(context.Background(), pack, "echo", `{"input": "test"}`, 5*time.Second)
		if err != nil {
			t.Fatalf("DispatchTool error: %v", err)
		}

		if result.GetOutputJson() != `{"echoed": true}` {
			t.Errorf("unexpected result: %v", result)
		}
	})

	t.Run("times out if pack does not respond", func(t *testing.T) {
		service, registry := createTestService()

		manifest := &pb.PackManifest{
			PackId:  "slow-pack",
			Version: "1.0.0",
			Tools: []*pb.ToolDefinition{
				{Name: "slow-tool", Description: "Slow tool"},
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		stream := newMockRegisterStream(ctx)

		go func() {
			service.Register(manifest, stream)
		}()

		time.Sleep(50 * time.Millisecond)

		pack := registry.GetPack("slow-pack")
		if pack == nil {
			t.Fatal("pack not registered")
		}

		// Use very short timeout
		result, err := service.DispatchTool(context.Background(), pack, "slow-tool", `{}`, 100*time.Millisecond)
		if err == nil {
			t.Error("expected timeout error")
		}
		if result != nil {
			t.Errorf("expected nil result on timeout, got: %v", result)
		}
		if !errors.Is(err, ErrToolTimeout) {
			t.Errorf("expected ErrToolTimeout, got: %v", err)
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		service, registry := createTestService()

		manifest := &pb.PackManifest{
			PackId:  "cancel-pack",
			Version: "1.0.0",
			Tools: []*pb.ToolDefinition{
				{Name: "cancel-tool", Description: "Tool to cancel"},
			},
		}

		streamCtx, streamCancel := context.WithCancel(context.Background())
		defer streamCancel()
		stream := newMockRegisterStream(streamCtx)

		go func() {
			service.Register(manifest, stream)
		}()

		time.Sleep(50 * time.Millisecond)

		pack := registry.GetPack("cancel-pack")
		if pack == nil {
			t.Fatal("pack not registered")
		}

		// Create a context that we'll cancel
		dispatchCtx, dispatchCancel := context.WithCancel(context.Background())

		// Start dispatch in goroutine
		errCh := make(chan error, 1)
		go func() {
			_, err := service.DispatchTool(dispatchCtx, pack, "cancel-tool", `{}`, 30*time.Second)
			errCh <- err
		}()

		// Cancel after a short delay
		time.Sleep(50 * time.Millisecond)
		dispatchCancel()

		select {
		case err := <-errCh:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("expected context.Canceled, got: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("DispatchTool did not return after context cancel")
		}
	})
}

func TestPackServicePendingRequests(t *testing.T) {
	t.Run("creates and retrieves pending request", func(t *testing.T) {
		service, _ := createTestService()

		requestID := "pending-001"
		ch := service.CreatePendingRequest(requestID)

		if ch == nil {
			t.Error("expected channel to be created")
		}

		// Verify it can receive
		go func() {
			resp := &pb.ExecuteToolResponse{
				RequestId: requestID,
				Result:    &pb.ExecuteToolResponse_OutputJson{OutputJson: `{}`},
			}
			service.ToolResult(context.Background(), resp)
		}()

		select {
		case <-ch:
			// Good
		case <-time.After(time.Second):
			t.Error("channel did not receive")
		}
	})

	t.Run("removes pending request after delivery", func(t *testing.T) {
		service, _ := createTestService()

		requestID := "pending-002"
		ch := service.CreatePendingRequest(requestID)

		// Deliver result
		resp := &pb.ExecuteToolResponse{
			RequestId: requestID,
			Result:    &pb.ExecuteToolResponse_OutputJson{OutputJson: `{}`},
		}
		service.ToolResult(context.Background(), resp)

		// Drain channel
		<-ch

		// Subsequent delivery should be no-op (request already removed)
		resp2 := &pb.ExecuteToolResponse{
			RequestId: requestID,
			Result:    &pb.ExecuteToolResponse_OutputJson{OutputJson: `{"second": true}`},
		}
		result, err := service.ToolResult(context.Background(), resp2)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result == nil {
			t.Error("expected empty response")
		}
	})

	t.Run("handles concurrent pending requests", func(t *testing.T) {
		service, _ := createTestService()

		var wg sync.WaitGroup
		numRequests := 50

		for i := 0; i < numRequests; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				requestID := "concurrent-" + string(rune('a'+id%26)) + "-" + string(rune('0'+id/26))

				ch := service.CreatePendingRequest(requestID)

				// Simulate async response
				go func() {
					time.Sleep(time.Duration(id%10) * time.Millisecond)
					resp := &pb.ExecuteToolResponse{
						RequestId: requestID,
						Result:    &pb.ExecuteToolResponse_OutputJson{OutputJson: `{}`},
					}
					service.ToolResult(context.Background(), resp)
				}()

				select {
				case <-ch:
					// Good
				case <-time.After(time.Second):
					t.Errorf("request %s did not receive response", requestID)
				}
			}(i)
		}

		wg.Wait()
	})

	t.Run("cancel removes pending request", func(t *testing.T) {
		service, _ := createTestService()

		requestID := "cancel-pending-001"
		_ = service.CreatePendingRequest(requestID)

		// Cancel the pending request
		service.CancelPendingRequest(requestID)

		// Subsequent ToolResult should be no-op
		resp := &pb.ExecuteToolResponse{
			RequestId: requestID,
			Result:    &pb.ExecuteToolResponse_OutputJson{OutputJson: `{}`},
		}
		result, err := service.ToolResult(context.Background(), resp)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		// Should return empty since request was cancelled
		if result == nil {
			t.Error("expected empty response")
		}
	})
}

func TestPackServiceConcurrentStreams(t *testing.T) {
	t.Run("handles multiple packs connecting concurrently", func(t *testing.T) {
		service, registry := createTestService()

		var wg sync.WaitGroup
		numPacks := 10

		for i := 0; i < numPacks; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				manifest := &pb.PackManifest{
					PackId:  "concurrent-pack-" + string(rune('a'+id)),
					Version: "1.0.0",
					Tools: []*pb.ToolDefinition{
						{Name: "tool-" + string(rune('a'+id)), Description: "Tool"},
					},
				}

				ctx, cancel := context.WithCancel(context.Background())
				stream := newMockRegisterStream(ctx)

				// Run Register briefly then cancel
				go func() {
					service.Register(manifest, stream)
				}()

				time.Sleep(50 * time.Millisecond)
				cancel()
			}(i)
		}

		wg.Wait()

		// Give time for unregistrations
		time.Sleep(100 * time.Millisecond)

		// All packs should be unregistered
		packs := registry.ListPacks()
		if len(packs) != 0 {
			t.Errorf("expected 0 packs after all streams closed, got %d", len(packs))
		}
	})
}

func TestToolResultReturnsEmpty(t *testing.T) {
	service, _ := createTestService()

	resp := &pb.ExecuteToolResponse{
		RequestId: "any-id",
		Result:    &pb.ExecuteToolResponse_OutputJson{OutputJson: `{}`},
	}

	result, err := service.ToolResult(context.Background(), resp)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Should return a non-nil empty response
	if result == nil {
		t.Error("expected non-nil empty response")
	}
}
