// ABOUTME: Simulates a Claude Code interactive session in a tmux pane.
// ABOUTME: The adapter discovers panes running a binary named "claude-sim" (or "claude").
// ABOUTME: Accepts text input, fakes a thinking+response cycle with tool use, and returns to a prompt.
package main

import (
	"bufio"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"
)

// Prompt matches real Claude Code's magenta arrow.
const prompt = "\033[1;35m❯\033[0m "

// Horizontal rule that real Claude emits between response and next prompt.
const horizontalRule = "\033[2m─────────────────────────────────────────────────────────────────\033[0m"

var responses = []string{
	"I'll look into that. Let me check the relevant files.\n\nAfter reviewing the code, I can see that the function handles edge cases correctly. The error wrapping follows the project's `fmt.Errorf(\"context: %%w\", err)` pattern.",
	"Let me analyze this step by step.\n\nFirst, I'll read the configuration. The gateway loads config from `COVEN_CONFIG` or the default path. The gRPC server binds to `:50051` and the HTTP server to `:8080`.\n\nThe architecture looks solid — bidirectional streaming with request_id correlation.",
	"Good question. The agent lifecycle is:\n1. TCP connect to gateway\n2. Send RegisterAgent (first message)\n3. Receive Welcome with instance_id\n4. Enter message loop\n5. Handle SendMessage → stream MessageResponse events\n6. Heartbeat every 30s\n\nEach response MUST terminate with done, error, or cancelled.",
	"I'll fix that bug. The issue is a nil pointer dereference when the store returns an empty slice.\n\n```go\nif agents == nil {\n    agents = []Agent{}\n}\n```\n\nThis ensures JSON marshaling produces `[]` instead of `null`.",
}

// toolUseResponse simulates Claude reading a file (tool approval flow).
var toolUseResponse = struct {
	tool   string
	input  string
	output string
}{
	tool:   "Read",
	input:  "internal/gateway/gateway.go",
	output: "// Gateway orchestrates the gRPC server, HTTP server, agent manager...",
}

func main() {
	workDir, _ := os.Getwd()
	fmt.Printf("\033[1;36mClaude Code\033[0m (sim) v0.1.0\n")
	fmt.Printf("Working directory: %s\n", workDir)
	fmt.Printf("Model: claude-opus-4-6 (simulated)\n")
	fmt.Printf("Type a message or /quit to exit.\n\n")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print(prompt)
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "/quit" || input == "/exit" {
			fmt.Println("\nGoodbye!")
			return
		}

		// Simulate thinking
		thinkingDuration := time.Duration(500+rand.Intn(1000)) * time.Millisecond
		showThinking(thinkingDuration)

		// Every 3rd message (by length), simulate a tool use before the response.
		idx := len(input) % len(responses)
		if idx%3 == 1 {
			simulateToolUse()
		}

		// Pick a response (deterministic from input length for reproducibility).
		resp := responses[idx]

		// Stream it character by character (simulates Claude's streaming).
		streamText(resp)

		// Horizontal rule + blank line (matches real Claude footer).
		fmt.Printf("\n%s\n\n", horizontalRule)
	}
}

func showThinking(d time.Duration) {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	end := time.Now().Add(d)
	i := 0
	for time.Now().Before(end) {
		fmt.Printf("\r\033[2m%s Thinking...\033[0m", frames[i%len(frames)])
		time.Sleep(80 * time.Millisecond)
		i++
	}
	fmt.Print("\r\033[K") // clear the thinking line
}

func streamText(text string) {
	for _, ch := range text {
		fmt.Print(string(ch))
		time.Sleep(time.Duration(5+rand.Intn(15)) * time.Millisecond)
	}
}

func simulateToolUse() {
	// Show tool use block like real Claude.
	fmt.Printf("\033[1m📂 %s\033[0m %s\n", toolUseResponse.tool, toolUseResponse.input)
	time.Sleep(200 * time.Millisecond)
	fmt.Printf("\033[2m%s\033[0m\n\n", toolUseResponse.output)
	time.Sleep(300 * time.Millisecond)
}
