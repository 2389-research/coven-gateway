// ABOUTME: Tests for the agent management layer including Manager, Connection, and Router.
// ABOUTME: Validates registration, message routing, and response handling functionality.

package agent

import (
	"context"
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

// TestRouterSelectAgent tests the round-robin router selection.
func TestRouterSelectAgent(t *testing.T) {
	t.Run("returns error when no agents available", func(t *testing.T) {
		router := NewRouter()
		agents := []*Connection{}

		_, err := router.SelectAgent(agents)
		if err == nil {
			t.Error("expected error when no agents available")
		}
		if err != ErrNoAgentsAvailable {
			t.Errorf("expected ErrNoAgentsAvailable, got %v", err)
		}
	})

	t.Run("selects single agent", func(t *testing.T) {
		router := NewRouter()
		stream := newMockStream()
		agent := NewConnection("agent-1", "Agent One", []string{"chat"}, stream, slog.Default())

		selected, err := router.SelectAgent([]*Connection{agent})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if selected.ID != "agent-1" {
			t.Errorf("expected agent-1, got %s", selected.ID)
		}
	})

	t.Run("round-robin selects agents in order", func(t *testing.T) {
		router := NewRouter()
		stream1 := newMockStream()
		stream2 := newMockStream()
		stream3 := newMockStream()

		agents := []*Connection{
			NewConnection("agent-1", "Agent One", []string{"chat"}, stream1, slog.Default()),
			NewConnection("agent-2", "Agent Two", []string{"chat"}, stream2, slog.Default()),
			NewConnection("agent-3", "Agent Three", []string{"chat"}, stream3, slog.Default()),
		}

		// First cycle
		selected1, _ := router.SelectAgent(agents)
		selected2, _ := router.SelectAgent(agents)
		selected3, _ := router.SelectAgent(agents)
		// Second cycle should wrap around
		selected4, _ := router.SelectAgent(agents)

		if selected1.ID != "agent-1" {
			t.Errorf("first selection: expected agent-1, got %s", selected1.ID)
		}
		if selected2.ID != "agent-2" {
			t.Errorf("second selection: expected agent-2, got %s", selected2.ID)
		}
		if selected3.ID != "agent-3" {
			t.Errorf("third selection: expected agent-3, got %s", selected3.ID)
		}
		if selected4.ID != "agent-1" {
			t.Errorf("fourth selection (wrap): expected agent-1, got %s", selected4.ID)
		}
	})
}

// TestConnectionSend tests sending messages through a connection.
func TestConnectionSend(t *testing.T) {
	t.Run("sends message to stream", func(t *testing.T) {
		stream := newMockStream()
		conn := NewConnection("agent-1", "Test Agent", []string{"chat"}, stream, slog.Default())

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
		conn := NewConnection("agent-1", "Test Agent", []string{"chat"}, stream, slog.Default())

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
		conn := NewConnection("agent-1", "Test Agent", []string{"chat"}, stream, slog.Default())

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
		conn := NewConnection("agent-1", "Test Agent", []string{"chat"}, stream, slog.Default())

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
		conn := NewConnection("agent-1", "Test Agent", []string{"chat", "code"}, stream, slog.Default())

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

		conn1 := NewConnection("agent-1", "Agent One", []string{"chat"}, stream1, slog.Default())
		conn2 := NewConnection("agent-1", "Agent One Duplicate", []string{"chat"}, stream2, slog.Default())

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
		conn := NewConnection("agent-1", "Test Agent", []string{"chat"}, stream, slog.Default())

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
		conn := NewConnection("agent-1", "Test Agent", []string{"chat"}, stream, slog.Default())

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
			conn := NewConnection(
				"agent-"+string(rune('0'+i)),
				"Agent "+string(rune('0'+i)),
				[]string{"chat"},
				stream,
				slog.Default(),
			)
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
	t.Run("returns error when no agents available", func(t *testing.T) {
		manager := NewManager(slog.Default())

		req := &SendRequest{
			ThreadID: "thread-1",
			Sender:   "user@test.com",
			Content:  "Hello",
		}

		_, err := manager.SendMessage(context.Background(), req)
		if err == nil {
			t.Error("expected error when no agents available")
		}
		if err != ErrNoAgentsAvailable {
			t.Errorf("expected ErrNoAgentsAvailable, got %v", err)
		}
	})

	t.Run("sends message to selected agent and returns response channel", func(t *testing.T) {
		manager := NewManager(slog.Default())
		stream := newMockStream()
		conn := NewConnection("agent-1", "Test Agent", []string{"chat"}, stream, slog.Default())
		manager.Register(conn)

		req := &SendRequest{
			ThreadID: "thread-1",
			Sender:   "user@test.com",
			Content:  "Hello, agent!",
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
		conn := NewConnection("agent-1", "Test Agent", []string{"chat"}, stream, slog.Default())
		manager.Register(conn)

		req := &SendRequest{
			ThreadID: "thread-1",
			Sender:   "user@test.com",
			Content:  "Here is a file",
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
		conn := NewConnection("agent-1", "Test Agent", []string{"chat"}, stream, slog.Default())
		manager.Register(conn)

		req := &SendRequest{
			ThreadID: "thread-1",
			Sender:   "user@test.com",
			Content:  "Hello",
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

// TestAgentInfo tests the AgentInfo struct.
func TestAgentInfo(t *testing.T) {
	t.Run("contains correct information", func(t *testing.T) {
		manager := NewManager(slog.Default())
		stream := newMockStream()
		conn := NewConnection("agent-1", "Test Agent", []string{"chat", "code"}, stream, slog.Default())
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
				conn := NewConnection(
					"agent-"+string(rune('a'+id)),
					"Agent",
					[]string{"chat"},
					stream,
					slog.Default(),
				)
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
