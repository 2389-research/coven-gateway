// ABOUTME: Tests for SSE/event formatting helper functions in api.go.
// ABOUTME: Covers pure conversion helpers that transform agent responses to SSE wire format.

package gateway

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/2389/coven-gateway/internal/agent"
)

// TestTextSSE verifies textSSE builds a correct SSEEvent with the right key/value.
func TestTextSSE(t *testing.T) {
	evt := textSSE("thinking", "text", "some thought")
	if evt.Event != "thinking" {
		t.Errorf("Event = %q, want %q", evt.Event, "thinking")
	}
	data, ok := evt.Data.(map[string]string)
	if !ok {
		t.Fatalf("Data is not map[string]string: %T", evt.Data)
	}
	if data["text"] != "some thought" {
		t.Errorf("text = %q, want %q", data["text"], "some thought")
	}
}

// TestMalformedEvent verifies malformedEvent returns an error SSE event.
func TestMalformedEvent(t *testing.T) {
	evt := malformedEvent("tool_use")
	if evt.Event != "error" {
		t.Errorf("Event = %q, want %q", evt.Event, "error")
	}
	data, ok := evt.Data.(map[string]string)
	if !ok {
		t.Fatalf("Data is not map[string]string: %T", evt.Data)
	}
	if !strings.Contains(data["error"], "tool_use") {
		t.Errorf("error message %q does not mention 'tool_use'", data["error"])
	}
}

// TestToolUseToSSE_HappyPath verifies a valid ToolUseEvent is converted correctly.
func TestToolUseToSSE_HappyPath(t *testing.T) {
	tu := &agent.ToolUseEvent{ID: "tu-1", Name: "read_file", InputJSON: `{"path":"/tmp/f"}`}
	evt := toolUseToSSE(tu)
	if evt.Event != "tool_use" {
		t.Errorf("Event = %q, want %q", evt.Event, "tool_use")
	}
	data, ok := evt.Data.(map[string]string)
	if !ok {
		t.Fatalf("Data is not map[string]string: %T", evt.Data)
	}
	if data["id"] != "tu-1" {
		t.Errorf("id = %q, want %q", data["id"], "tu-1")
	}
	if data["name"] != "read_file" {
		t.Errorf("name = %q, want %q", data["name"], "read_file")
	}
	if data["input_json"] != `{"path":"/tmp/f"}` {
		t.Errorf("input_json = %q", data["input_json"])
	}
}

// TestToolUseToSSE_Nil verifies nil input returns a malformed error event.
func TestToolUseToSSE_Nil(t *testing.T) {
	evt := toolUseToSSE(nil)
	if evt.Event != "error" {
		t.Errorf("Event = %q, want %q", evt.Event, "error")
	}
}

// TestToolResultToSSE_HappyPath verifies a valid ToolResultEvent is converted correctly.
func TestToolResultToSSE_HappyPath(t *testing.T) {
	tr := &agent.ToolResultEvent{ID: "tr-1", Output: "file contents", IsError: false}
	evt := toolResultToSSE(tr)
	if evt.Event != "tool_result" {
		t.Errorf("Event = %q, want %q", evt.Event, "tool_result")
	}
	data, ok := evt.Data.(map[string]any)
	if !ok {
		t.Fatalf("Data is not map[string]any: %T", evt.Data)
	}
	if data["id"] != "tr-1" {
		t.Errorf("id = %q, want %q", data["id"], "tr-1")
	}
	if data["output"] != "file contents" {
		t.Errorf("output = %q, want %q", data["output"], "file contents")
	}
	if data["is_error"] != false {
		t.Errorf("is_error = %v, want false", data["is_error"])
	}
}

// TestToolResultToSSE_Nil verifies nil input returns a malformed error event.
func TestToolResultToSSE_Nil(t *testing.T) {
	evt := toolResultToSSE(nil)
	if evt.Event != "error" {
		t.Errorf("Event = %q, want %q", evt.Event, "error")
	}
}

// TestFileToSSE_HappyPath verifies a valid FileEvent is converted correctly.
func TestFileToSSE_HappyPath(t *testing.T) {
	f := &agent.FileEvent{Filename: "report.pdf", MimeType: "application/pdf"}
	evt := fileToSSE(f)
	if evt.Event != "file" {
		t.Errorf("Event = %q, want %q", evt.Event, "file")
	}
	data, ok := evt.Data.(map[string]string)
	if !ok {
		t.Fatalf("Data is not map[string]string: %T", evt.Data)
	}
	if data["filename"] != "report.pdf" {
		t.Errorf("filename = %q, want %q", data["filename"], "report.pdf")
	}
	if data["mime_type"] != "application/pdf" {
		t.Errorf("mime_type = %q, want %q", data["mime_type"], "application/pdf")
	}
}

// TestFileToSSE_Nil verifies nil input returns a malformed error event.
func TestFileToSSE_Nil(t *testing.T) {
	evt := fileToSSE(nil)
	if evt.Event != "error" {
		t.Errorf("Event = %q, want %q", evt.Event, "error")
	}
}

// TestUsageToSSE_HappyPath verifies a valid UsageEvent is converted correctly.
func TestUsageToSSE_HappyPath(t *testing.T) {
	u := &agent.UsageEvent{
		InputTokens:      100,
		OutputTokens:     50,
		CacheReadTokens:  10,
		CacheWriteTokens: 5,
		ThinkingTokens:   20,
	}
	evt := usageToSSE(u)
	if evt.Event != "usage" {
		t.Errorf("Event = %q, want %q", evt.Event, "usage")
	}
	data, ok := evt.Data.(map[string]any)
	if !ok {
		t.Fatalf("Data is not map[string]any: %T", evt.Data)
	}
	if data["input_tokens"] != int32(100) {
		t.Errorf("input_tokens = %v, want 100", data["input_tokens"])
	}
	if data["output_tokens"] != int32(50) {
		t.Errorf("output_tokens = %v, want 50", data["output_tokens"])
	}
	if data["cache_read_tokens"] != int32(10) {
		t.Errorf("cache_read_tokens = %v, want 10", data["cache_read_tokens"])
	}
	if data["thinking_tokens"] != int32(20) {
		t.Errorf("thinking_tokens = %v, want 20", data["thinking_tokens"])
	}
}

// TestUsageToSSE_Nil verifies nil input returns a malformed error event.
func TestUsageToSSE_Nil(t *testing.T) {
	evt := usageToSSE(nil)
	if evt.Event != "error" {
		t.Errorf("Event = %q, want %q", evt.Event, "error")
	}
}

// TestToolStateToSSE_HappyPath verifies a valid ToolStateEvent is converted correctly.
func TestToolStateToSSE_HappyPath(t *testing.T) {
	ts := &agent.ToolStateEvent{ID: "ts-1", State: "running", Detail: "reading file"}
	evt := toolStateToSSE(ts)
	if evt.Event != "tool_state" {
		t.Errorf("Event = %q, want %q", evt.Event, "tool_state")
	}
	data, ok := evt.Data.(map[string]string)
	if !ok {
		t.Fatalf("Data is not map[string]string: %T", evt.Data)
	}
	if data["id"] != "ts-1" {
		t.Errorf("id = %q, want %q", data["id"], "ts-1")
	}
	if data["state"] != "running" {
		t.Errorf("state = %q, want %q", data["state"], "running")
	}
	if data["detail"] != "reading file" {
		t.Errorf("detail = %q, want %q", data["detail"], "reading file")
	}
}

// TestToolStateToSSE_Nil verifies nil input returns a malformed error event.
func TestToolStateToSSE_Nil(t *testing.T) {
	evt := toolStateToSSE(nil)
	if evt.Event != "error" {
		t.Errorf("Event = %q, want %q", evt.Event, "error")
	}
}

// TestToolApprovalToSSE_HappyPath verifies a valid ToolApprovalRequestEvent is converted correctly.
func TestToolApprovalToSSE_HappyPath(t *testing.T) {
	ta := &agent.ToolApprovalRequestEvent{
		ID:        "ta-1",
		Name:      "delete_file",
		InputJSON: `{"path":"/etc/passwd"}`,
		RequestID: "req-99",
	}
	evt := toolApprovalToSSE(ta)
	if evt.Event != "tool_approval" {
		t.Errorf("Event = %q, want %q", evt.Event, "tool_approval")
	}
	data, ok := evt.Data.(map[string]string)
	if !ok {
		t.Fatalf("Data is not map[string]string: %T", evt.Data)
	}
	if data["id"] != "ta-1" {
		t.Errorf("id = %q, want %q", data["id"], "ta-1")
	}
	if data["name"] != "delete_file" {
		t.Errorf("name = %q, want %q", data["name"], "delete_file")
	}
	if data["request_id"] != "req-99" {
		t.Errorf("request_id = %q, want %q", data["request_id"], "req-99")
	}
}

// TestToolApprovalToSSE_Nil verifies nil input returns a malformed error event.
func TestToolApprovalToSSE_Nil(t *testing.T) {
	evt := toolApprovalToSSE(nil)
	if evt.Event != "error" {
		t.Errorf("Event = %q, want %q", evt.Event, "error")
	}
}

// TestResponseToSSEEvent_AllArms exercises every converter arm in sseConverters plus the fallback.
func TestResponseToSSEEvent_AllArms(t *testing.T) {
	gw := newTestGateway(t)

	tests := []struct {
		name      string
		resp      *agent.Response
		wantEvent string
		checkData func(t *testing.T, data any)
	}{
		{
			name:      "thinking",
			resp:      &agent.Response{Event: agent.EventThinking, Text: "thinking..."},
			wantEvent: "thinking",
			checkData: func(t *testing.T, data any) {
				m := data.(map[string]string)
				if m["text"] != "thinking..." {
					t.Errorf("text = %q, want %q", m["text"], "thinking...")
				}
			},
		},
		{
			name:      "text",
			resp:      &agent.Response{Event: agent.EventText, Text: "hello"},
			wantEvent: "text",
			checkData: func(t *testing.T, data any) {
				m := data.(map[string]string)
				if m["text"] != "hello" {
					t.Errorf("text = %q, want %q", m["text"], "hello")
				}
			},
		},
		{
			name: "tool_use",
			resp: &agent.Response{
				Event:   agent.EventToolUse,
				ToolUse: &agent.ToolUseEvent{ID: "x", Name: "tool", InputJSON: "{}"},
			},
			wantEvent: "tool_use",
			checkData: func(t *testing.T, data any) {
				m := data.(map[string]string)
				if m["name"] != "tool" {
					t.Errorf("name = %q, want %q", m["name"], "tool")
				}
			},
		},
		{
			name: "tool_result",
			resp: &agent.Response{
				Event:      agent.EventToolResult,
				ToolResult: &agent.ToolResultEvent{ID: "y", Output: "ok"},
			},
			wantEvent: "tool_result",
			checkData: func(t *testing.T, data any) {
				m := data.(map[string]any)
				if m["output"] != "ok" {
					t.Errorf("output = %q, want %q", m["output"], "ok")
				}
			},
		},
		{
			name:      "file",
			resp:      &agent.Response{Event: agent.EventFile, File: &agent.FileEvent{Filename: "f.txt", MimeType: "text/plain"}},
			wantEvent: "file",
			checkData: func(t *testing.T, data any) {
				m := data.(map[string]string)
				if m["filename"] != "f.txt" {
					t.Errorf("filename = %q, want %q", m["filename"], "f.txt")
				}
			},
		},
		{
			name:      "done",
			resp:      &agent.Response{Event: agent.EventDone, Text: "full response", Done: true},
			wantEvent: "done",
			checkData: func(t *testing.T, data any) {
				m := data.(map[string]string)
				if m["full_response"] != "full response" {
					t.Errorf("full_response = %q, want %q", m["full_response"], "full response")
				}
			},
		},
		{
			name:      "error",
			resp:      &agent.Response{Event: agent.EventError, Error: "boom"},
			wantEvent: "error",
			checkData: func(t *testing.T, data any) {
				m := data.(map[string]string)
				if m["error"] != "boom" {
					t.Errorf("error = %q, want %q", m["error"], "boom")
				}
			},
		},
		{
			name:      "session_init",
			resp:      &agent.Response{Event: agent.EventSessionInit, SessionID: "sess-1"},
			wantEvent: "session_init",
			checkData: func(t *testing.T, data any) {
				m := data.(map[string]string)
				if m["session_id"] != "sess-1" {
					t.Errorf("session_id = %q, want %q", m["session_id"], "sess-1")
				}
			},
		},
		{
			name:      "session_orphaned",
			resp:      &agent.Response{Event: agent.EventSessionOrphaned, Error: "orphaned reason"},
			wantEvent: "session_orphaned",
			checkData: func(t *testing.T, data any) {
				m := data.(map[string]string)
				if m["reason"] != "orphaned reason" {
					t.Errorf("reason = %q, want %q", m["reason"], "orphaned reason")
				}
			},
		},
		{
			name: "usage",
			resp: &agent.Response{
				Event: agent.EventUsage,
				Usage: &agent.UsageEvent{InputTokens: 5, OutputTokens: 3},
			},
			wantEvent: "usage",
			checkData: func(t *testing.T, data any) {
				m := data.(map[string]any)
				if m["input_tokens"] != int32(5) {
					t.Errorf("input_tokens = %v, want 5", m["input_tokens"])
				}
			},
		},
		{
			name: "tool_state",
			resp: &agent.Response{
				Event:     agent.EventToolState,
				ToolState: &agent.ToolStateEvent{ID: "z", State: "done", Detail: ""},
			},
			wantEvent: "tool_state",
			checkData: func(t *testing.T, data any) {
				m := data.(map[string]string)
				if m["state"] != "done" {
					t.Errorf("state = %q, want %q", m["state"], "done")
				}
			},
		},
		{
			name:      "canceled",
			resp:      &agent.Response{Event: agent.EventCanceled, Error: "user canceled"},
			wantEvent: "canceled",
			checkData: func(t *testing.T, data any) {
				m := data.(map[string]string)
				if m["reason"] != "user canceled" {
					t.Errorf("reason = %q, want %q", m["reason"], "user canceled")
				}
			},
		},
		{
			name: "tool_approval",
			resp: &agent.Response{
				Event:               agent.EventToolApprovalRequest,
				ToolApprovalRequest: &agent.ToolApprovalRequestEvent{ID: "a", Name: "rm", InputJSON: "{}", RequestID: "r1"},
			},
			wantEvent: "tool_approval",
			checkData: func(t *testing.T, data any) {
				m := data.(map[string]string)
				if m["name"] != "rm" {
					t.Errorf("name = %q, want %q", m["name"], "rm")
				}
			},
		},
		{
			name:      "unknown_type_fallback",
			resp:      &agent.Response{Event: agent.ResponseEvent(9999), Text: "mystery"},
			wantEvent: "unknown",
			checkData: func(t *testing.T, data any) {
				m := data.(map[string]string)
				if m["text"] != "mystery" {
					t.Errorf("text = %q, want %q", m["text"], "mystery")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evt := gw.responseToSSEEvent(tt.resp)
			if evt.Event != tt.wantEvent {
				t.Errorf("Event = %q, want %q", evt.Event, tt.wantEvent)
			}
			if tt.checkData != nil {
				tt.checkData(t, evt.Data)
			}
		})
	}
}

// TestWriteSSEEvent_HappyPath verifies writeSSEEvent writes correct SSE lines to a recorder.
func TestWriteSSEEvent_HappyPath(t *testing.T) {
	gw := newTestGateway(t)
	rec := httptest.NewRecorder()

	gw.writeSSEEvent(rec, "text", map[string]string{"text": "hello world"})

	body := rec.Body.String()
	if !strings.Contains(body, "event: text\n") {
		t.Errorf("body missing 'event: text\\n', got: %q", body)
	}
	if !strings.Contains(body, "data: ") {
		t.Errorf("body missing 'data: ', got: %q", body)
	}

	// Parse the data portion to verify JSON correctness
	dataLine := ""
	for line := range strings.SplitSeq(body, "\n") {
		if rest, ok := strings.CutPrefix(line, "data: "); ok {
			dataLine = rest
			break
		}
	}
	var decoded map[string]string
	if err := json.Unmarshal([]byte(dataLine), &decoded); err != nil {
		t.Fatalf("data JSON invalid: %v (raw: %q)", err, dataLine)
	}
	if decoded["text"] != "hello world" {
		t.Errorf("decoded text = %q, want %q", decoded["text"], "hello world")
	}
}

// TestExtractPathSegment covers all branches: success, missing prefix, missing suffix,
// empty segment, path traversal, and embedded slash.
func TestExtractPathSegment(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		prefix      string
		suffix      string
		wantSegment string
		wantOK      bool
	}{
		{
			name:        "valid segment",
			path:        "/api/agents/abc123/history",
			prefix:      "/api/agents/",
			suffix:      "/history",
			wantSegment: "abc123",
			wantOK:      true,
		},
		{
			name:        "missing prefix",
			path:        "/other/abc123/history",
			prefix:      "/api/agents/",
			suffix:      "/history",
			wantSegment: "",
			wantOK:      false,
		},
		{
			name:        "missing suffix",
			path:        "/api/agents/abc123",
			prefix:      "/api/agents/",
			suffix:      "/history",
			wantSegment: "",
			wantOK:      false,
		},
		{
			name:        "empty segment (trailing slash after prefix before suffix)",
			path:        "/api/agents//history",
			prefix:      "/api/agents/",
			suffix:      "/history",
			wantSegment: "",
			wantOK:      false,
		},
		{
			name:        "path traversal",
			path:        "/api/agents/../history",
			prefix:      "/api/agents/",
			suffix:      "/history",
			wantSegment: "",
			wantOK:      false,
		},
		{
			name:        "embedded slash in segment",
			path:        "/api/agents/a/b/history",
			prefix:      "/api/agents/",
			suffix:      "/history",
			wantSegment: "",
			wantOK:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := extractPathSegment(tt.path, tt.prefix, tt.suffix)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.wantSegment {
				t.Errorf("segment = %q, want %q", got, tt.wantSegment)
			}
		})
	}
}

// TestSendJSONError verifies sendJSONError sets the correct status code and JSON body.
func TestSendJSONError(t *testing.T) {
	gw := newTestGateway(t)
	rec := httptest.NewRecorder()

	gw.sendJSONError(rec, 400, "something went wrong")

	if rec.Code != 400 {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode error body: %v", err)
	}
	if body["error"] != "something went wrong" {
		t.Errorf("error = %q, want %q", body["error"], "something went wrong")
	}
}

// TestFormatSSEEvent_Format verifies the exact wire format produced by formatSSEEvent.
func TestFormatSSEEvent_Format(t *testing.T) {
	got := formatSSEEvent("done", `{"full_response":"ok"}`)
	want := "event: done\ndata: {\"full_response\":\"ok\"}\n\n"
	if got != want {
		t.Errorf("formatSSEEvent = %q, want %q", got, want)
	}
}

// TestFormatSSEEvent_EmptyData verifies formatSSEEvent works with an empty data string.
func TestFormatSSEEvent_EmptyData(t *testing.T) {
	got := formatSSEEvent("ping", "")
	if !strings.HasPrefix(got, "event: ping\n") {
		t.Errorf("expected 'event: ping\\n' prefix, got %q", got)
	}
	if !strings.HasSuffix(got, "\n\n") {
		t.Errorf("expected trailing '\\n\\n', got %q", got)
	}
}
