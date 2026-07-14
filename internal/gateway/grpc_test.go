// ABOUTME: Tests for the CovenControl gRPC service message dispatch.
// ABOUTME: Proves pack tool execution does not block the agent message loop.

package gateway

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/2389/coven-gateway/internal/agent"
	"github.com/2389/coven-gateway/internal/packs"
	pb "github.com/2389/coven-gateway/proto/coven"
)

// fakeAgentStream implements pb.CovenControl_AgentStreamServer for tests.
// Only Context, Send, and Recv are exercised; the embedded nil
// grpc.ServerStream panics if anything else is called, which is what we want.
type fakeAgentStream struct {
	grpc.ServerStream
	ctx  context.Context
	mu   sync.Mutex
	sent []*pb.ServerMessage
}

func (f *fakeAgentStream) Context() context.Context { return f.ctx }

func (f *fakeAgentStream) Send(msg *pb.ServerMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, msg)
	return nil
}

func (f *fakeAgentStream) Recv() (*pb.AgentMessage, error) {
	<-f.ctx.Done()
	return nil, io.EOF
}

func (f *fakeAgentStream) sentMessages() []*pb.ServerMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*pb.ServerMessage(nil), f.sent...)
}

func TestExecutePackToolDoesNotBlockMessageLoop(t *testing.T) {
	logger := slog.Default()

	// A registered pack whose channel nobody serves: RouteToolCall blocks
	// until the router timeout, deterministically.
	registry := packs.NewRegistry(logger)
	manifest := &pb.PackManifest{
		PackId:  "slowpack",
		Version: "1.0.0",
		Tools:   []*pb.ToolDefinition{{Name: "slow_tool"}},
	}
	if err := registry.RegisterPack("slowpack", manifest); err != nil {
		t.Fatalf("RegisterPack failed: %v", err)
	}
	router := packs.NewRouter(packs.RouterConfig{
		Registry: registry,
		Logger:   logger,
		Timeout:  300 * time.Millisecond,
	})

	srv := &covenControlServer{
		gateway: &Gateway{packRouter: router},
		logger:  logger,
	}

	ctx := t.Context()
	stream := &fakeAgentStream{ctx: ctx}
	conn := agent.NewConnection(agent.ConnectionParams{ID: "agent-1", Name: "Test Agent", Stream: stream, Logger: logger})

	start := time.Now()
	srv.dispatchMessage(stream, conn, &pb.AgentMessage{
		Payload: &pb.AgentMessage_ExecutePackTool{
			ExecutePackTool: &pb.ExecutePackTool{
				RequestId: "req-1",
				ToolName:  "slow_tool",
			},
		},
	}, make(chan struct{}, maxConcurrentPackToolsPerConn))
	elapsed := time.Since(start)

	// The message loop must get control back immediately, not after the
	// tool's 300ms timeout.
	if elapsed > 100*time.Millisecond {
		t.Fatalf("dispatchMessage blocked %v on a pack tool; the message loop must not stall", elapsed)
	}

	// The (timeout-error) result must still reach the agent, eventually.
	deadline := time.After(2 * time.Second)
	for {
		var result *pb.PackToolResult
		for _, msg := range stream.sentMessages() {
			if r := msg.GetPackToolResult(); r != nil {
				result = r
				break
			}
		}
		if result != nil {
			if got := result.GetRequestId(); got != "req-1" {
				t.Fatalf("PackToolResult for request %q, want %q", got, "req-1")
			}
			if result.GetError() == "" {
				t.Fatal("expected a timeout error in PackToolResult")
			}
			return
		}
		select {
		case <-deadline:
			t.Fatal("PackToolResult never sent to agent")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
