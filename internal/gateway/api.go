// ABOUTME: HTTP API handlers for exposing agent messaging via SSE.
// ABOUTME: Provides POST /api/send endpoint for external clients like TUI.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/2389/fold-gateway/internal/agent"
)

// SendMessageRequest is the JSON request body for POST /api/send.
type SendMessageRequest struct {
	ThreadID string `json:"thread_id,omitempty"`
	Sender   string `json:"sender"`
	Content  string `json:"content"`
}

// SSEEvent represents a Server-Sent Event.
type SSEEvent struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

// handleSendMessage handles POST /api/send requests.
// It accepts a JSON body with the message content and streams responses via SSE.
func (g *Gateway) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		g.sendJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Content == "" {
		g.sendJSONError(w, http.StatusBadRequest, "content is required")
		return
	}

	// Use mock sender if set (for testing), otherwise use agent manager
	sender := g.getSender()

	// Create the send request
	sendReq := &agent.SendRequest{
		ThreadID: req.ThreadID,
		Sender:   req.Sender,
		Content:  req.Content,
	}

	// Send message to an agent
	respChan, err := sender.SendMessage(r.Context(), sendReq)
	if err != nil {
		if err == agent.ErrNoAgentsAvailable {
			g.sendJSONError(w, http.StatusServiceUnavailable, "no agents available")
			return
		}
		g.sendJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		g.logger.Error("streaming not supported")
		return
	}

	// Stream responses
	g.streamResponses(r.Context(), w, flusher, respChan)
}

// streamResponses reads from the response channel and writes SSE events.
func (g *Gateway) streamResponses(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, respChan <-chan *agent.Response) {
	for {
		select {
		case <-ctx.Done():
			g.writeSSEEvent(w, "error", map[string]string{"error": "request cancelled"})
			flusher.Flush()
			return

		case resp, ok := <-respChan:
			if !ok {
				return
			}

			event := g.responseToSSEEvent(resp)
			g.writeSSEEvent(w, event.Event, event.Data)
			flusher.Flush()

			if resp.Done {
				return
			}
		}
	}
}

// responseToSSEEvent converts an agent response to an SSE event.
func (g *Gateway) responseToSSEEvent(resp *agent.Response) SSEEvent {
	switch resp.Event {
	case agent.EventThinking:
		return SSEEvent{
			Event: "thinking",
			Data:  map[string]string{"text": resp.Text},
		}
	case agent.EventText:
		return SSEEvent{
			Event: "text",
			Data:  map[string]string{"text": resp.Text},
		}
	case agent.EventToolUse:
		return SSEEvent{
			Event: "tool_use",
			Data: map[string]string{
				"id":         resp.ToolUse.ID,
				"name":       resp.ToolUse.Name,
				"input_json": resp.ToolUse.InputJSON,
			},
		}
	case agent.EventToolResult:
		return SSEEvent{
			Event: "tool_result",
			Data: map[string]interface{}{
				"id":       resp.ToolResult.ID,
				"output":   resp.ToolResult.Output,
				"is_error": resp.ToolResult.IsError,
			},
		}
	case agent.EventFile:
		return SSEEvent{
			Event: "file",
			Data: map[string]string{
				"filename":  resp.File.Filename,
				"mime_type": resp.File.MimeType,
			},
		}
	case agent.EventDone:
		return SSEEvent{
			Event: "done",
			Data:  map[string]string{"full_response": resp.Text},
		}
	case agent.EventError:
		return SSEEvent{
			Event: "error",
			Data:  map[string]string{"error": resp.Error},
		}
	default:
		return SSEEvent{
			Event: "unknown",
			Data:  map[string]string{"text": resp.Text},
		}
	}
}

// writeSSEEvent writes a single SSE event to the response writer.
func (g *Gateway) writeSSEEvent(w http.ResponseWriter, event string, data interface{}) {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		g.logger.Error("failed to marshal SSE data", "error", err)
		return
	}

	fmt.Fprintf(w, "event: %s\n", event)
	fmt.Fprintf(w, "data: %s\n\n", dataJSON)
}

// sendJSONError writes a JSON error response.
func (g *Gateway) sendJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// messageSender is an interface for sending messages to agents.
// This allows injecting mock implementations for testing.
type messageSender interface {
	SendMessage(ctx context.Context, req *agent.SendRequest) (<-chan *agent.Response, error)
	ListAgents() []*agent.AgentInfo
}

// getSender returns the message sender (mock or real agent manager).
func (g *Gateway) getSender() messageSender {
	if g.mockSender != nil {
		return g.mockSender
	}
	return g.agentManager
}
