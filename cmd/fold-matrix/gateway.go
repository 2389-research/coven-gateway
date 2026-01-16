// ABOUTME: Gateway API client for fold-matrix bridge
// ABOUTME: Sends messages and streams SSE responses from fold-gateway

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// EventType represents SSE event types from the gateway.
type EventType string

const (
	EventThinking   EventType = "thinking"
	EventText       EventType = "text"
	EventToolUse    EventType = "tool_use"
	EventToolResult EventType = "tool_result"
	EventFile       EventType = "file"
	EventDone       EventType = "done"
	EventError      EventType = "error"
)

// SSEEvent represents a parsed Server-Sent Event.
type SSEEvent struct {
	Type EventType
	Data string
}

// TextEventData is the JSON structure for text/thinking/done events.
type TextEventData struct {
	Text         string `json:"text,omitempty"`
	FullResponse string `json:"full_response,omitempty"`
}

// ErrorEventData is the JSON structure for error events.
type ErrorEventData struct {
	Error string `json:"error"`
}

// SendRequest is the request body for POST /api/send.
type SendRequest struct {
	ThreadID  string `json:"thread_id,omitempty"`
	Sender    string `json:"sender"`
	Content   string `json:"content"`
	Frontend  string `json:"frontend"`
	ChannelID string `json:"channel_id"`
}

// GatewayClient communicates with fold-gateway HTTP API.
type GatewayClient struct {
	baseURL string
	client  *http.Client
}

// NewGatewayClient creates a new gateway client.
func NewGatewayClient(baseURL string) *GatewayClient {
	return &GatewayClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		client:  &http.Client{},
	}
}

// SendMessage sends a message to the gateway and streams SSE responses via callback.
// The callback is called for each SSE event received.
// Returns the full response text on success, or an error.
func (g *GatewayClient) SendMessage(ctx context.Context, req SendRequest, onEvent func(SSEEvent)) (string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		g.baseURL+"/api/send", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	// Handle non-200 responses
	if resp.StatusCode != http.StatusOK {
		return "", g.handleErrorResponse(resp)
	}

	// Parse SSE stream
	return g.parseSSEStream(ctx, resp.Body, onEvent)
}

// handleErrorResponse extracts error message from non-200 responses.
func (g *GatewayClient) handleErrorResponse(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	// Try to parse as JSON error
	if resp.Header.Get("Content-Type") == "application/json" {
		var errResp ErrorEventData
		if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
			return fmt.Errorf("gateway error (%d): %s", resp.StatusCode, errResp.Error)
		}
	}

	return fmt.Errorf("gateway returned status %d: %s", resp.StatusCode, string(body))
}

// parseSSEStream reads SSE events from the response body.
func (g *GatewayClient) parseSSEStream(ctx context.Context, body io.Reader, onEvent func(SSEEvent)) (string, error) {
	scanner := bufio.NewScanner(body)

	var eventType EventType
	var dataLines []string
	var fullResponse string

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return fullResponse, ctx.Err()
		default:
		}

		line := scanner.Text()

		// Empty line signals end of event
		if line == "" {
			if eventType != "" && len(dataLines) > 0 {
				event := SSEEvent{
					Type: eventType,
					Data: strings.Join(dataLines, "\n"),
				}

				// Extract full response from done event
				if eventType == EventDone {
					var data TextEventData
					if json.Unmarshal([]byte(event.Data), &data) == nil {
						fullResponse = data.FullResponse
					}
				}

				// Check for error event
				if eventType == EventError {
					var data ErrorEventData
					if json.Unmarshal([]byte(event.Data), &data) == nil {
						return "", fmt.Errorf("agent error: %s", data.Error)
					}
				}

				if onEvent != nil {
					onEvent(event)
				}
			}
			eventType = ""
			dataLines = nil
			continue
		}

		// Parse event type
		if strings.HasPrefix(line, "event:") {
			eventType = EventType(strings.TrimSpace(strings.TrimPrefix(line, "event:")))
			continue
		}

		// Parse data
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data:"))
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		return fullResponse, fmt.Errorf("reading SSE stream: %w", err)
	}

	return fullResponse, nil
}
