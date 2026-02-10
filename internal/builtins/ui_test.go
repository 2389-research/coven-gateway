// ABOUTME: Tests for UI pack tool handlers.
// ABOUTME: Tests the ask_user tool and question routing.

package builtins

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/2389/coven-gateway/internal/packs"
	pb "github.com/2389/coven-gateway/proto/coven"
)

// mockClientStreamer records sent questions for testing (thread-safe).
type mockClientStreamer struct {
	mu        sync.Mutex
	questions []*pb.UserQuestionRequest
	sent      chan struct{} // signaled when a question is sent
}

func newMockClientStreamer() *mockClientStreamer {
	return &mockClientStreamer{
		sent: make(chan struct{}, 10),
	}
}

func (m *mockClientStreamer) SendUserQuestion(agentID string, req *pb.UserQuestionRequest) error {
	m.mu.Lock()
	m.questions = append(m.questions, req)
	m.mu.Unlock()

	// Signal that a question was sent
	select {
	case m.sent <- struct{}{}:
	default:
	}
	return nil
}

func (m *mockClientStreamer) getQuestions() []*pb.UserQuestionRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Return a copy to avoid race conditions
	result := make([]*pb.UserQuestionRequest, len(m.questions))
	copy(result, m.questions)
	return result
}

func TestUIPackToolDefinitions(t *testing.T) {
	streamer := newMockClientStreamer()
	router := NewInMemoryQuestionRouter(streamer)
	pack := UIPack(router)

	t.Run("pack has correct ID", func(t *testing.T) {
		if pack.ID != "builtin:ui" {
			t.Errorf("expected pack ID 'builtin:ui', got %q", pack.ID)
		}
	})

	t.Run("has ask_user tool", func(t *testing.T) {
		found := false
		for _, tool := range pack.Tools {
			if tool.Definition.GetName() == "ask_user" {
				found = true
				caps := tool.Definition.GetRequiredCapabilities()
				if len(caps) != 1 || caps[0] != "ui" {
					t.Errorf("ask_user should require only 'ui' capability, got %v", caps)
				}
			}
		}
		if !found {
			t.Error("ask_user tool not found")
		}
	})
}

func TestAskUserValidation(t *testing.T) {
	streamer := newMockClientStreamer()
	router := NewInMemoryQuestionRouter(streamer)
	pack := UIPack(router)

	handler := findAskUserHandler(pack)
	if handler == nil {
		t.Fatal("ask_user handler not found")
	}

	t.Run("requires question", func(t *testing.T) {
		input := json.RawMessage(`{"options": [{"label": "Yes"}]}`)
		_, err := handler(context.Background(), "test-agent", input)
		if err == nil {
			t.Error("expected error when question is missing")
		}
	})

	t.Run("requires at least one option", func(t *testing.T) {
		input := json.RawMessage(`{"question": "Do you want to proceed?", "options": []}`)
		_, err := handler(context.Background(), "test-agent", input)
		if err == nil {
			t.Error("expected error when options is empty")
		}
	})

	t.Run("validates input JSON", func(t *testing.T) {
		_, err := handler(context.Background(), "test-agent", json.RawMessage(`{invalid`))
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})

	t.Run("rejects duplicate option labels", func(t *testing.T) {
		input := json.RawMessage(`{
			"question": "Pick one?",
			"options": [{"label": "Same"}, {"label": "Same"}]
		}`)
		_, err := handler(context.Background(), "test-agent", input)
		if err == nil {
			t.Error("expected error for duplicate option labels")
		}
	})
}

func TestAskUserWithAnswer(t *testing.T) {
	streamer := newMockClientStreamer()
	router := NewInMemoryQuestionRouter(streamer)
	pack := UIPack(router)

	handler := findAskUserHandler(pack)
	if handler == nil {
		t.Fatal("ask_user handler not found")
	}

	input := json.RawMessage(`{
		"question": "Which option?",
		"options": [{"label": "Option A"}, {"label": "Option B"}],
		"timeout_seconds": 5
	}`)

	// Run the handler in a goroutine since it blocks
	resultChan := make(chan struct {
		output json.RawMessage
		err    error
	}, 1)

	go func() {
		output, err := handler(context.Background(), "test-agent", input)
		resultChan <- struct {
			output json.RawMessage
			err    error
		}{output, err}
	}()

	// Wait for the question to be sent using proper synchronization
	select {
	case <-streamer.sent:
		// Question was sent
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for question to be sent")
	}

	// Verify question was sent
	questions := streamer.getQuestions()
	if len(questions) != 1 {
		t.Fatalf("expected 1 question to be sent, got %d", len(questions))
	}

	q := questions[0]
	if q.GetQuestion() != "Which option?" {
		t.Errorf("expected question 'Which option?', got %q", q.GetQuestion())
	}
	if len(q.GetOptions()) != 2 {
		t.Errorf("expected 2 options, got %d", len(q.GetOptions()))
	}

	// Deliver an answer
	answer := &pb.AnswerQuestionRequest{
		AgentId:    "test-agent",
		QuestionId: q.GetQuestionId(),
		Selected:   []string{"Option A"},
	}
	err := router.DeliverAnswer("test-agent", q.GetQuestionId(), answer)
	if err != nil {
		t.Fatalf("failed to deliver answer: %v", err)
	}

	// Wait for result
	select {
	case result := <-resultChan:
		if result.err != nil {
			t.Fatalf("handler error: %v", result.err)
		}

		var output AskUserOutput
		if err := json.Unmarshal(result.output, &output); err != nil {
			t.Fatalf("failed to unmarshal output: %v", err)
		}

		if !output.Answered {
			t.Error("expected answered=true")
		}
		if len(output.Selected) != 1 || output.Selected[0] != "Option A" {
			t.Errorf("expected selected=['Option A'], got %v", output.Selected)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for handler result")
	}
}

func TestAskUserTimeout(t *testing.T) {
	streamer := newMockClientStreamer()
	router := NewInMemoryQuestionRouter(streamer)
	pack := UIPack(router)

	handler := findAskUserHandler(pack)
	if handler == nil {
		t.Fatal("ask_user handler not found")
	}

	input := json.RawMessage(`{
		"question": "Quick question?",
		"options": [{"label": "Yes"}, {"label": "No"}],
		"timeout_seconds": 1
	}`)

	// Run the handler - it should timeout after 1 second
	start := time.Now()
	output, err := handler(context.Background(), "test-agent", input)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	// Verify timeout occurred within reasonable range
	if elapsed < 900*time.Millisecond || elapsed > 2*time.Second {
		t.Errorf("expected timeout around 1s, got %v", elapsed)
	}

	var result AskUserOutput
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}

	if result.Answered {
		t.Error("expected answered=false on timeout")
	}
	if result.Reason != "timeout" {
		t.Errorf("expected reason='timeout', got %q", result.Reason)
	}
}

func TestDeliverAnswerUnknownQuestion(t *testing.T) {
	streamer := newMockClientStreamer()
	router := NewInMemoryQuestionRouter(streamer)

	err := router.DeliverAnswer("test-agent", "unknown-question-id", &pb.AnswerQuestionRequest{})
	if err == nil {
		t.Error("expected error for unknown question ID")
	}
}

func TestDeliverAnswerWrongAgent(t *testing.T) {
	streamer := newMockClientStreamer()
	router := NewInMemoryQuestionRouter(streamer)
	pack := UIPack(router)

	handler := findAskUserHandler(pack)
	if handler == nil {
		t.Fatal("ask_user handler not found")
	}

	input := json.RawMessage(`{
		"question": "Test question?",
		"options": [{"label": "Yes"}, {"label": "No"}],
		"timeout_seconds": 5
	}`)

	// Run the handler in a goroutine since it blocks
	go func() {
		_, _ = handler(context.Background(), "agent-1", input)
	}()

	// Wait for the question to be sent
	select {
	case <-streamer.sent:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for question to be sent")
	}

	questions := streamer.getQuestions()
	if len(questions) != 1 {
		t.Fatalf("expected 1 question, got %d", len(questions))
	}

	q := questions[0]

	// Try to answer with wrong agent ID
	answer := &pb.AnswerQuestionRequest{
		AgentId:    "agent-2", // Wrong agent!
		QuestionId: q.GetQuestionId(),
		Selected:   []string{"Yes"},
	}
	err := router.DeliverAnswer("agent-2", q.GetQuestionId(), answer)
	if err == nil {
		t.Error("expected error when answering with wrong agent ID")
	}
}

func findAskUserHandler(pack *packs.BuiltinPack) packs.ToolHandler {
	for _, tool := range pack.Tools {
		if tool.Definition.GetName() == "ask_user" {
			return tool.Handler
		}
	}
	return nil
}
