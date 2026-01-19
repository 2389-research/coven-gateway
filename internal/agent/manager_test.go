// ABOUTME: Tests for the agent management layer including Manager and Connection.
// ABOUTME: Validates registration, message handling, and response functionality.

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	pb "github.com/2389/fold-gateway/proto/fold"
	"google.golang.org/grpc"
)

// mockStream implements pb.FoldControl_AgentStreamServer for testing.
type mockStream struct {
	grpc.ServerStream
	sentMessages []*pb.ServerMessage
	mu           sync.Mutex
}

func newMockStream() *mockStream {
	return &mockStream{
		sentMessages: make([]*pb.ServerMessage, 0),
	}
}

func (m *mockStream) Send(msg *pb.ServerMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sentMessages = append(m.sentMessages, msg)
	return nil
}

func (m *mockStream) Recv() (*pb.AgentMessage, error) {
	return nil, nil
}

func (m *mockStream) getSentMessages() []*pb.ServerMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*pb.ServerMessage, len(m.sentMessages))
	copy(result, m.sentMessages)
	return result
}

// TestConnectionSend tests sending messages through a connection.
func TestConnectionSend(t *testing.T) {
	t.Run("sends message to stream", func(t *testing.T) {
		stream := newMockStream()
		conn := NewConnection(ConnectionParams{ID: "agent-1", Name: "Test Agent", Capabilities: []string{"chat"}, Stream: stream, Logger: slog.Default()})

		msg := &pb.ServerMessage{
			Payload: &pb.ServerMessage_Welcome{
				Welcome: &pb.Welcome{
					ServerId: "server-1",
					AgentId:  "agent-1",
				},
			},
		}

		err := conn.Send(msg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		sent := stream.getSentMessages()
		if len(sent) != 1 {
			t.Fatalf("expected 1 message, got %d", len(sent))
		}
		if sent[0].GetWelcome().GetAgentId() != "agent-1" {
			t.Error("message not sent correctly")
		}
	})
}

// TestConnectionRequestHandling tests request/response correlation.
func TestConnectionRequestHandling(t *testing.T) {
	t.Run("creates and closes request channels", func(t *testing.T) {
		stream := newMockStream()
		conn := NewConnection(ConnectionParams{ID: "agent-1", Name: "Test Agent", Capabilities: []string{"chat"}, Stream: stream, Logger: slog.Default()})

		requestID := "req-123"
		respChan := conn.CreateRequest(requestID)

		if respChan == nil {
			t.Fatal("expected response channel, got nil")
		}

		// Close the request
		conn.CloseRequest(requestID)

		// Channel should be closed
		select {
		case _, ok := <-respChan:
			if ok {
				t.Error("expected channel to be closed")
			}
		default:
			t.Error("channel should be readable (closed)")
		}
	})

	t.Run("routes response to correct request channel", func(t *testing.T) {
		stream := newMockStream()
		conn := NewConnection(ConnectionParams{ID: "agent-1", Name: "Test Agent", Capabilities: []string{"chat"}, Stream: stream, Logger: slog.Default()})

		requestID := "req-456"
		respChan := conn.CreateRequest(requestID)

		// Simulate a response
		resp := &pb.MessageResponse{
			RequestId: requestID,
			Event: &pb.MessageResponse_Text{
				Text: "Hello, world!",
			},
		}

		conn.HandleResponse(resp)

		select {
		case received := <-respChan:
			if received.GetText() != "Hello, world!" {
				t.Errorf("expected 'Hello, world!', got '%s'", received.GetText())
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("timeout waiting for response")
		}
	})

	t.Run("ignores response for unknown request", func(t *testing.T) {
		stream := newMockStream()
		conn := NewConnection(ConnectionParams{ID: "agent-1", Name: "Test Agent", Capabilities: []string{"chat"}, Stream: stream, Logger: slog.Default()})

		// Send response without creating request first
		resp := &pb.MessageResponse{
			RequestId: "unknown-req",
			Event: &pb.MessageResponse_Text{
				Text: "orphaned response",
			},
		}

		// Should not panic
		conn.HandleResponse(resp)
	})
}

// TestManagerRegister tests agent registration.
func TestManagerRegister(t *testing.T) {
	t.Run("registers agent successfully", func(t *testing.T) {
		manager := NewManager(slog.Default())
		stream := newMockStream()
		conn := NewConnection(ConnectionParams{ID: "agent-1", Name: "Test Agent", Capabilities: []string{"chat", "code"}, Stream: stream, Logger: slog.Default()})

		err := manager.Register(conn)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify agent is registered
		agents := manager.ListAgents()
		if len(agents) != 1 {
			t.Fatalf("expected 1 agent, got %d", len(agents))
		}
		if agents[0].ID != "agent-1" {
			t.Errorf("expected agent-1, got %s", agents[0].ID)
		}
		if agents[0].Name != "Test Agent" {
			t.Errorf("expected 'Test Agent', got '%s'", agents[0].Name)
		}
	})

	t.Run("returns error for duplicate agent ID", func(t *testing.T) {
		manager := NewManager(slog.Default())
		stream1 := newMockStream()
		stream2 := newMockStream()

		conn1 := NewConnection(ConnectionParams{ID: "agent-1", Name: "Agent One", Capabilities: []string{"chat"}, Stream: stream1, Logger: slog.Default()})
		conn2 := NewConnection(ConnectionParams{ID: "agent-1", Name: "Agent One Duplicate", Capabilities: []string{"chat"}, Stream: stream2, Logger: slog.Default()})

		err := manager.Register(conn1)
		if err != nil {
			t.Fatalf("unexpected error on first register: %v", err)
		}

		err = manager.Register(conn2)
		if err == nil {
			t.Error("expected error for duplicate agent ID")
		}
		if err != ErrAgentAlreadyRegistered {
			t.Errorf("expected ErrAgentAlreadyRegistered, got %v", err)
		}
	})
}

// TestManagerUnregister tests agent unregistration.
func TestManagerUnregister(t *testing.T) {
	t.Run("unregisters existing agent", func(t *testing.T) {
		manager := NewManager(slog.Default())
		stream := newMockStream()
		conn := NewConnection(ConnectionParams{ID: "agent-1", Name: "Test Agent", Capabilities: []string{"chat"}, Stream: stream, Logger: slog.Default()})

		manager.Register(conn)
		manager.Unregister("agent-1")

		agents := manager.ListAgents()
		if len(agents) != 0 {
			t.Errorf("expected 0 agents, got %d", len(agents))
		}
	})

	t.Run("unregistering non-existent agent is no-op", func(t *testing.T) {
		manager := NewManager(slog.Default())

		// Should not panic
		manager.Unregister("non-existent")
	})
}

// TestManagerGetAgent tests retrieving a specific agent.
func TestManagerGetAgent(t *testing.T) {
	t.Run("returns agent when exists", func(t *testing.T) {
		manager := NewManager(slog.Default())
		stream := newMockStream()
		conn := NewConnection(ConnectionParams{ID: "agent-1", Name: "Test Agent", Capabilities: []string{"chat"}, Stream: stream, Logger: slog.Default()})

		manager.Register(conn)

		agent, ok := manager.GetAgent("agent-1")
		if !ok {
			t.Fatal("expected to find agent")
		}
		if agent.ID != "agent-1" {
			t.Errorf("expected agent-1, got %s", agent.ID)
		}
	})

	t.Run("returns false when agent not found", func(t *testing.T) {
		manager := NewManager(slog.Default())

		_, ok := manager.GetAgent("non-existent")
		if ok {
			t.Error("expected not to find agent")
		}
	})
}

// TestManagerListAgents tests listing all agents.
func TestManagerListAgents(t *testing.T) {
	t.Run("returns empty list when no agents", func(t *testing.T) {
		manager := NewManager(slog.Default())

		agents := manager.ListAgents()
		if len(agents) != 0 {
			t.Errorf("expected 0 agents, got %d", len(agents))
		}
	})

	t.Run("returns all registered agents", func(t *testing.T) {
		manager := NewManager(slog.Default())

		for i := 1; i <= 3; i++ {
			stream := newMockStream()
			conn := NewConnection(ConnectionParams{
				ID:           fmt.Sprintf("agent-%d", i),
				Name:         fmt.Sprintf("Agent %d", i),
				Capabilities: []string{"chat"},
				Stream:       stream,
				Logger:       slog.Default(),
			})
			manager.Register(conn)
		}

		agents := manager.ListAgents()
		if len(agents) != 3 {
			t.Errorf("expected 3 agents, got %d", len(agents))
		}
	})
}

// TestManagerSendMessage tests sending messages through the manager.
func TestManagerSendMessage(t *testing.T) {
	t.Run("returns error when agent_id is missing", func(t *testing.T) {
		manager := NewManager(slog.Default())
		stream := newMockStream()
		conn := NewConnection(ConnectionParams{ID: "agent-1", Name: "Test Agent", Capabilities: []string{"chat"}, Stream: stream, Logger: slog.Default()})
		manager.Register(conn)

		req := &SendRequest{
			ThreadID: "thread-1",
			Sender:   "user@test.com",
			Content:  "Hello",
			// AgentID is intentionally not set
		}

		_, err := manager.SendMessage(context.Background(), req)
		if err == nil {
			t.Error("expected error when agent_id is missing")
		}
		if err.Error() != "agent_id is required" {
			t.Errorf("expected 'agent_id is required' error, got %v", err)
		}
	})

	t.Run("returns error when specified agent not found", func(t *testing.T) {
		manager := NewManager(slog.Default())

		req := &SendRequest{
			ThreadID: "thread-1",
			Sender:   "user@test.com",
			Content:  "Hello",
			AgentID:  "nonexistent-agent",
		}

		_, err := manager.SendMessage(context.Background(), req)
		if err == nil {
			t.Error("expected error when agent not found")
		}
		if err != ErrAgentNotFound {
			t.Errorf("expected ErrAgentNotFound, got %v", err)
		}
	})

	t.Run("sends message to specified agent and returns response channel", func(t *testing.T) {
		manager := NewManager(slog.Default())
		stream := newMockStream()
		conn := NewConnection(ConnectionParams{ID: "agent-1", Name: "Test Agent", Capabilities: []string{"chat"}, Stream: stream, Logger: slog.Default()})
		manager.Register(conn)

		req := &SendRequest{
			ThreadID: "thread-1",
			Sender:   "user@test.com",
			Content:  "Hello, agent!",
			AgentID:  "agent-1",
		}

		respChan, err := manager.SendMessage(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if respChan == nil {
			t.Fatal("expected response channel, got nil")
		}

		// Verify the message was sent
		sent := stream.getSentMessages()
		if len(sent) != 1 {
			t.Fatalf("expected 1 message sent, got %d", len(sent))
		}

		sendMsg := sent[0].GetSendMessage()
		if sendMsg == nil {
			t.Fatal("expected SendMessage payload")
		}
		if sendMsg.GetContent() != "Hello, agent!" {
			t.Errorf("expected 'Hello, agent!', got '%s'", sendMsg.GetContent())
		}
		if sendMsg.GetThreadId() != "thread-1" {
			t.Errorf("expected thread-1, got %s", sendMsg.GetThreadId())
		}
		if sendMsg.GetSender() != "user@test.com" {
			t.Errorf("expected user@test.com, got %s", sendMsg.GetSender())
		}
	})

	t.Run("sends message with attachments", func(t *testing.T) {
		manager := NewManager(slog.Default())
		stream := newMockStream()
		conn := NewConnection(ConnectionParams{ID: "agent-1", Name: "Test Agent", Capabilities: []string{"chat"}, Stream: stream, Logger: slog.Default()})
		manager.Register(conn)

		req := &SendRequest{
			ThreadID: "thread-1",
			Sender:   "user@test.com",
			Content:  "Here is a file",
			AgentID:  "agent-1",
			Attachments: []Attachment{
				{
					Filename: "test.txt",
					MimeType: "text/plain",
					Data:     []byte("file contents"),
				},
			},
		}

		_, err := manager.SendMessage(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		sent := stream.getSentMessages()
		sendMsg := sent[0].GetSendMessage()
		if len(sendMsg.GetAttachments()) != 1 {
			t.Fatalf("expected 1 attachment, got %d", len(sendMsg.GetAttachments()))
		}

		attachment := sendMsg.GetAttachments()[0]
		if attachment.GetFilename() != "test.txt" {
			t.Errorf("expected test.txt, got %s", attachment.GetFilename())
		}
		if attachment.GetMimeType() != "text/plain" {
			t.Errorf("expected text/plain, got %s", attachment.GetMimeType())
		}
	})

	t.Run("generates unique request ID", func(t *testing.T) {
		manager := NewManager(slog.Default())
		stream := newMockStream()
		conn := NewConnection(ConnectionParams{ID: "agent-1", Name: "Test Agent", Capabilities: []string{"chat"}, Stream: stream, Logger: slog.Default()})
		manager.Register(conn)

		req := &SendRequest{
			ThreadID: "thread-1",
			Sender:   "user@test.com",
			Content:  "Hello",
			AgentID:  "agent-1",
		}

		manager.SendMessage(context.Background(), req)
		manager.SendMessage(context.Background(), req)

		sent := stream.getSentMessages()
		if len(sent) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(sent))
		}

		reqID1 := sent[0].GetSendMessage().GetRequestId()
		reqID2 := sent[1].GetSendMessage().GetRequestId()

		if reqID1 == "" || reqID2 == "" {
			t.Error("request IDs should not be empty")
		}
		if reqID1 == reqID2 {
			t.Error("request IDs should be unique")
		}
	})
}

// TestManagerSendMessageTargeting tests sending to a specific agent by ID with multiple agents.
func TestManagerSendMessageTargeting(t *testing.T) {
	t.Run("sends to specific agent when multiple agents available", func(t *testing.T) {
		manager := NewManager(slog.Default())
		stream1 := newMockStream()
		stream2 := newMockStream()
		conn1 := NewConnection(ConnectionParams{ID: "agent-1", Name: "Agent One", Capabilities: []string{"chat"}, Stream: stream1, Logger: slog.Default()})
		conn2 := NewConnection(ConnectionParams{ID: "agent-2", Name: "Agent Two", Capabilities: []string{"chat"}, Stream: stream2, Logger: slog.Default()})
		manager.Register(conn1)
		manager.Register(conn2)

		req := &SendRequest{
			ThreadID: "thread-1",
			Sender:   "user@test.com",
			Content:  "Hello",
			AgentID:  "agent-2", // Target specific agent
		}

		_, err := manager.SendMessage(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify message was sent to agent-2, not agent-1
		sent1 := stream1.getSentMessages()
		sent2 := stream2.getSentMessages()

		if len(sent1) != 0 {
			t.Errorf("expected 0 messages to agent-1, got %d", len(sent1))
		}
		if len(sent2) != 1 {
			t.Errorf("expected 1 message to agent-2, got %d", len(sent2))
		}
	})
}

// TestAgentInfo tests the AgentInfo struct.
func TestAgentInfo(t *testing.T) {
	t.Run("contains correct information", func(t *testing.T) {
		manager := NewManager(slog.Default())
		stream := newMockStream()
		conn := NewConnection(ConnectionParams{ID: "agent-1", Name: "Test Agent", Capabilities: []string{"chat", "code"}, Stream: stream, Logger: slog.Default()})
		manager.Register(conn)

		agents := manager.ListAgents()
		info := agents[0]

		if info.ID != "agent-1" {
			t.Errorf("expected agent-1, got %s", info.ID)
		}
		if info.Name != "Test Agent" {
			t.Errorf("expected 'Test Agent', got '%s'", info.Name)
		}
		if len(info.Capabilities) != 2 {
			t.Errorf("expected 2 capabilities, got %d", len(info.Capabilities))
		}
	})
}

// TestResponseTypes tests Response struct and events.
func TestResponseTypes(t *testing.T) {
	t.Run("response event types are distinct", func(t *testing.T) {
		events := []ResponseEvent{
			EventThinking,
			EventText,
			EventToolUse,
			EventToolResult,
			EventFile,
			EventDone,
			EventError,
		}

		seen := make(map[ResponseEvent]bool)
		for _, e := range events {
			if seen[e] {
				t.Errorf("duplicate event type: %d", e)
			}
			seen[e] = true
		}
	})
}

// TestManagerGetByInstanceID tests retrieving an agent by instance ID.
func TestManagerGetByInstanceID(t *testing.T) {
	t.Run("returns agent when instance ID exists", func(t *testing.T) {
		manager := NewManager(slog.Default())
		stream := newMockStream()
		conn := NewConnection(ConnectionParams{
			ID:           "bob-projects-website",
			Name:         "bob",
			Capabilities: []string{"chat"},
			PrincipalID:  "principal-uuid",
			WorkingDir:   "/projects/website",
			InstanceID:   "0fb8187d-c06",
			Stream:       stream,
			Logger:       slog.Default(),
		})

		manager.Register(conn)

		// Lookup by instance ID
		got := manager.GetByInstanceID("0fb8187d-c06")
		if got == nil {
			t.Fatal("expected to find agent by instance ID")
		}
		if got.ID != "bob-projects-website" {
			t.Errorf("expected ID 'bob-projects-website', got '%s'", got.ID)
		}
		if got.WorkingDir != "/projects/website" {
			t.Errorf("expected WorkingDir '/projects/website', got '%s'", got.WorkingDir)
		}
	})

	t.Run("returns nil when instance ID not found", func(t *testing.T) {
		manager := NewManager(slog.Default())

		got := manager.GetByInstanceID("nonexistent")
		if got != nil {
			t.Error("expected nil for non-existent instance ID")
		}
	})

	t.Run("finds correct agent among multiple", func(t *testing.T) {
		manager := NewManager(slog.Default())

		// Register multiple agents
		for i, instanceID := range []string{"aaa111", "bbb222", "ccc333"} {
			stream := newMockStream()
			conn := NewConnection(ConnectionParams{
				ID:         fmt.Sprintf("agent-%d", i),
				Name:       fmt.Sprintf("Agent %d", i),
				InstanceID: instanceID,
				Stream:     stream,
				Logger:     slog.Default(),
			})
			manager.Register(conn)
		}

		// Find the middle one
		got := manager.GetByInstanceID("bbb222")
		if got == nil {
			t.Fatal("expected to find agent")
		}
		if got.ID != "agent-1" {
			t.Errorf("expected 'agent-1', got '%s'", got.ID)
		}
	})
}

// TestManagerGetByPrincipalAndWorkDir tests retrieving an agent by principal ID and working dir.
func TestManagerGetByPrincipalAndWorkDir(t *testing.T) {
	t.Run("returns agent when principal and workdir match", func(t *testing.T) {
		manager := NewManager(slog.Default())
		stream := newMockStream()
		conn := NewConnection(ConnectionParams{
			ID:          "bob-projects-website",
			Name:        "bob",
			PrincipalID: "principal-uuid",
			WorkingDir:  "/projects/website",
			Stream:      stream,
			Logger:      slog.Default(),
		})

		manager.Register(conn)

		// Exact match
		got := manager.GetByPrincipalAndWorkDir("principal-uuid", "/projects/website")
		if got == nil {
			t.Fatal("expected to find agent by principal and workdir")
		}
		if got.ID != "bob-projects-website" {
			t.Errorf("expected ID 'bob-projects-website', got '%s'", got.ID)
		}
	})

	t.Run("returns nil when workdir does not match", func(t *testing.T) {
		manager := NewManager(slog.Default())
		stream := newMockStream()
		conn := NewConnection(ConnectionParams{
			ID:          "bob-projects-website",
			Name:        "bob",
			PrincipalID: "principal-uuid",
			WorkingDir:  "/projects/website",
			Stream:      stream,
			Logger:      slog.Default(),
		})

		manager.Register(conn)

		got := manager.GetByPrincipalAndWorkDir("principal-uuid", "/other/dir")
		if got != nil {
			t.Error("expected nil when workdir does not match")
		}
	})

	t.Run("returns nil when principal does not match", func(t *testing.T) {
		manager := NewManager(slog.Default())
		stream := newMockStream()
		conn := NewConnection(ConnectionParams{
			ID:          "bob-projects-website",
			Name:        "bob",
			PrincipalID: "principal-uuid",
			WorkingDir:  "/projects/website",
			Stream:      stream,
			Logger:      slog.Default(),
		})

		manager.Register(conn)

		got := manager.GetByPrincipalAndWorkDir("other-uuid", "/projects/website")
		if got != nil {
			t.Error("expected nil when principal does not match")
		}
	})

	t.Run("finds correct agent among multiple", func(t *testing.T) {
		manager := NewManager(slog.Default())

		// Register multiple agents with different principal/workdir combos
		testCases := []struct {
			id          string
			principalID string
			workingDir  string
		}{
			{"agent-1", "alice", "/home/alice/proj1"},
			{"agent-2", "alice", "/home/alice/proj2"},
			{"agent-3", "bob", "/home/bob/proj1"},
		}

		for _, tc := range testCases {
			stream := newMockStream()
			conn := NewConnection(ConnectionParams{
				ID:          tc.id,
				Name:        "Agent",
				PrincipalID: tc.principalID,
				WorkingDir:  tc.workingDir,
				Stream:      stream,
				Logger:      slog.Default(),
			})
			manager.Register(conn)
		}

		// Find alice's second project
		got := manager.GetByPrincipalAndWorkDir("alice", "/home/alice/proj2")
		if got == nil {
			t.Fatal("expected to find agent")
		}
		if got.ID != "agent-2" {
			t.Errorf("expected 'agent-2', got '%s'", got.ID)
		}
	})
}

// TestConcurrentAccess tests thread safety of the Manager.
func TestConcurrentAccess(t *testing.T) {
	t.Run("handles concurrent register and list", func(t *testing.T) {
		manager := NewManager(slog.Default())
		var wg sync.WaitGroup

		// Concurrent registrations
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				stream := newMockStream()
				conn := NewConnection(ConnectionParams{
					ID:           fmt.Sprintf("agent-%d", id),
					Name:         "Agent",
					Capabilities: []string{"chat"},
					Stream:       stream,
					Logger:       slog.Default(),
				})
				manager.Register(conn)
			}(i)
		}

		// Concurrent list operations
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				manager.ListAgents()
			}()
		}

		wg.Wait()
	})
}
