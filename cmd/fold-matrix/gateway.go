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
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
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

// BindingInfo represents a binding's current state.
type BindingInfo struct {
	BindingID  string `json:"binding_id"`
	AgentName  string `json:"agent_name"`
	WorkingDir string `json:"working_dir"`
	Online     bool   `json:"online"`
}

// CreateBindingRequest is the request body for POST /api/bindings.
type CreateBindingRequest struct {
	Frontend   string `json:"frontend"`
	ChannelID  string `json:"channel_id"`
	InstanceID string `json:"instance_id"`
}

// CreateBindingResponse is the response from POST /api/bindings.
type CreateBindingResponse struct {
	BindingID   string  `json:"binding_id"`
	AgentName   string  `json:"agent_name"`
	WorkingDir  string  `json:"working_dir"`
	ReboundFrom *string `json:"rebound_from"`
}

// AgentInfo represents an online agent.
type AgentInfo struct {
	ID         string   `json:"id"`
	InstanceID string   `json:"instance_id"`
	Name       string   `json:"name"`
	WorkingDir string   `json:"working_dir"`
	Workspaces []string `json:"workspaces"`
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
		client: &http.Client{
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout: 10 * time.Second,
			},
			// No overall timeout - SSE streams can be long-lived
		},
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
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
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
	// Increase buffer size to handle large AI responses (up to 10MB)
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)

	var eventType EventType
	var dataLines []string
	var fullResponse string

	// processEvent handles a complete SSE event
	processEvent := func() {
		if eventType == "" || len(dataLines) == 0 {
			return
		}
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

		if onEvent != nil {
			onEvent(event)
		}
	}

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return fullResponse, ctx.Err()
		default:
		}

		line := scanner.Text()

		// Empty line signals end of event
		if line == "" {
			// Check for error event before processing
			if eventType == EventError && len(dataLines) > 0 {
				var data ErrorEventData
				if json.Unmarshal([]byte(strings.Join(dataLines, "\n")), &data) == nil {
					return "", fmt.Errorf("agent error: %s", data.Error)
				}
			}
			processEvent()
			eventType = ""
			dataLines = nil
			continue
		}

		// Parse event type
		if strings.HasPrefix(line, "event:") {
			eventType = EventType(strings.TrimSpace(strings.TrimPrefix(line, "event:")))
			continue
		}

		// Parse data (SSE spec says single space after colon is optional but common)
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(line, "data:")
			if strings.HasPrefix(data, " ") {
				data = data[1:] // Remove optional leading space per SSE spec
			}
			dataLines = append(dataLines, data)
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		return fullResponse, fmt.Errorf("reading SSE stream: %w", err)
	}

	// Process any remaining buffered event (if stream closed without trailing newline)
	processEvent()

	return fullResponse, nil
}

// CreateBinding binds a channel to an agent by instance ID.
func (g *GatewayClient) CreateBinding(ctx context.Context, frontend, channelID, instanceID string) (*CreateBindingResponse, error) {
	reqBody := CreateBindingRequest{
		Frontend:   frontend,
		ChannelID:  channelID,
		InstanceID: instanceID,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		g.baseURL+"/api/bindings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, g.handleErrorResponse(resp)
	}

	var result CreateBindingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &result, nil
}

// DeleteBinding removes the binding for a channel.
func (g *GatewayClient) DeleteBinding(ctx context.Context, frontend, channelID string) error {
	u := fmt.Sprintf("%s/api/bindings?frontend=%s&channel_id=%s",
		g.baseURL, url.QueryEscape(frontend), url.QueryEscape(channelID))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, u, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		return g.handleErrorResponse(resp)
	}

	return nil
}

// GetBinding returns the current binding for a channel.
func (g *GatewayClient) GetBinding(ctx context.Context, frontend, channelID string) (*BindingInfo, error) {
	u := fmt.Sprintf("%s/api/bindings?frontend=%s&channel_id=%s",
		g.baseURL, url.QueryEscape(frontend), url.QueryEscape(channelID))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // Not bound
	}

	if resp.StatusCode != http.StatusOK {
		return nil, g.handleErrorResponse(resp)
	}

	var result BindingInfo
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &result, nil
}

// ListAgents returns all online agents.
func (g *GatewayClient) ListAgents(ctx context.Context) ([]AgentInfo, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		g.baseURL+"/api/agents", nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, g.handleErrorResponse(resp)
	}

	var result []AgentInfo
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return result, nil
}
