// ABOUTME: TUI client for interacting with fold-gateway agents via HTTP API.
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

// getToken returns the JWT token from FOLD_TOKEN env var or ~/.config/fold/token file
func getToken() string {
	// Check env var first
	if token := os.Getenv("FOLD_TOKEN"); token != "" {
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

	tokenPath := filepath.Join(configDir, "fold", "token")
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

	fmt.Printf("fold-tui connected to %s\n", *server)
	if getToken() != "" {
		fmt.Println("Auth: JWT token configured (FOLD_TOKEN)")
	} else {
		fmt.Println("Auth: none (set FOLD_TOKEN for authentication)")
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
			fmt.Printf("[thinking] %s\n", truncate(text, 80))
		}

	case "text":
		if text, ok := payload["text"].(string); ok {
			fmt.Print(text)
		}

	case "tool_use":
		name := payload["name"]
		fmt.Printf("[tool] using: %v\n", name)

	case "tool_result":
		isError := false
		if e, ok := payload["is_error"].(bool); ok {
			isError = e
		}
		if isError {
			fmt.Printf("[tool] error: %v\n", payload["output"])
		} else {
			fmt.Printf("[tool] result received\n")
		}

	case "file":
		filename := payload["filename"]
		fmt.Printf("[file] %v\n", filename)

	case "done":
		fmt.Println()
		fmt.Println("[done]")

	case "error":
		if errMsg, ok := payload["error"].(string); ok {
			fmt.Printf("[error] %s\n", errMsg)
		}

	default:
		fmt.Printf("[%s] %v\n", eventType, payload)
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
