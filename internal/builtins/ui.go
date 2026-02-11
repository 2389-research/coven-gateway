// ABOUTME: UI pack provides tools for agent-user interaction: ask_user.
// ABOUTME: Requires the "ui" capability. Tools communicate with connected clients.

package builtins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/2389/coven-gateway/internal/packs"
	pb "github.com/2389/coven-gateway/proto/coven"
)

// QuestionRouter handles routing user question responses to waiting handlers.
// It's the bridge between the AnswerQuestion RPC and the ask_user tool handlers.
type QuestionRouter interface {
	// SendQuestion sends a question to the client and returns a channel for the answer.
	// The channel will receive the answer or be closed on timeout.
	SendQuestion(ctx context.Context, agentID string, req *pb.UserQuestionRequest) (<-chan *pb.AnswerQuestionRequest, error)

	// DeliverAnswer routes an answer to the waiting handler.
	DeliverAnswer(agentID, questionID string, answer *pb.AnswerQuestionRequest) error
}

// UIPack creates the UI pack with user interaction tools.
func UIPack(router QuestionRouter) *packs.BuiltinPack {
	u := &uiHandlers{router: router}
	return &packs.BuiltinPack{
		ID: "builtin:ui",
		Tools: []*packs.BuiltinTool{
			{
				Definition: &pb.ToolDefinition{
					Name:        "ask_user",
					Description: "Ask the user a question and wait for their response. Use this when you need user input, clarification, or to present choices. Returns the user's selected option(s) or indicates timeout if they don't respond.",
					InputSchemaJson: `{
						"type": "object",
						"properties": {
							"question": {
								"type": "string",
								"description": "The question to ask the user"
							},
							"options": {
								"type": "array",
								"items": {
									"type": "object",
									"properties": {
										"label": {"type": "string", "description": "Display text for this option"},
										"description": {"type": "string", "description": "Optional explanation of this option"}
									},
									"required": ["label"]
								},
								"description": "Multiple choice options (2-4 recommended)"
							},
							"multi_select": {
								"type": "boolean",
								"description": "Allow selecting multiple options (default: false)"
							},
							"header": {
								"type": "string",
								"description": "Short label/category for the question (optional)"
							},
							"timeout_seconds": {
								"type": "integer",
								"description": "How long to wait for response (default: 60, max: 300)"
							}
						},
						"required": ["question", "options"]
					}`,
					RequiredCapabilities: []string{"ui"},
					TimeoutSeconds:       300, // Max timeout for the tool itself
				},
				Handler: u.AskUser,
			},
		},
	}
}

type uiHandlers struct {
	router QuestionRouter
}

// AskUserInput is the input schema for the ask_user tool.
type AskUserInput struct {
	Question       string          `json:"question"`
	Options        []AskUserOption `json:"options"`
	MultiSelect    bool            `json:"multi_select,omitempty"`
	Header         string          `json:"header,omitempty"`
	TimeoutSeconds int             `json:"timeout_seconds,omitempty"`
}

type AskUserOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// AskUserOutput is the output schema for the ask_user tool.
type AskUserOutput struct {
	Answered   bool     `json:"answered"`
	Selected   []string `json:"selected,omitempty"`
	CustomText string   `json:"custom_text,omitempty"`
	Reason     string   `json:"reason,omitempty"`
}

// validateAskUserInput validates the input fields for the ask_user tool.
func validateAskUserInput(in *AskUserInput) error {
	if in.Question == "" {
		return errors.New("question is required")
	}
	if len(in.Options) < 1 {
		return errors.New("at least one option is required")
	}
	seenLabels := make(map[string]bool)
	for _, opt := range in.Options {
		if seenLabels[opt.Label] {
			return fmt.Errorf("duplicate option label: %q", opt.Label)
		}
		seenLabels[opt.Label] = true
	}
	return nil
}

// normalizeTimeout returns a timeout in seconds, clamped to [1, 300].
func normalizeTimeout(requested int) int32 {
	switch {
	case requested <= 0:
		return 60
	case requested > 300:
		return 300
	default:
		return int32(requested)
	}
}

// buildQuestionRequest creates a UserQuestionRequest from validated input.
func buildQuestionRequest(agentID string, in *AskUserInput, timeout int32) *pb.UserQuestionRequest {
	req := &pb.UserQuestionRequest{
		AgentId:        agentID,
		QuestionId:     uuid.New().String(),
		Question:       in.Question,
		Options:        make([]*pb.QuestionOption, len(in.Options)),
		MultiSelect:    in.MultiSelect,
		TimeoutSeconds: timeout,
	}
	if in.Header != "" {
		req.Header = &in.Header
	}
	for i, opt := range in.Options {
		req.Options[i] = &pb.QuestionOption{Label: opt.Label}
		if opt.Description != "" {
			req.Options[i].Description = &opt.Description
		}
	}
	return req
}

func (u *uiHandlers) AskUser(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
	var in AskUserInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}
	if err := validateAskUserInput(&in); err != nil {
		return nil, err
	}

	timeout := normalizeTimeout(in.TimeoutSeconds)
	req := buildQuestionRequest(agentID, &in, timeout)

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	answerChan, err := u.router.SendQuestion(timeoutCtx, agentID, req)
	if err != nil {
		return nil, fmt.Errorf("failed to send question: %w", err)
	}

	return u.awaitAnswer(answerChan, timeoutCtx)
}

// awaitAnswer waits for an answer from the channel or returns timeout.
func (u *uiHandlers) awaitAnswer(answerChan <-chan *pb.AnswerQuestionRequest, ctx context.Context) (json.RawMessage, error) {
	select {
	case answer, ok := <-answerChan:
		if !ok || answer == nil {
			return json.Marshal(AskUserOutput{Answered: false, Reason: "no_response"})
		}
		out := AskUserOutput{Answered: true, Selected: answer.Selected}
		if answer.CustomText != nil {
			out.CustomText = *answer.CustomText
		}
		return json.Marshal(out)
	case <-ctx.Done():
		return json.Marshal(AskUserOutput{Answered: false, Reason: "timeout"})
	}
}

// pendingQuestion tracks a question awaiting an answer.
type pendingQuestion struct {
	agentID    string
	answerChan chan *pb.AnswerQuestionRequest
	done       chan struct{} // signals when answer delivered or context canceled
	closeOnce  sync.Once     // ensures answerChan is closed exactly once
}

// InMemoryQuestionRouter is a simple in-memory implementation of QuestionRouter.
// It tracks pending questions and routes answers to waiting handlers.
type InMemoryQuestionRouter struct {
	mu       sync.RWMutex
	pending  map[string]*pendingQuestion // questionID -> pending question
	streamer ClientStreamer
}

// ClientStreamer is the interface for sending events to clients.
type ClientStreamer interface {
	// SendToAgent sends an event to clients subscribed to an agent's conversation
	SendUserQuestion(agentID string, req *pb.UserQuestionRequest) error
}

// NewInMemoryQuestionRouter creates a new question router.
func NewInMemoryQuestionRouter(streamer ClientStreamer) *InMemoryQuestionRouter {
	return &InMemoryQuestionRouter{
		pending:  make(map[string]*pendingQuestion),
		streamer: streamer,
	}
}

func (r *InMemoryQuestionRouter) SendQuestion(ctx context.Context, agentID string, req *pb.UserQuestionRequest) (<-chan *pb.AnswerQuestionRequest, error) {
	// Create answer channel and done signal
	answerChan := make(chan *pb.AnswerQuestionRequest, 1)
	done := make(chan struct{})

	pq := &pendingQuestion{
		agentID:    agentID,
		answerChan: answerChan,
		done:       done,
	}

	// Register pending question
	r.mu.Lock()
	r.pending[req.QuestionId] = pq
	r.mu.Unlock()

	// Send question to client
	if err := r.streamer.SendUserQuestion(agentID, req); err != nil {
		r.mu.Lock()
		delete(r.pending, req.QuestionId)
		r.mu.Unlock()
		close(answerChan)
		close(done)
		return nil, err
	}

	// Clean up on context done (goroutine exits early if answer is delivered first)
	go func() {
		select {
		case <-ctx.Done():
			r.mu.Lock()
			if pq, ok := r.pending[req.QuestionId]; ok {
				delete(r.pending, req.QuestionId)
				pq.closeOnce.Do(func() { close(pq.answerChan) })
			}
			r.mu.Unlock()
		case <-done:
			// Answer was already delivered, nothing to clean up
		}
	}()

	return answerChan, nil
}

func (r *InMemoryQuestionRouter) DeliverAnswer(agentID, questionID string, answer *pb.AnswerQuestionRequest) error {
	r.mu.Lock()
	pq, ok := r.pending[questionID]
	if ok {
		delete(r.pending, questionID)
	}
	r.mu.Unlock()

	if !ok {
		return fmt.Errorf("no pending question with ID %s", questionID)
	}

	// Validate that the answer is for the correct agent
	if pq.agentID != agentID {
		return fmt.Errorf("answer agent_id %q does not match question agent_id %q", agentID, pq.agentID)
	}

	// Signal cleanup goroutine to exit
	close(pq.done)

	// Non-blocking send (channel has buffer of 1)
	select {
	case pq.answerChan <- answer:
	default:
		// Channel full - should not happen with buffer of 1
	}

	// Use sync.Once to ensure answerChan is closed exactly once, preventing
	// double-close panic if context cancellation races with answer delivery.
	pq.closeOnce.Do(func() { close(pq.answerChan) })

	return nil
}
