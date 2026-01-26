// ABOUTME: Tests for ConversationService
// ABOUTME: Verifies message persistence, thread management, and response streaming

package conversation

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/2389/coven-gateway/internal/agent"
	"github.com/2389/coven-gateway/internal/store"
)

// mockSender implements MessageSender for testing
type mockSender struct {
	responses []*agent.Response
	err       error
	lastReq   *agent.SendRequest
}

func (m *mockSender) SendMessage(ctx context.Context, req *agent.SendRequest) (<-chan *agent.Response, error) {
	m.lastReq = req
	if m.err != nil {
		return nil, m.err
	}

	ch := make(chan *agent.Response, len(m.responses))
	for _, resp := range m.responses {
		ch <- resp
	}
	close(ch)
	return ch, nil
}

func createTestStore(t *testing.T) *store.SQLiteStore {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	s, err := store.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestService_SendMessage_RecordsUserMessageFirst(t *testing.T) {
	// Setup
	testStore := createTestStore(t)
	sender := &mockSender{
		responses: []*agent.Response{
			{Event: agent.EventText, Text: "Hello"},
			{Event: agent.EventDone, Text: "Hello", Done: true},
		},
	}
	svc := New(testStore, sender, nil)

	// Send message
	ctx := context.Background()
	resp, err := svc.SendMessage(ctx, &SendRequest{
		AgentID: "test-agent",
		Sender:  "user",
		Content: "Hi there",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Drain the response channel
	for range resp.Stream {
	}

	// Give persistence goroutine time to complete
	time.Sleep(100 * time.Millisecond)

	// Verify user message was saved
	messages, err := testStore.GetThreadMessages(ctx, resp.ThreadID, 10)
	require.NoError(t, err)

	// Should have at least the user message
	require.GreaterOrEqual(t, len(messages), 1)

	// Find the user message (don't assume order)
	var userMsg *store.Message
	for _, msg := range messages {
		if msg.Sender == "user" {
			userMsg = msg
			break
		}
	}

	require.NotNil(t, userMsg, "user message not found")
	assert.Equal(t, "user", userMsg.Sender)
	assert.Equal(t, "Hi there", userMsg.Content)
	assert.Equal(t, store.MessageTypeMessage, userMsg.Type)
}

func TestService_SendMessage_CreatesThread(t *testing.T) {
	testStore := createTestStore(t)
	sender := &mockSender{
		responses: []*agent.Response{
			{Event: agent.EventDone, Done: true},
		},
	}
	svc := New(testStore, sender, nil)

	ctx := context.Background()
	resp, err := svc.SendMessage(ctx, &SendRequest{
		AgentID:      "test-agent",
		FrontendName: "test-frontend",
		ExternalID:   "channel-123",
		Sender:       "user",
		Content:      "Hello",
	})
	require.NoError(t, err)

	// Drain responses
	for range resp.Stream {
	}

	// Verify thread was created
	thread, err := testStore.GetThread(ctx, resp.ThreadID)
	require.NoError(t, err)
	assert.Equal(t, "test-agent", thread.AgentID)
	assert.Equal(t, "test-frontend", thread.FrontendName)
	assert.Equal(t, "channel-123", thread.ExternalID)
}

func TestService_SendMessage_ReusesExistingThread(t *testing.T) {
	testStore := createTestStore(t)
	sender := &mockSender{
		responses: []*agent.Response{
			{Event: agent.EventDone, Done: true},
		},
	}
	svc := New(testStore, sender, nil)

	ctx := context.Background()

	// Send first message
	resp1, err := svc.SendMessage(ctx, &SendRequest{
		AgentID:      "test-agent",
		FrontendName: "test-frontend",
		ExternalID:   "channel-123",
		Sender:       "user",
		Content:      "First message",
	})
	require.NoError(t, err)
	for range resp1.Stream {
	}

	// Send second message with same frontend/external ID
	sender.responses = []*agent.Response{
		{Event: agent.EventDone, Done: true},
	}
	resp2, err := svc.SendMessage(ctx, &SendRequest{
		AgentID:      "test-agent",
		FrontendName: "test-frontend",
		ExternalID:   "channel-123",
		Sender:       "user",
		Content:      "Second message",
	})
	require.NoError(t, err)
	for range resp2.Stream {
	}

	// Should use same thread
	assert.Equal(t, resp1.ThreadID, resp2.ThreadID)

	// Should have 2 user messages in thread
	messages, err := testStore.GetThreadMessages(ctx, resp1.ThreadID, 10)
	require.NoError(t, err)

	userMsgCount := 0
	for _, msg := range messages {
		if msg.Sender == "user" {
			userMsgCount++
		}
	}
	assert.Equal(t, 2, userMsgCount)
}

func TestService_SendMessage_UsesProvidedThreadID(t *testing.T) {
	testStore := createTestStore(t)
	sender := &mockSender{
		responses: []*agent.Response{
			{Event: agent.EventDone, Done: true},
		},
	}
	svc := New(testStore, sender, nil)

	ctx := context.Background()
	customThreadID := "custom-thread-id-123"

	resp, err := svc.SendMessage(ctx, &SendRequest{
		ThreadID: customThreadID,
		AgentID:  "test-agent",
		Sender:   "user",
		Content:  "Hello",
	})
	require.NoError(t, err)
	for range resp.Stream {
	}

	assert.Equal(t, customThreadID, resp.ThreadID)

	// Thread should exist with this ID
	thread, err := testStore.GetThread(ctx, customThreadID)
	require.NoError(t, err)
	assert.Equal(t, customThreadID, thread.ID)
}

func TestService_SendMessage_PersistsToolUse(t *testing.T) {
	testStore := createTestStore(t)
	sender := &mockSender{
		responses: []*agent.Response{
			{
				Event: agent.EventToolUse,
				ToolUse: &agent.ToolUseEvent{
					ID:        "tool-123",
					Name:      "read_file",
					InputJSON: `{"path": "/tmp/test.txt"}`,
				},
			},
			{
				Event: agent.EventToolResult,
				ToolResult: &agent.ToolResultEvent{
					ID:     "tool-123",
					Output: "file contents here",
				},
			},
			{Event: agent.EventDone, Done: true},
		},
	}
	svc := New(testStore, sender, nil)

	ctx := context.Background()
	resp, err := svc.SendMessage(ctx, &SendRequest{
		AgentID: "test-agent",
		Sender:  "user",
		Content: "Read a file",
	})
	require.NoError(t, err)

	// Drain responses
	for range resp.Stream {
	}

	// Give persistence goroutine time to complete
	time.Sleep(100 * time.Millisecond)

	// Verify tool events were saved
	messages, err := testStore.GetThreadMessages(ctx, resp.ThreadID, 10)
	require.NoError(t, err)

	var toolUseMsg, toolResultMsg *store.Message
	for _, msg := range messages {
		switch msg.Type {
		case store.MessageTypeToolUse:
			toolUseMsg = msg
		case store.MessageTypeToolResult:
			toolResultMsg = msg
		}
	}

	require.NotNil(t, toolUseMsg, "tool_use message not found")
	assert.Equal(t, "read_file", toolUseMsg.ToolName)
	assert.Equal(t, "tool-123", toolUseMsg.ToolID)
	assert.Contains(t, toolUseMsg.Content, "/tmp/test.txt")

	require.NotNil(t, toolResultMsg, "tool_result message not found")
	assert.Equal(t, "tool-123", toolResultMsg.ToolID)
	assert.Equal(t, "file contents here", toolResultMsg.Content)
}

func TestService_SendMessage_AccumulatesStreamingText(t *testing.T) {
	testStore := createTestStore(t)
	sender := &mockSender{
		responses: []*agent.Response{
			{Event: agent.EventText, Text: "Hello "},
			{Event: agent.EventText, Text: "world"},
			{Event: agent.EventText, Text: "!"},
			{Event: agent.EventDone, Done: true},
		},
	}
	svc := New(testStore, sender, nil)

	ctx := context.Background()
	resp, err := svc.SendMessage(ctx, &SendRequest{
		AgentID: "test-agent",
		Sender:  "user",
		Content: "Say hello",
	})
	require.NoError(t, err)

	// Drain responses
	for range resp.Stream {
	}

	// Give persistence goroutine time to complete
	time.Sleep(100 * time.Millisecond)

	// Verify accumulated text was saved
	messages, err := testStore.GetThreadMessages(ctx, resp.ThreadID, 10)
	require.NoError(t, err)

	var agentMsg *store.Message
	for _, msg := range messages {
		if msg.Sender == "agent:test-agent" && msg.Type == store.MessageTypeMessage {
			agentMsg = msg
			break
		}
	}

	require.NotNil(t, agentMsg, "agent message not found")
	assert.Equal(t, "Hello world!", agentMsg.Content)
}

func TestService_SendMessage_RequiresAgentID(t *testing.T) {
	testStore := createTestStore(t)
	sender := &mockSender{}
	svc := New(testStore, sender, nil)

	ctx := context.Background()
	_, err := svc.SendMessage(ctx, &SendRequest{
		Sender:  "user",
		Content: "Hello",
		// AgentID missing
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent_id")
}

func TestService_SendMessage_ForwardsToSender(t *testing.T) {
	testStore := createTestStore(t)
	sender := &mockSender{
		responses: []*agent.Response{
			{Event: agent.EventDone, Done: true},
		},
	}
	svc := New(testStore, sender, nil)

	ctx := context.Background()
	_, err := svc.SendMessage(ctx, &SendRequest{
		AgentID: "test-agent",
		Sender:  "user",
		Content: "Hello",
		Attachments: []agent.Attachment{
			{Filename: "test.txt", Data: []byte("content")},
		},
	})
	require.NoError(t, err)

	// Verify the sender received the request
	require.NotNil(t, sender.lastReq)
	assert.Equal(t, "test-agent", sender.lastReq.AgentID)
	assert.Equal(t, "user", sender.lastReq.Sender)
	assert.Equal(t, "Hello", sender.lastReq.Content)
	assert.Len(t, sender.lastReq.Attachments, 1)
}

func TestService_GetHistory(t *testing.T) {
	testStore := createTestStore(t)
	sender := &mockSender{
		responses: []*agent.Response{
			{Event: agent.EventText, Text: "Response"},
			{Event: agent.EventDone, Done: true},
		},
	}
	svc := New(testStore, sender, nil)

	ctx := context.Background()
	resp, err := svc.SendMessage(ctx, &SendRequest{
		AgentID: "test-agent",
		Sender:  "user",
		Content: "Hello",
	})
	require.NoError(t, err)
	for range resp.Stream {
	}

	// Give persistence time
	time.Sleep(100 * time.Millisecond)

	// Get history through service
	messages, err := svc.GetHistory(ctx, resp.ThreadID, 10)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(messages), 1)
}

func TestService_GetThread(t *testing.T) {
	testStore := createTestStore(t)
	sender := &mockSender{
		responses: []*agent.Response{
			{Event: agent.EventDone, Done: true},
		},
	}
	svc := New(testStore, sender, nil)

	ctx := context.Background()
	resp, err := svc.SendMessage(ctx, &SendRequest{
		AgentID:      "test-agent",
		FrontendName: "matrix",
		ExternalID:   "!room:server",
		Sender:       "user",
		Content:      "Hello",
	})
	require.NoError(t, err)
	for range resp.Stream {
	}

	// Get thread through service
	thread, err := svc.GetThread(ctx, resp.ThreadID)
	require.NoError(t, err)
	assert.Equal(t, resp.ThreadID, thread.ID)
	assert.Equal(t, "test-agent", thread.AgentID)
	assert.Equal(t, "matrix", thread.FrontendName)
}

func TestService_SendMessage_SenderError(t *testing.T) {
	testStore := createTestStore(t)
	sender := &mockSender{
		err: agent.ErrAgentNotFound,
	}
	svc := New(testStore, sender, nil)

	ctx := context.Background()
	_, err := svc.SendMessage(ctx, &SendRequest{
		AgentID: "nonexistent-agent",
		Sender:  "user",
		Content: "Hello",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent send failed")

	// User message should still be saved even though agent failed
	// (this is the "record first" principle)
	threads, err := testStore.ListThreads(ctx, 10)
	require.NoError(t, err)
	require.Len(t, threads, 1)

	messages, err := testStore.GetThreadMessages(ctx, threads[0].ID, 10)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	assert.Equal(t, "user", messages[0].Sender)
	assert.Equal(t, "Hello", messages[0].Content)
}
