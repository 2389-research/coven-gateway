package tmux

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// loadFixture reads a test fixture from testdata/ and returns the ANSI-stripped content.
func loadFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", name, err)
	}
	return StripANSI(string(data))
}

// feedFixture processes an entire fixture through a tracker, simulating pipe-pane
// output arriving in a single chunk (the most common case for captured output).
func feedFixture(t *testing.T, tracker *ResponseTracker, content string) {
	t.Helper()
	tracker.Feed(content)
	tracker.FlushPending()
}

// extractDoneText finds the last done event text from a list of events.
func extractDoneText(events []string) string {
	var last string
	for _, e := range events {
		if text, ok := strings.CutPrefix(e, "done:"); ok {
			last = text
		}
	}
	return last
}

// hasDoneEvent returns true if any done event was recorded.
func hasDoneEvent(events []string) bool {
	return slices.ContainsFunc(events, func(e string) bool {
		return strings.HasPrefix(e, "done:")
	})
}

// hasThinkingEvent returns true if any thinking event was recorded.
func hasThinkingEvent(events []string) bool {
	return slices.Contains(events, "thinking")
}

func TestFixture_StartupBanner(t *testing.T) {
	// The startup banner should NOT trigger any state transitions.
	// It appears before any user input, so the tracker stays idle.
	tracker, events := newTestTracker("")
	content := loadFixture(t, "startup_banner.txt")

	feedFixture(t, tracker, content)

	if hasThinkingEvent(*events) {
		t.Error("startup banner should not trigger thinking event")
	}
	if hasDoneEvent(*events) {
		t.Error("startup banner should not trigger done event")
	}
}

func TestFixture_SimpleResponse(t *testing.T) {
	tracker, events := newTestTracker("what does main.go do")
	content := loadFixture(t, "simple_response.txt")

	feedFixture(t, tracker, content)

	if !hasThinkingEvent(*events) {
		t.Error("expected thinking event")
	}

	doneText := extractDoneText(*events)
	if doneText == "" {
		t.Fatalf("no done event; events: %v", *events)
	}

	// Verify response content is captured.
	if !strings.Contains(doneText, "entry point for the application") {
		t.Errorf("response text missing expected content: %q", doneText)
	}
	if !strings.Contains(doneText, "runServe()") {
		t.Errorf("response text missing 'runServe()': %q", doneText)
	}

	// Verify input echo is filtered out.
	if strings.Contains(doneText, "what does main.go do") {
		t.Error("input echo leaked into response")
	}
}

func TestFixture_ToolUseRead(t *testing.T) {
	tracker, events := newTestTracker("read the config file")
	content := loadFixture(t, "tool_use_read.txt")

	feedFixture(t, tracker, content)

	doneText := extractDoneText(*events)
	if doneText == "" {
		t.Fatalf("no done event; events: %v", *events)
	}

	// Tool output should be included in the response text.
	if !strings.Contains(doneText, "Read config.yaml") {
		t.Errorf("tool use header missing from response: %q", doneText)
	}
	if !strings.Contains(doneText, "grpc_addr") {
		t.Errorf("tool output missing from response: %q", doneText)
	}
	if !strings.Contains(doneText, "gRPC server") {
		t.Errorf("explanation text missing from response: %q", doneText)
	}
}

func TestFixture_ToolUseEdit(t *testing.T) {
	tracker, events := newTestTracker("fix the nil pointer bug")
	content := loadFixture(t, "tool_use_edit.txt")

	feedFixture(t, tracker, content)

	doneText := extractDoneText(*events)
	if doneText == "" {
		t.Fatalf("no done event; events: %v", *events)
	}

	// Diff-style edit output should be captured.
	if !strings.Contains(doneText, "Edit internal/agent/manager.go") {
		t.Errorf("edit tool header missing: %q", doneText)
	}
	if !strings.Contains(doneText, "return []Agent{}") {
		t.Errorf("edit content missing: %q", doneText)
	}
}

func TestFixture_ToolUseBash(t *testing.T) {
	tracker, events := newTestTracker("run the tests")
	content := loadFixture(t, "tool_use_bash.txt")

	feedFixture(t, tracker, content)

	doneText := extractDoneText(*events)
	if doneText == "" {
		t.Fatalf("no done event; events: %v", *events)
	}

	// Bash output should be captured.
	if !strings.Contains(doneText, "Bash go test") {
		t.Errorf("bash tool header missing: %q", doneText)
	}
	if !strings.Contains(doneText, "FAIL") {
		t.Errorf("test output missing: %q", doneText)
	}
}

func TestFixture_MultiToolChain(t *testing.T) {
	tracker, events := newTestTracker("add a health check endpoint")
	content := loadFixture(t, "multi_tool_chain.txt")

	feedFixture(t, tracker, content)

	doneText := extractDoneText(*events)
	if doneText == "" {
		t.Fatalf("no done event; events: %v", *events)
	}

	// Should capture the full response including multiple tool uses.
	if !strings.Contains(doneText, "Read internal/gateway/api.go") {
		t.Errorf("first read tool missing: %q", doneText)
	}
	if !strings.Contains(doneText, "handleHealth") {
		t.Errorf("edit content missing: %q", doneText)
	}
	if !strings.Contains(doneText, "Build successful") {
		t.Errorf("bash verification missing: %q", doneText)
	}
}

func TestFixture_CodeBlockWithDashes(t *testing.T) {
	// This is a critical edge case: a code block containing "─── Frontend ───"
	// which looks similar to a horizontal rule but shouldn't end the response.
	tracker, events := newTestTracker("show me the makefile targets")
	content := loadFixture(t, "code_block_with_dashes.txt")

	feedFixture(t, tracker, content)

	doneText := extractDoneText(*events)
	if doneText == "" {
		t.Fatalf("no done event; events: %v", *events)
	}

	// The content AFTER the dash-containing comment should still be in the response.
	if !strings.Contains(doneText, "not a boundary marker") {
		t.Errorf("response truncated at dash-containing line: %q", doneText)
	}
	// The dash-containing line itself should be in the response (not treated as boundary).
	if !strings.Contains(doneText, "Frontend") {
		t.Errorf("code block content with dashes missing: %q", doneText)
	}
}

func TestFixture_PermissionPrompt(t *testing.T) {
	// Permission prompts appear mid-response. They should be captured as text.
	tracker, events := newTestTracker("delete the temp files")
	content := loadFixture(t, "permission_prompt.txt")

	feedFixture(t, tracker, content)

	doneText := extractDoneText(*events)
	if doneText == "" {
		t.Fatalf("no done event; events: %v", *events)
	}

	// The full response including permission dialog should be captured.
	if !strings.Contains(doneText, "Allow this command") {
		t.Errorf("permission prompt missing from response: %q", doneText)
	}
	if !strings.Contains(doneText, "temp directory is now clean") {
		t.Errorf("post-permission response missing: %q", doneText)
	}
}

func TestFixture_MultilineCodeBlock(t *testing.T) {
	tracker, events := newTestTracker("write a fibonacci function")
	content := loadFixture(t, "multiline_code_block.txt")

	feedFixture(t, tracker, content)

	doneText := extractDoneText(*events)
	if doneText == "" {
		t.Fatalf("no done event; events: %v", *events)
	}

	// Code block should be fully captured.
	if !strings.Contains(doneText, "func Fibonacci(n int) int") {
		t.Errorf("code block content missing: %q", doneText)
	}
	// Markdown table with dashes should not trigger boundary detection.
	if !strings.Contains(doneText, "| 9  | 34") {
		t.Errorf("table content missing (dashes in table may have triggered false boundary): %q", doneText)
	}
	// Content after table should be present.
	if !strings.Contains(doneText, "iterative approach avoids") {
		t.Errorf("content after table missing: %q", doneText)
	}
}

func TestFixture_ErrorResponse(t *testing.T) {
	// Short response with minimal thinking (single spinner frame).
	tracker, events := newTestTracker("read /etc/shadow")
	content := loadFixture(t, "error_response.txt")

	feedFixture(t, tracker, content)

	doneText := extractDoneText(*events)
	if doneText == "" {
		t.Fatalf("no done event; events: %v", *events)
	}

	if !strings.Contains(doneText, "root permissions") {
		t.Errorf("error response content missing: %q", doneText)
	}
}

func TestFixture_ChunkedDelivery(t *testing.T) {
	// Simulate pipe-pane delivering the same fixture in small chunks
	// (more realistic than single-chunk delivery).
	tracker, events := newTestTracker("what does main.go do")
	content := loadFixture(t, "simple_response.txt")

	// Feed in 80-byte chunks (simulates terminal line buffering).
	for len(content) > 0 {
		chunk := content
		if len(chunk) > 80 {
			chunk = content[:80]
		}
		content = content[len(chunk):]
		tracker.Feed(chunk)
	}
	tracker.FlushPending()

	doneText := extractDoneText(*events)
	if doneText == "" {
		t.Fatalf("chunked delivery failed to produce done event; events: %v", *events)
	}
	if !strings.Contains(doneText, "entry point") {
		t.Errorf("chunked delivery lost content: %q", doneText)
	}
}
