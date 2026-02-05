// ABOUTME: UI pack provides tools for agent-user interaction: ask_user.
// ABOUTME: Requires the "ui" capability. Tools communicate with connected clients.

package builtins

import (
	"context"
	"encoding/json"
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

// AskUserInput is the input schema for the ask_user tool
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

// AskUserOutput is the output schema for the ask_user tool
type AskUserOutput struct {
	Answered   bool     `json:"answered"`
	Selected   []string `json:"selected,omitempty"`
	CustomText string   `json:"custom_text,omitempty"`
	Reason     string   `json:"reason,omitempty"`
}

func (u *uiHandlers) AskUser(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
	var in AskUserInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	// Validate input
	if in.Question == "" {
		return nil, fmt.Errorf("question is required")
	}
	if len(in.Options) < 1 {
		return nil, fmt.Errorf("at least one option is required")
	}

	// Validate no duplicate option labels
	seenLabels := make(map[string]bool)
	for _, opt := range in.Options {
		if seenLabels[opt.Label] {
			return nil, fmt.Errorf("duplicate option label: %q", opt.Label)
		}
		seenLabels[opt.Label] = true
	}

	// Set default timeout
	timeout := in.TimeoutSeconds
	if timeout <= 0 {
		timeout = 60
	}
	if timeout > 300 {
		timeout = 300
	}

	// Build the proto request
	questionID := uuid.New().String()
	req := &pb.UserQuestionRequest{
		AgentId:        agentID,
		QuestionId:     questionID,
		Question:       in.Question,
		Options:        make([]*pb.QuestionOption, len(in.Options)),
		MultiSelect:    in.MultiSelect,
		TimeoutSeconds: int32(timeout),
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

	// Create timeout context BEFORE sending the question so cleanup goroutine
	// in SendQuestion will be notified when timeout fires
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	// Send question and wait for answer
	answerChan, err := u.router.SendQuestion(timeoutCtx, agentID, req)
	if err != nil {
		return nil, fmt.Errorf("failed to send question: %w", err)
	}

	select {
	case answer, ok := <-answerChan:
		if !ok || answer == nil {
			// Channel closed without answer (client disconnected, etc.)
			return json.Marshal(AskUserOutput{
				Answered: false,
				Reason:   "no_response",
			})
		}
		// Got an answer
		out := AskUserOutput{
			Answered: true,
			Selected: answer.Selected,
		}
		if answer.CustomText != nil {
			out.CustomText = *answer.CustomText
		}
		return json.Marshal(out)

	case <-timeoutCtx.Done():
		return json.Marshal(AskUserOutput{
			Answered: false,
			Reason:   "timeout",
		})
	}
}

// pendingQuestion tracks a question awaiting an answer
type pendingQuestion struct {
	agentID    string
	answerChan chan *pb.AnswerQuestionRequest
	done       chan struct{} // signals when answer delivered or context cancelled
}

// InMemoryQuestionRouter is a simple in-memory implementation of QuestionRouter.
// It tracks pending questions and routes answers to waiting handlers.
type InMemoryQuestionRouter struct {
	mu       sync.RWMutex
	pending  map[string]*pendingQuestion // questionID -> pending question
	streamer ClientStreamer
}

// ClientStreamer is the interface for sending events to clients
type ClientStreamer interface {
	// SendToAgent sends an event to clients subscribed to an agent's conversation
	SendUserQuestion(agentID string, req *pb.UserQuestionRequest) error
}

// NewInMemoryQuestionRouter creates a new question router
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
				close(pq.answerChan)
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

	// Non-blocking send (channel has buffer of 1)
	select {
	case pq.answerChan <- answer:
	default:
		// Channel full - should not happen with buffer of 1
	}
	close(pq.answerChan)

	// Signal that answer was delivered so cleanup goroutine can exit
	close(pq.done)

	return nil
}
