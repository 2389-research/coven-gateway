// ABOUTME: TUI client for interacting with coven-gateway agents via HTTP API.
// ABOUTME: Provides readline-style input and SSE streaming output with JWT auth.

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

// getToken returns the JWT token from COVEN_TOKEN env var or ~/.config/coven/token file
func getToken() string {
	// Check env var first
	if token := os.Getenv("COVEN_TOKEN"); token != "" {
		return token
	}

	// Try to read from token file
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		configDir = filepath.Join(homeDir, ".config")
	}

	tokenPath := filepath.Join(configDir, "coven", "token")
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(data))
}

// sendRequest is the JSON body sent to POST /api/send.
type sendRequest struct {
	ThreadID string `json:"thread_id,omitempty"`
	Sender   string `json:"sender"`
	Content  string `json:"content"`
	AgentID  string `json:"agent_id,omitempty"`
}

// agentInfo is the JSON response from GET /api/agents.
type agentInfo struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Capabilities []string `json:"capabilities"`
}

// sseEvent is a parsed Server-Sent Event.
type sseEvent struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

func main() {
	// Parse command line flags
	server := flag.String("server", "http://localhost:8080", "Gateway server URL")
	sender := flag.String("sender", "tui-user", "Sender name for messages")
	threadID := flag.String("thread", "", "Thread ID for conversation continuity")
	flag.Parse()

	fmt.Printf("coven-tui connected to %s\n", *server)
	if getToken() != "" {
		fmt.Println("Auth: JWT token configured (COVEN_TOKEN)")
	} else {
		fmt.Println("Auth: none (set COVEN_TOKEN for authentication)")
	}
	fmt.Println("Type a message and press Enter. /help for commands. Ctrl+C to quit.")
	fmt.Println()

	// Setup context with signal handling for graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Run the interactive loop
	if err := run(ctx, *server, *sender, *threadID); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\nGoodbye!")
}

func run(ctx context.Context, server, sender, threadID string) error {
	scanner := bufio.NewScanner(os.Stdin)
	var selectedAgentID string

	for {
		// Print prompt (include agent ID if one is selected)
		if selectedAgentID != "" {
			fmt.Printf("[%s]> ", selectedAgentID)
		} else {
			fmt.Print("> ")
		}

		// Read input with context awareness
		inputCh := make(chan string, 1)
		errCh := make(chan error, 1)

		go func() {
			if scanner.Scan() {
				inputCh <- scanner.Text()
			} else {
				if err := scanner.Err(); err != nil {
					errCh <- err
				} else {
					errCh <- io.EOF
				}
			}
		}()

		var input string
		select {
		case <-ctx.Done():
			return nil
		case err := <-errCh:
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("reading input: %w", err)
		case input = <-inputCh:
		}

		// Trim whitespace
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		// Check for quit commands
		if input == "/quit" || input == "/exit" || input == "/q" {
			return nil
		}

		// Check for /agents command
		if input == "/agents" {
			if err := listAgents(ctx, server); err != nil {
				fmt.Printf("[error] %v\n", err)
			}
			fmt.Println()
			continue
		}

		// Check for /use command
		if strings.HasPrefix(input, "/use") {
			args := strings.TrimSpace(strings.TrimPrefix(input, "/use"))
			if args == "" {
				// Clear agent selection
				selectedAgentID = ""
				fmt.Println("Cleared agent selection, using router")
			} else {
				// Set agent selection
				selectedAgentID = args
				fmt.Printf("Now using %s\n", selectedAgentID)
			}
			fmt.Println()
			continue
		}

		// Check for /history command
		if input == "/history" {
			if selectedAgentID == "" {
				fmt.Println("No agent selected. Use /use <agent_id> first.")
			} else {
				if err := fetchHistory(ctx, server, selectedAgentID); err != nil {
					fmt.Printf("[error] %v\n", err)
				}
			}
			fmt.Println()
			continue
		}

		// Check for /help command
		if input == "/help" {
			printHelp()
			fmt.Println()
			continue
		}

		// Send message and stream response
		if err := sendMessage(ctx, server, sender, threadID, selectedAgentID, input); err != nil {
			fmt.Printf("[error] %v\n", err)
		}
		fmt.Println()
	}
}

// printHelp displays available commands.
func printHelp() {
	fmt.Println("Commands:")
	fmt.Println("  /agents        List connected agents")
	fmt.Println("  /use <id>      Set default agent for messages")
	fmt.Println("  /use           Clear agent selection, use router")
	fmt.Println("  /history       Show conversation history (requires /use)")
	fmt.Println("  /help          Show this help")
	fmt.Println("  /quit          Exit the TUI")
}

// listAgents fetches and displays connected agents.
func listAgents(ctx context.Context, server string) error {
	url := fmt.Sprintf("%s/api/agents", server)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	// Add auth header if token is configured
	if token := getToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetching agents: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	var agents []agentInfo
	if err := json.NewDecoder(resp.Body).Decode(&agents); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if len(agents) == 0 {
		fmt.Println("No agents connected")
		return nil
	}

	fmt.Println("Connected agents:")
	for _, a := range agents {
		caps := strings.Join(a.Capabilities, ", ")
		fmt.Printf("  %s: %s [%s]\n", a.ID, a.Name, caps)
	}
	return nil
}

// historyEvent represents an event in the agent history response.
type historyEvent struct {
	ID        string `json:"id"`
	Direction string `json:"direction"`
	Author    string `json:"author"`
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Text      string `json:"text,omitempty"`
}

// historyResponse represents the response from GET /api/agents/{id}/history.
type historyResponse struct {
	AgentID string         `json:"agent_id"`
	Events  []historyEvent `json:"events"`
	Count   int            `json:"count"`
	HasMore bool           `json:"has_more"`
}

// fetchHistory fetches and displays conversation history for an agent.
func fetchHistory(ctx context.Context, server, agentID string) error {
	url := fmt.Sprintf("%s/api/agents/%s/history?limit=20", server, agentID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	if token := getToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetching history: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	var history historyResponse
	if err := json.NewDecoder(resp.Body).Decode(&history); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if len(history.Events) == 0 {
		fmt.Println("No conversation history")
		return nil
	}

	fmt.Printf("Recent history for %s (%d events):\n", agentID, history.Count)
	fmt.Println(strings.Repeat("-", 60))

	for _, evt := range history.Events {
		// Format based on direction
		prefix := "  "
		if evt.Direction == "inbound_to_agent" {
			prefix = "\033[34m→\033[0m " // Blue arrow for user messages
		} else if evt.Direction == "outbound_from_agent" {
			prefix = "\033[32m←\033[0m " // Green arrow for agent messages
		}

		// Show event based on type
		switch evt.Type {
		case "message":
			text := stripMarkdown(evt.Text)
			if len(text) > 200 {
				text = text[:197] + "..."
			}
			fmt.Printf("%s%s\n", prefix, text)
		case "tool_use":
			fmt.Printf("%s\033[33m[tool]\033[0m %s\n", prefix, truncate(evt.Text, 60))
		case "tool_result":
			fmt.Printf("%s\033[2m[result]\033[0m\n", prefix)
		default:
			if evt.Text != "" {
				fmt.Printf("%s[%s] %s\n", prefix, evt.Type, truncate(evt.Text, 60))
			}
		}
	}

	if history.HasMore {
		fmt.Printf("\033[2m... more history available\033[0m\n")
	}
	fmt.Println(strings.Repeat("-", 60))

	return nil
}

func sendMessage(ctx context.Context, server, sender, threadID, agentID, content string) error {
	// Build request body
	reqBody := sendRequest{
		ThreadID: threadID,
		Sender:   sender,
		Content:  content,
		AgentID:  agentID,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/api/send", server)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	// Add auth header if token is configured
	if token := getToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	// Send request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	// Check for error responses
	if resp.StatusCode != http.StatusOK {
		// Try to read error from body
		if resp.Header.Get("Content-Type") == "application/json" {
			var errResp map[string]string
			if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil {
				if msg, ok := errResp["error"]; ok {
					return fmt.Errorf("%s", msg)
				}
			}
		}
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	// Stream SSE responses
	return streamSSE(ctx, resp.Body)
}

func streamSSE(ctx context.Context, body io.Reader) error {
	scanner := bufio.NewScanner(body)

	var eventType string
	var dataLines []string

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()

		// Empty line signals end of event
		if line == "" {
			if eventType != "" && len(dataLines) > 0 {
				data := strings.Join(dataLines, "\n")
				if err := handleSSEEvent(eventType, data); err != nil {
					return err
				}
			}
			eventType = ""
			dataLines = nil
			continue
		}

		// Parse event type
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}

		// Parse data
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data:"))
			continue
		}
	}

	return scanner.Err()
}

func handleSSEEvent(eventType, data string) error {
	// Parse JSON data
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return fmt.Errorf("parsing event data: %w", err)
	}

	switch eventType {
	case "thinking":
		if text, ok := payload["text"].(string); ok {
			fmt.Printf("\033[2m[thinking] %s\033[0m\n", truncate(text, 80))
		}

	case "text":
		if text, ok := payload["text"].(string); ok {
			fmt.Print(stripMarkdown(text))
		}

	case "tool_use":
		// Field is "name" from SSE event
		name := payload["name"]
		fmt.Printf("\033[33m[tool] %v\033[0m\n", name)

	case "tool_result":
		isError := false
		if e, ok := payload["is_error"].(bool); ok {
			isError = e
		}
		if isError {
			output := truncate(fmt.Sprintf("%v", payload["output"]), 100)
			fmt.Printf("\033[31m[tool error] %s\033[0m\n", output)
		} else {
			fmt.Printf("\033[32m[tool done]\033[0m\n")
		}

	case "tool_state":
		state := payload["state"]
		detail := payload["detail"]
		if detail != nil && detail != "" {
			fmt.Printf("\033[2m[tool %v] %v\033[0m\n", state, detail)
		}

	case "tool_approval":
		name := payload["name"]
		fmt.Printf("\033[33m[approval needed] %v - respond in web UI\033[0m\n", name)

	case "usage":
		// Silently ignore usage events in TUI
		return nil

	case "file":
		filename := payload["filename"]
		fmt.Printf("[file] %v\n", filename)

	case "done":
		fmt.Println()

	case "cancelled":
		reason := payload["reason"]
		fmt.Printf("\033[33m[cancelled] %v\033[0m\n", reason)

	case "error":
		if errMsg, ok := payload["error"].(string); ok {
			fmt.Printf("\033[31m[error] %s\033[0m\n", errMsg)
		}

	default:
		// Ignore unknown events silently
	}

	return nil
}

// truncate shortens a string to maxLen, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// stripMarkdown removes common markdown formatting from text.
func stripMarkdown(s string) string {
	// Remove bold/italic markers (order matters: ** before *)
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "__", "")
	// Don't remove single * as it's often used for lists
	return s
}
