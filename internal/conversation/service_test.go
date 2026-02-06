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
	svc := New(testStore, sender, nil, nil)

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

	// Verify user message was saved (now in ledger_events)
	events, err := testStore.GetEventsByThreadID(ctx, resp.ThreadID, 10)
	require.NoError(t, err)

	// Should have at least the user message
	require.GreaterOrEqual(t, len(events), 1)

	// Find the user event (don't assume order)
	var userEvt *store.LedgerEvent
	for _, evt := range events {
		if evt.Author == "user" {
			userEvt = evt
			break
		}
	}

	require.NotNil(t, userEvt, "user event not found")
	assert.Equal(t, "user", userEvt.Author)
	require.NotNil(t, userEvt.Text)
	assert.Equal(t, "Hi there", *userEvt.Text)
	assert.Equal(t, store.EventTypeMessage, userEvt.Type)
}

func TestService_SendMessage_CreatesThread(t *testing.T) {
	testStore := createTestStore(t)
	sender := &mockSender{
		responses: []*agent.Response{
			{Event: agent.EventDone, Done: true},
		},
	}
	svc := New(testStore, sender, nil, nil)

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
	svc := New(testStore, sender, nil, nil)

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

	// Should have 2 user events in thread (now in ledger_events)
	events, err := testStore.GetEventsByThreadID(ctx, resp1.ThreadID, 10)
	require.NoError(t, err)

	userEvtCount := 0
	for _, evt := range events {
		if evt.Author == "user" {
			userEvtCount++
		}
	}
	assert.Equal(t, 2, userEvtCount)
}

func TestService_SendMessage_UsesProvidedThreadID(t *testing.T) {
	testStore := createTestStore(t)
	sender := &mockSender{
		responses: []*agent.Response{
			{Event: agent.EventDone, Done: true},
		},
	}
	svc := New(testStore, sender, nil, nil)

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
	svc := New(testStore, sender, nil, nil)

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

	// Verify tool events were saved (now in ledger_events)
	events, err := testStore.GetEventsByThreadID(ctx, resp.ThreadID, 10)
	require.NoError(t, err)

	var toolUseEvt, toolResultEvt *store.LedgerEvent
	for _, evt := range events {
		switch evt.Type {
		case store.EventTypeToolCall:
			toolUseEvt = evt
		case store.EventTypeToolResult:
			toolResultEvt = evt
		}
	}

	require.NotNil(t, toolUseEvt, "tool_call event not found")
	require.NotNil(t, toolUseEvt.Text)
	assert.Contains(t, *toolUseEvt.Text, "read_file")
	assert.Contains(t, *toolUseEvt.Text, "tool-123")
	assert.Contains(t, *toolUseEvt.Text, "/tmp/test.txt")

	require.NotNil(t, toolResultEvt, "tool_result event not found")
	require.NotNil(t, toolResultEvt.Text)
	assert.Contains(t, *toolResultEvt.Text, "tool-123")
	assert.Contains(t, *toolResultEvt.Text, "file contents here")
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
	svc := New(testStore, sender, nil, nil)

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

	// Verify accumulated text was saved (now in ledger_events)
	events, err := testStore.GetEventsByThreadID(ctx, resp.ThreadID, 10)
	require.NoError(t, err)

	var agentEvt *store.LedgerEvent
	for _, evt := range events {
		if evt.Author == "agent:test-agent" && evt.Type == store.EventTypeMessage {
			agentEvt = evt
			break
		}
	}

	require.NotNil(t, agentEvt, "agent event not found")
	require.NotNil(t, agentEvt.Text)
	assert.Equal(t, "Hello world!", *agentEvt.Text)
}

func TestService_SendMessage_RequiresAgentID(t *testing.T) {
	testStore := createTestStore(t)
	sender := &mockSender{}
	svc := New(testStore, sender, nil, nil)

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
	svc := New(testStore, sender, nil, nil)

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
	svc := New(testStore, sender, nil, nil)

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
	svc := New(testStore, sender, nil, nil)

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
	svc := New(testStore, sender, nil, nil)

	ctx := context.Background()
	_, err := svc.SendMessage(ctx, &SendRequest{
		AgentID: "nonexistent-agent",
		Sender:  "user",
		Content: "Hello",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent send failed")

	// User message should still be saved even though agent failed
	// (this is the "record first" principle) - now in ledger_events
	threads, err := testStore.ListThreads(ctx, 10)
	require.NoError(t, err)
	require.Len(t, threads, 1)

	events, err := testStore.GetEventsByThreadID(ctx, threads[0].ID, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "user", events[0].Author)
	require.NotNil(t, events[0].Text)
	assert.Equal(t, "Hello", *events[0].Text)
}
