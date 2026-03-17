package tmux

import (
	"log/slog"
	"strings"
	"testing"
)

func newTestTracker(inputText string) (*ResponseTracker, *[]string) {
	var events []string
	tracker := NewResponseTracker(slog.Default(), inputText, func(state ResponseState, text string) {
		switch state {
		case StateThinking:
			events = append(events, "thinking")
		case StateResponding:
			events = append(events, "text:"+text)
		case StateDone:
			events = append(events, "done:"+text)
		}
	})
	return tracker, &events
}

func TestResponseTracker_BasicFlow(t *testing.T) {
	tracker, events := newTestTracker("hello")

	// Simulate claude-sim output (already ANSI-stripped).
	tracker.Feed("hello\n")
	tracker.Feed("⠋ Thinking...⠙ Thinking...\n")
	tracker.Feed("This is the response.\n")
	tracker.Feed("─────────────────────────────────────────────────────────────────\n")
	tracker.Feed("\n")
	tracker.Feed("❯\n")

	if len(*events) < 3 {
		t.Fatalf("expected at least 3 events, got %d: %v", len(*events), *events)
	}
	if (*events)[0] != "thinking" {
		t.Errorf("event[0] = %q, want thinking", (*events)[0])
	}

	// Find the done event.
	var doneText string
	for _, e := range *events {
		if text, ok := strings.CutPrefix(e, "done:"); ok {
			doneText = text
			break
		}
	}
	if doneText == "" {
		t.Fatalf("no done event found in %v", *events)
	}
	if !strings.Contains(doneText, "This is the response.") {
		t.Errorf("done text = %q, want to contain 'This is the response.'", doneText)
	}
}

func TestResponseTracker_InputEchoFiltered(t *testing.T) {
	tracker, events := newTestTracker("explain gRPC")

	tracker.Feed("explain gRPC\n")         // bare input echo
	tracker.Feed("❯ explain gRPC\n")       // prompt + input echo
	tracker.Feed("⠋ Thinking...\n")        // thinking
	tracker.Feed("gRPC is a framework.\n") // response
	tracker.Feed("─────────────────────────────────────────────────────────────────\n")
	tracker.Feed("❯\n")

	// Neither input echo should appear in the response.
	var doneText string
	for _, e := range *events {
		if text, ok := strings.CutPrefix(e, "done:"); ok {
			doneText = text
			break
		}
	}
	if doneText == "" {
		t.Fatal("no done event")
	}
	if strings.Contains(doneText, "explain gRPC") {
		t.Errorf("input echo leaked into response: %q", doneText)
	}
}

func TestResponseTracker_SpinnerMergedWithThinking(t *testing.T) {
	// pipe-pane merges spinner frames with \r into one line.
	tracker, events := newTestTracker("test")

	tracker.Feed("test\n")
	tracker.Feed("⠋ Thinking...⠙ Thinking...⠹ Thinking...\n")
	tracker.Feed("The answer is 42.\n")
	tracker.Feed("─────────────────────────────────────────────────────────────────\n")
	tracker.Feed("❯\n")

	if (*events)[0] != "thinking" {
		t.Errorf("expected thinking event first, got %v", *events)
	}

	var doneText string
	for _, e := range *events {
		if text, ok := strings.CutPrefix(e, "done:"); ok {
			doneText = text
			break
		}
	}
	if !strings.Contains(doneText, "The answer is 42.") {
		t.Errorf("response missing in done text: %q", doneText)
	}
}

func TestResponseTracker_EmptyLineBetweenRuleAndPrompt(t *testing.T) {
	// The actual terminal output has an empty line between rule and prompt.
	tracker, events := newTestTracker("hi")

	tracker.Feed("hi\n")
	tracker.Feed("Thinking...\n")
	tracker.Feed("Hello there!\n")
	tracker.Feed("─────────────────────────────────────────────────────────────────\n")
	tracker.Feed("\n")  // empty line
	tracker.Feed("❯\n") // prompt

	var done bool
	for _, e := range *events {
		if strings.HasPrefix(e, "done:") {
			done = true
		}
	}
	if !done {
		t.Fatalf("empty line between rule and prompt prevented done detection; events: %v", *events)
	}
}

func TestResponseTracker_PendingLineBuffer(t *testing.T) {
	// Chunks arrive without trailing \n — should be joined.
	tracker, events := newTestTracker("q")

	tracker.Feed("q\n")
	tracker.Feed("Thinking...\n")
	// Response arrives in small chunks without newlines.
	tracker.Feed("Hello ") // no \n → pending
	tracker.Feed("world!") // no \n → pending grows
	tracker.Feed("\n")     // now "Hello world!" is a complete line
	tracker.Feed("─────────────────────────────────────────────────────────────────\n")
	tracker.Feed("❯\n")

	var doneText string
	for _, e := range *events {
		if text, ok := strings.CutPrefix(e, "done:"); ok {
			doneText = text
			break
		}
	}
	if !strings.Contains(doneText, "Hello world!") {
		t.Errorf("pending buffer didn't join chunks: %q", doneText)
	}
}

func TestResponseTracker_FlushPendingPrompt(t *testing.T) {
	// The prompt ❯ often arrives without trailing \n.
	tracker, events := newTestTracker("test")

	tracker.Feed("test\n")
	tracker.Feed("Thinking...\n")
	tracker.Feed("Response text.\n")
	tracker.Feed("─────────────────────────────────────────────────────────────────\n")
	tracker.Feed("\n")
	tracker.Feed("❯ ") // no trailing \n
	tracker.FlushPending()

	var done bool
	for _, e := range *events {
		if strings.HasPrefix(e, "done:") {
			done = true
		}
	}
	if !done {
		t.Fatalf("FlushPending didn't detect prompt; events: %v", *events)
	}
}

func TestResponseTracker_ToolUseInResponse(t *testing.T) {
	// Tool use appears as bold text followed by dim output.
	tracker, events := newTestTracker("hello")

	tracker.Feed("hello\n")
	tracker.Feed("⠋ Thinking...⠙ Thinking...📂 Read gateway.go\n")
	tracker.Feed("// file contents here\n")
	tracker.Feed("\n")
	tracker.Feed("The code looks good.\n")
	tracker.Feed("─────────────────────────────────────────────────────────────────\n")
	tracker.Feed("❯\n")

	var doneText string
	for _, e := range *events {
		if text, ok := strings.CutPrefix(e, "done:"); ok {
			doneText = text
			break
		}
	}
	if doneText == "" {
		t.Fatalf("no done event; events: %v", *events)
	}
	if !strings.Contains(doneText, "The code looks good.") {
		t.Errorf("response text missing: %q", doneText)
	}
}

func TestResponseTracker_MultipleResponses(t *testing.T) {
	// Two full request/response cycles on the same tracker.
	tracker, events := newTestTracker("q1")

	// First response.
	tracker.Feed("q1\n")
	tracker.Feed("Thinking...\n")
	tracker.Feed("Answer 1.\n")
	tracker.Feed("─────────────────────────────────────────────────────────────────\n")
	tracker.Feed("❯\n")

	count := 0
	for _, e := range *events {
		if strings.HasPrefix(e, "done:") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected 1 done after first response, got %d", count)
	}

	// Reset for second response (as WaitForResponse does).
	tracker.mu.Lock()
	tracker.state = StateIdle
	tracker.textBuf.Reset()
	tracker.pending = ""
	tracker.sawRule = false
	tracker.inputText = "q2"
	tracker.mu.Unlock()

	tracker.Feed("q2\n")
	tracker.Feed("Thinking...\n")
	tracker.Feed("Answer 2.\n")
	tracker.Feed("─────────────────────────────────────────────────────────────────\n")
	tracker.Feed("❯\n")

	count = 0
	for _, e := range *events {
		if strings.HasPrefix(e, "done:") {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("expected 2 done events after both responses, got %d", count)
	}
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain text", "hello", "hello"},
		{"bold", "\033[1mhello\033[0m", "hello"},
		{"dim", "\033[2mThinking...\033[0m", "Thinking..."},
		{"color", "\033[1;35m❯\033[0m ", "❯ "},
		{"carriage return", "line\roverwrite", "lineoverwrite"},
		{"erase line", "\033[K", ""},
		{"osc sequence", "\033]0;title\007", ""},
		{"mixed", "\033[2m⠋ Thinking...\033[0m\r\033[2m⠙ Thinking...\033[0m", "⠋ Thinking...⠙ Thinking..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripANSI(tt.input)
			if got != tt.want {
				t.Errorf("StripANSI(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsSpinnerFrame(t *testing.T) {
	if !isSpinnerFrame("⠋ Thinking...") {
		t.Error("expected spinner frame for ⠋")
	}
	if !isSpinnerFrame("⠙ Processing") {
		t.Error("expected spinner frame for ⠙")
	}
	if isSpinnerFrame("Hello world") {
		t.Error("plain text should not be spinner")
	}
	if isSpinnerFrame("") {
		t.Error("empty string should not be spinner")
	}
}
