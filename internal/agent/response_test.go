// ABOUTME: Tests for response builders and conversion functions in the agent manager.
// ABOUTME: Validates correct transformation from protobuf types to internal Response types.

package agent

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "github.com/2389/coven-gateway/proto/coven"
)

// =============================================================================
// Response Builder Tests
// =============================================================================

func TestBuildThinkingResponse(t *testing.T) {
	event := &pb.MessageResponse_Thinking{Thinking: "I am pondering..."}
	resp := buildThinkingResponse(event)

	assert.Equal(t, EventThinking, resp.Event)
	assert.Equal(t, "I am pondering...", resp.Text)
}

func TestBuildTextResponse(t *testing.T) {
	event := &pb.MessageResponse_Text{Text: "Hello, world!"}
	resp := buildTextResponse(event)

	assert.Equal(t, EventText, resp.Event)
	assert.Equal(t, "Hello, world!", resp.Text)
}

func TestBuildToolUseResponse(t *testing.T) {
	event := &pb.MessageResponse_ToolUse{
		ToolUse: &pb.ToolUse{
			Id:        "tool-123",
			Name:      "read_file",
			InputJson: `{"path": "/tmp/test.txt"}`,
		},
	}
	resp := buildToolUseResponse(event)

	assert.Equal(t, EventToolUse, resp.Event)
	require.NotNil(t, resp.ToolUse)
	assert.Equal(t, "tool-123", resp.ToolUse.ID)
	assert.Equal(t, "read_file", resp.ToolUse.Name)
	assert.Equal(t, `{"path": "/tmp/test.txt"}`, resp.ToolUse.InputJSON)
}

func TestBuildToolResultResponse(t *testing.T) {
	event := &pb.MessageResponse_ToolResult{
		ToolResult: &pb.ToolResult{
			Id:      "tool-123",
			Output:  "file contents here",
			IsError: false,
		},
	}
	resp := buildToolResultResponse(event)

	assert.Equal(t, EventToolResult, resp.Event)
	require.NotNil(t, resp.ToolResult)
	assert.Equal(t, "tool-123", resp.ToolResult.ID)
	assert.Equal(t, "file contents here", resp.ToolResult.Output)
	assert.False(t, resp.ToolResult.IsError)
}

func TestBuildToolResultResponse_Error(t *testing.T) {
	event := &pb.MessageResponse_ToolResult{
		ToolResult: &pb.ToolResult{
			Id:      "tool-456",
			Output:  "permission denied",
			IsError: true,
		},
	}
	resp := buildToolResultResponse(event)

	assert.Equal(t, EventToolResult, resp.Event)
	require.NotNil(t, resp.ToolResult)
	assert.Equal(t, "tool-456", resp.ToolResult.ID)
	assert.True(t, resp.ToolResult.IsError)
}

func TestBuildFileResponse(t *testing.T) {
	event := &pb.MessageResponse_File{
		File: &pb.FileData{
			Filename: "report.pdf",
			MimeType: "application/pdf",
			Data:     []byte("PDF content"),
		},
	}
	resp := buildFileResponse(event)

	assert.Equal(t, EventFile, resp.Event)
	require.NotNil(t, resp.File)
	assert.Equal(t, "report.pdf", resp.File.Filename)
	assert.Equal(t, "application/pdf", resp.File.MimeType)
	assert.Equal(t, []byte("PDF content"), resp.File.Data)
}

func TestBuildDoneResponse(t *testing.T) {
	event := &pb.MessageResponse_Done{
		Done: &pb.Done{
			FullResponse: "Complete response text",
		},
	}
	resp := buildDoneResponse(event)

	assert.Equal(t, EventDone, resp.Event)
	assert.Equal(t, "Complete response text", resp.Text)
	assert.True(t, resp.Done)
}

func TestBuildErrorResponse(t *testing.T) {
	event := &pb.MessageResponse_Error{Error: "Something went wrong"}
	resp := buildErrorResponse(event)

	assert.Equal(t, EventError, resp.Event)
	assert.Equal(t, "Something went wrong", resp.Error)
	assert.True(t, resp.Done)
}

func TestBuildSessionInitResponse(t *testing.T) {
	event := &pb.MessageResponse_SessionInit{
		SessionInit: &pb.SessionInit{
			SessionId: "session-abc-123",
		},
	}
	resp := buildSessionInitResponse(event)

	assert.Equal(t, EventSessionInit, resp.Event)
	assert.Equal(t, "session-abc-123", resp.SessionID)
}

func TestBuildSessionOrphanedResponse(t *testing.T) {
	event := &pb.MessageResponse_SessionOrphaned{
		SessionOrphaned: &pb.SessionOrphaned{
			Reason: "Client disconnected",
		},
	}
	resp := buildSessionOrphanedResponse(event)

	assert.Equal(t, EventSessionOrphaned, resp.Event)
	assert.Equal(t, "Client disconnected", resp.Error)
}

func TestBuildUsageResponse(t *testing.T) {
	event := &pb.MessageResponse_Usage{
		Usage: &pb.TokenUsage{
			InputTokens:      100,
			OutputTokens:     200,
			CacheReadTokens:  50,
			CacheWriteTokens: 25,
			ThinkingTokens:   10,
		},
	}
	resp := buildUsageResponse(event)

	assert.Equal(t, EventUsage, resp.Event)
	require.NotNil(t, resp.Usage)
	assert.Equal(t, int32(100), resp.Usage.InputTokens)
	assert.Equal(t, int32(200), resp.Usage.OutputTokens)
	assert.Equal(t, int32(50), resp.Usage.CacheReadTokens)
	assert.Equal(t, int32(25), resp.Usage.CacheWriteTokens)
	assert.Equal(t, int32(10), resp.Usage.ThinkingTokens)
}

func TestBuildToolStateResponse(t *testing.T) {
	detail := "Processing..."
	event := &pb.MessageResponse_ToolState{
		ToolState: &pb.ToolStateUpdate{
			Id:     "tool-789",
			State:  pb.ToolState_TOOL_STATE_RUNNING,
			Detail: &detail,
		},
	}
	resp := buildToolStateResponse(event)

	assert.Equal(t, EventToolState, resp.Event)
	require.NotNil(t, resp.ToolState)
	assert.Equal(t, "tool-789", resp.ToolState.ID)
	assert.Equal(t, "running", resp.ToolState.State)
	assert.Equal(t, "Processing...", resp.ToolState.Detail)
}

func TestBuildToolStateResponse_NilDetail(t *testing.T) {
	// The detail field is optional in protobuf - test with nil
	event := &pb.MessageResponse_ToolState{
		ToolState: &pb.ToolStateUpdate{
			Id:     "tool-999",
			State:  pb.ToolState_TOOL_STATE_COMPLETED,
			Detail: nil, // optional field not set
		},
	}
	resp := buildToolStateResponse(event)

	assert.Equal(t, EventToolState, resp.Event)
	require.NotNil(t, resp.ToolState)
	assert.Equal(t, "tool-999", resp.ToolState.ID)
	assert.Equal(t, "completed", resp.ToolState.State)
	assert.Equal(t, "", resp.ToolState.Detail) // GetDetail returns empty string for nil
}

func TestBuildCancelledResponse(t *testing.T) {
	event := &pb.MessageResponse_Cancelled{
		Cancelled: &pb.Cancelled{
			Reason: "User requested cancellation",
		},
	}
	resp := buildCancelledResponse(event)

	assert.Equal(t, EventCanceled, resp.Event)
	assert.Equal(t, "User requested cancellation", resp.Error)
	assert.True(t, resp.Done)
}

func TestBuildToolApprovalRequestResponse(t *testing.T) {
	event := &pb.MessageResponse_ToolApprovalRequest{
		ToolApprovalRequest: &pb.ToolApprovalRequest{
			Id:        "approval-001",
			Name:      "execute_command",
			InputJson: `{"command": "rm -rf /tmp/test"}`,
		},
	}
	resp := buildToolApprovalRequestResponse(event, "req-xyz")

	assert.Equal(t, EventToolApprovalRequest, resp.Event)
	require.NotNil(t, resp.ToolApprovalRequest)
	assert.Equal(t, "approval-001", resp.ToolApprovalRequest.ID)
	assert.Equal(t, "execute_command", resp.ToolApprovalRequest.Name)
	assert.Equal(t, `{"command": "rm -rf /tmp/test"}`, resp.ToolApprovalRequest.InputJSON)
	assert.Equal(t, "req-xyz", resp.ToolApprovalRequest.RequestID)
}

// =============================================================================
// Tool State String Conversion Tests
// =============================================================================

func TestToolStateToString(t *testing.T) {
	tests := []struct {
		state    pb.ToolState
		expected string
	}{
		{pb.ToolState_TOOL_STATE_UNSPECIFIED, "unknown"}, // UNSPECIFIED (value 0) falls through to default
		{pb.ToolState_TOOL_STATE_PENDING, "pending"},
		{pb.ToolState_TOOL_STATE_AWAITING_APPROVAL, "awaiting_approval"},
		{pb.ToolState_TOOL_STATE_RUNNING, "running"},
		{pb.ToolState_TOOL_STATE_COMPLETED, "completed"},
		{pb.ToolState_TOOL_STATE_FAILED, "failed"},
		{pb.ToolState_TOOL_STATE_DENIED, "denied"},
		{pb.ToolState_TOOL_STATE_TIMEOUT, "timeout"},
		{pb.ToolState_TOOL_STATE_CANCELLED, "canceled"},
		{pb.ToolState(999), "unknown"}, // Unknown state
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := toolStateToString(tt.state)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// Content Event Conversion Tests
// =============================================================================

func TestConvertContentEvent(t *testing.T) {
	t.Run("converts thinking event", func(t *testing.T) {
		event := &pb.MessageResponse_Thinking{Thinking: "Analyzing..."}
		resp := convertContentEvent(event)
		require.NotNil(t, resp)
		assert.Equal(t, EventThinking, resp.Event)
	})

	t.Run("converts text event", func(t *testing.T) {
		event := &pb.MessageResponse_Text{Text: "Response text"}
		resp := convertContentEvent(event)
		require.NotNil(t, resp)
		assert.Equal(t, EventText, resp.Event)
	})

	t.Run("converts tool use event", func(t *testing.T) {
		event := &pb.MessageResponse_ToolUse{
			ToolUse: &pb.ToolUse{Id: "t1", Name: "test"},
		}
		resp := convertContentEvent(event)
		require.NotNil(t, resp)
		assert.Equal(t, EventToolUse, resp.Event)
	})

	t.Run("converts tool result event", func(t *testing.T) {
		event := &pb.MessageResponse_ToolResult{
			ToolResult: &pb.ToolResult{Id: "t1", Output: "result"},
		}
		resp := convertContentEvent(event)
		require.NotNil(t, resp)
		assert.Equal(t, EventToolResult, resp.Event)
	})

	t.Run("converts file event", func(t *testing.T) {
		event := &pb.MessageResponse_File{
			File: &pb.FileData{Filename: "test.txt"},
		}
		resp := convertContentEvent(event)
		require.NotNil(t, resp)
		assert.Equal(t, EventFile, resp.Event)
	})

	t.Run("returns nil for unknown event", func(t *testing.T) {
		resp := convertContentEvent("not a valid event")
		assert.Nil(t, resp)
	})
}

// =============================================================================
// Control Event Conversion Tests
// =============================================================================

func TestConvertControlEvent(t *testing.T) {
	t.Run("converts done event", func(t *testing.T) {
		event := &pb.MessageResponse_Done{
			Done: &pb.Done{FullResponse: "Complete"},
		}
		resp := convertControlEvent(event)
		require.NotNil(t, resp)
		assert.Equal(t, EventDone, resp.Event)
		assert.True(t, resp.Done)
	})

	t.Run("converts error event", func(t *testing.T) {
		event := &pb.MessageResponse_Error{Error: "Failed"}
		resp := convertControlEvent(event)
		require.NotNil(t, resp)
		assert.Equal(t, EventError, resp.Event)
		assert.True(t, resp.Done)
	})

	t.Run("converts session init event", func(t *testing.T) {
		event := &pb.MessageResponse_SessionInit{
			SessionInit: &pb.SessionInit{SessionId: "sess-1"},
		}
		resp := convertControlEvent(event)
		require.NotNil(t, resp)
		assert.Equal(t, EventSessionInit, resp.Event)
	})

	t.Run("converts session orphaned event", func(t *testing.T) {
		event := &pb.MessageResponse_SessionOrphaned{
			SessionOrphaned: &pb.SessionOrphaned{Reason: "timeout"},
		}
		resp := convertControlEvent(event)
		require.NotNil(t, resp)
		assert.Equal(t, EventSessionOrphaned, resp.Event)
	})

	t.Run("converts usage event", func(t *testing.T) {
		event := &pb.MessageResponse_Usage{
			Usage: &pb.TokenUsage{InputTokens: 100},
		}
		resp := convertControlEvent(event)
		require.NotNil(t, resp)
		assert.Equal(t, EventUsage, resp.Event)
	})

	t.Run("returns nil for unknown event", func(t *testing.T) {
		resp := convertControlEvent(42)
		assert.Nil(t, resp)
	})
}

// =============================================================================
// Tool State Event Conversion Tests
// =============================================================================

func TestConvertToolStateEvent(t *testing.T) {
	t.Run("converts tool state event", func(t *testing.T) {
		event := &pb.MessageResponse_ToolState{
			ToolState: &pb.ToolStateUpdate{Id: "ts-1", State: pb.ToolState_TOOL_STATE_RUNNING},
		}
		resp := convertToolStateEvent(event, "req-1")
		require.NotNil(t, resp)
		assert.Equal(t, EventToolState, resp.Event)
	})

	t.Run("converts cancelled event", func(t *testing.T) {
		event := &pb.MessageResponse_Cancelled{
			Cancelled: &pb.Cancelled{Reason: "User cancelled"},
		}
		resp := convertToolStateEvent(event, "req-2")
		require.NotNil(t, resp)
		assert.Equal(t, EventCanceled, resp.Event)
		assert.True(t, resp.Done)
	})

	t.Run("converts tool approval request event", func(t *testing.T) {
		event := &pb.MessageResponse_ToolApprovalRequest{
			ToolApprovalRequest: &pb.ToolApprovalRequest{Id: "ta-1", Name: "exec"},
		}
		resp := convertToolStateEvent(event, "req-3")
		require.NotNil(t, resp)
		assert.Equal(t, EventToolApprovalRequest, resp.Event)
		assert.Equal(t, "req-3", resp.ToolApprovalRequest.RequestID)
	})

	t.Run("returns nil for unknown event", func(t *testing.T) {
		resp := convertToolStateEvent("invalid", "req-4")
		assert.Nil(t, resp)
	})
}

// =============================================================================
// Manager ConvertResponse Tests
// =============================================================================

func TestManagerConvertResponse(t *testing.T) {
	m := NewManager(slog.Default())

	t.Run("converts content events", func(t *testing.T) {
		pbResp := &pb.MessageResponse{
			RequestId: "req-1",
			Event: &pb.MessageResponse_Text{
				Text: "Hello",
			},
		}
		resp := m.convertResponse(pbResp)
		assert.Equal(t, EventText, resp.Event)
		assert.Equal(t, "Hello", resp.Text)
	})

	t.Run("converts control events", func(t *testing.T) {
		pbResp := &pb.MessageResponse{
			RequestId: "req-2",
			Event: &pb.MessageResponse_Done{
				Done: &pb.Done{FullResponse: "Complete"},
			},
		}
		resp := m.convertResponse(pbResp)
		assert.Equal(t, EventDone, resp.Event)
		assert.True(t, resp.Done)
	})

	t.Run("converts tool state events with request ID", func(t *testing.T) {
		pbResp := &pb.MessageResponse{
			RequestId: "req-3",
			Event: &pb.MessageResponse_ToolApprovalRequest{
				ToolApprovalRequest: &pb.ToolApprovalRequest{
					Id:   "ta-1",
					Name: "dangerous_command",
				},
			},
		}
		resp := m.convertResponse(pbResp)
		assert.Equal(t, EventToolApprovalRequest, resp.Event)
		require.NotNil(t, resp.ToolApprovalRequest)
		assert.Equal(t, "req-3", resp.ToolApprovalRequest.RequestID)
	})

	t.Run("returns zero-value event for missing event", func(t *testing.T) {
		pbResp := &pb.MessageResponse{
			RequestId: "req-4",
			// No event set
		}
		resp := m.convertResponse(pbResp)
		// Should return empty Response, not nil
		// Note: zero-value for ResponseEvent is EventThinking (0)
		assert.Equal(t, ResponseEvent(0), resp.Event)
	})
}

// =============================================================================
// Nil Safety Tests
// =============================================================================

func TestBuildResponsesWithNilFields(t *testing.T) {
	t.Run("tool use with nil inner", func(t *testing.T) {
		event := &pb.MessageResponse_ToolUse{ToolUse: nil}
		resp := buildToolUseResponse(event)
		require.NotNil(t, resp)
		// GetXxx methods should return zero values for nil
		assert.Equal(t, "", resp.ToolUse.ID)
	})

	t.Run("tool result with nil inner", func(t *testing.T) {
		event := &pb.MessageResponse_ToolResult{ToolResult: nil}
		resp := buildToolResultResponse(event)
		require.NotNil(t, resp)
		assert.Equal(t, "", resp.ToolResult.ID)
	})

	t.Run("done with nil inner", func(t *testing.T) {
		event := &pb.MessageResponse_Done{Done: nil}
		resp := buildDoneResponse(event)
		require.NotNil(t, resp)
		assert.Equal(t, "", resp.Text)
	})

	t.Run("session init with nil inner", func(t *testing.T) {
		event := &pb.MessageResponse_SessionInit{SessionInit: nil}
		resp := buildSessionInitResponse(event)
		require.NotNil(t, resp)
		assert.Equal(t, "", resp.SessionID)
	})

	t.Run("usage with nil inner", func(t *testing.T) {
		event := &pb.MessageResponse_Usage{Usage: nil}
		resp := buildUsageResponse(event)
		require.NotNil(t, resp)
		assert.Equal(t, int32(0), resp.Usage.InputTokens)
	})

	t.Run("file with nil inner", func(t *testing.T) {
		event := &pb.MessageResponse_File{File: nil}
		resp := buildFileResponse(event)
		require.NotNil(t, resp)
		assert.Equal(t, EventFile, resp.Event)
		require.NotNil(t, resp.File)
		assert.Equal(t, "", resp.File.Filename)
		assert.Equal(t, "", resp.File.MimeType)
		assert.Nil(t, resp.File.Data)
	})

	t.Run("session orphaned with nil inner", func(t *testing.T) {
		event := &pb.MessageResponse_SessionOrphaned{SessionOrphaned: nil}
		resp := buildSessionOrphanedResponse(event)
		require.NotNil(t, resp)
		assert.Equal(t, EventSessionOrphaned, resp.Event)
		assert.Equal(t, "", resp.Error)
	})

	t.Run("tool state with nil inner", func(t *testing.T) {
		event := &pb.MessageResponse_ToolState{ToolState: nil}
		resp := buildToolStateResponse(event)
		require.NotNil(t, resp)
		assert.Equal(t, EventToolState, resp.Event)
		require.NotNil(t, resp.ToolState)
		assert.Equal(t, "", resp.ToolState.ID)
		assert.Equal(t, "unknown", resp.ToolState.State) // TOOL_STATE_UNSPECIFIED (0) maps to "unknown"
		assert.Equal(t, "", resp.ToolState.Detail)
	})

	t.Run("cancelled with nil inner", func(t *testing.T) {
		event := &pb.MessageResponse_Cancelled{Cancelled: nil}
		resp := buildCancelledResponse(event)
		require.NotNil(t, resp)
		assert.Equal(t, EventCanceled, resp.Event)
		assert.Equal(t, "", resp.Error)
		assert.True(t, resp.Done)
	})

	t.Run("tool approval request with nil inner", func(t *testing.T) {
		event := &pb.MessageResponse_ToolApprovalRequest{ToolApprovalRequest: nil}
		resp := buildToolApprovalRequestResponse(event, "req-test")
		require.NotNil(t, resp)
		assert.Equal(t, EventToolApprovalRequest, resp.Event)
		require.NotNil(t, resp.ToolApprovalRequest)
		assert.Equal(t, "", resp.ToolApprovalRequest.ID)
		assert.Equal(t, "", resp.ToolApprovalRequest.Name)
		assert.Equal(t, "", resp.ToolApprovalRequest.InputJSON)
		assert.Equal(t, "req-test", resp.ToolApprovalRequest.RequestID)
	})
}
