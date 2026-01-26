# Fold Protocol Enhancements

**Date:** 2026-01-19
**Status:** Draft Proposal
**Inspired by:** Ent Protocol (Lace)

---

## Summary

Four enhancements to steal from Ent:

1. **Token Usage Tracking** - Visibility into agent token consumption
2. **Structured Tool States** - Rich lifecycle for tool execution
3. **Context Injection** - Push context to agents mid-turn
4. **Request Cancellation** - Clean abort for in-flight requests

---

## 1. Token Usage Tracking

### Problem

Zero visibility into agent token consumption. Can't do budget control, cost attribution, or know when agents are hitting context limits.

### Proto Changes

```protobuf
// Token usage statistics from the LLM provider
message TokenUsage {
  int32 input_tokens = 1;         // Tokens in the prompt
  int32 output_tokens = 2;        // Tokens generated
  int32 cache_read_tokens = 3;    // Tokens read from cache (Anthropic)
  int32 cache_write_tokens = 4;   // Tokens written to cache (Anthropic)
  int32 thinking_tokens = 5;      // Extended thinking tokens (Claude)
}

// Add to MessageResponse.event oneof:
message MessageResponse {
  string request_id = 1;
  oneof event {
    string thinking = 2;
    string text = 3;
    ToolUse tool_use = 4;
    ToolResult tool_result = 5;
    Done done = 6;
    string error = 7;
    FileData file = 8;
    ToolApprovalRequest tool_approval_request = 9;
    SessionInit session_init = 10;
    SessionOrphaned session_orphaned = 11;
    TokenUsage usage = 12;          // NEW: Token consumption update
  }
}
```

### Usage Pattern

Agent sends `usage` event after each LLM call (can be multiple per request if tools involved):

```
Agent → Gateway: MessageResponse { request_id: "abc", usage: { input: 1500, output: 200 } }
Agent → Gateway: MessageResponse { request_id: "abc", text: "Here's what I found..." }
Agent → Gateway: MessageResponse { request_id: "abc", tool_use: { ... } }
Agent → Gateway: MessageResponse { request_id: "abc", usage: { input: 2000, output: 150 } }
Agent → Gateway: MessageResponse { request_id: "abc", done: {} }
```

### Gateway Behavior

- Store cumulative usage in ledger event
- Optionally enforce budget limits (future)
- Forward to clients for display

---

## 2. Structured Tool States

### Problem

Current model: `tool_use` → `tool_result`. No visibility into:
- Tool waiting for approval
- Tool currently executing
- Tool timed out vs failed vs denied

### Proto Changes

```protobuf
// Tool execution state (for ToolStateUpdate)
enum ToolState {
  TOOL_STATE_UNSPECIFIED = 0;
  TOOL_STATE_PENDING = 1;           // Tool identified, not yet started
  TOOL_STATE_AWAITING_APPROVAL = 2; // Waiting for human approval
  TOOL_STATE_RUNNING = 3;           // Actively executing
  TOOL_STATE_COMPLETED = 4;         // Finished successfully
  TOOL_STATE_FAILED = 5;            // Execution error
  TOOL_STATE_DENIED = 6;            // Approval denied
  TOOL_STATE_TIMEOUT = 7;           // Execution timed out
  TOOL_STATE_CANCELLED = 8;         // Cancelled by user/system
}

// Tool state transition notification
message ToolStateUpdate {
  string id = 1;                    // Tool invocation ID (matches ToolUse.id)
  ToolState state = 2;              // New state
  optional string detail = 3;       // Optional detail (error message, etc.)
}

// Add to MessageResponse.event oneof:
message MessageResponse {
  string request_id = 1;
  oneof event {
    // ... existing fields ...
    TokenUsage usage = 12;
    ToolStateUpdate tool_state = 13;  // NEW: Tool lifecycle updates
  }
}
```

### Usage Pattern

Full tool lifecycle:

```
Agent → Gateway: tool_use { id: "t1", name: "read_file", input: "..." }
Agent → Gateway: tool_state { id: "t1", state: AWAITING_APPROVAL }
Gateway → Agent: tool_approval { id: "t1", approved: true }
Agent → Gateway: tool_state { id: "t1", state: RUNNING }
Agent → Gateway: tool_state { id: "t1", state: COMPLETED }
Agent → Gateway: tool_result { id: "t1", output: "...", is_error: false }
```

Or for denied:

```
Agent → Gateway: tool_use { id: "t2", name: "delete_file", input: "..." }
Agent → Gateway: tool_state { id: "t2", state: AWAITING_APPROVAL }
Gateway → Agent: tool_approval { id: "t2", approved: false }
Agent → Gateway: tool_state { id: "t2", state: DENIED }
```

### Backward Compatibility

- `tool_state` updates are optional
- Old agents continue to work (just `tool_use` → `tool_result`)
- Clients that don't understand `tool_state` ignore them

---

## 3. Context Injection

### Problem

Can't push context to agents mid-turn. Use cases:
- Async tool results from Packs
- System alerts ("budget low", "shutting down in 5 min")
- User corrections while agent is working

### Proto Changes

```protobuf
// Priority for context injection
enum InjectionPriority {
  INJECTION_PRIORITY_UNSPECIFIED = 0;
  INJECTION_PRIORITY_IMMEDIATE = 1;   // Process before current turn completes
  INJECTION_PRIORITY_NORMAL = 2;      // Standard priority
  INJECTION_PRIORITY_DEFERRED = 3;    // Process after current work
}

// Context injection request (server → agent)
message InjectContext {
  string injection_id = 1;            // Unique ID for acknowledgment
  string content = 2;                 // Content to inject
  InjectionPriority priority = 3;     // When to process
  optional string source = 4;         // Origin (e.g., "pack:elevenlabs", "system")
}

// Context injection acknowledgment (agent → server)
message InjectionAck {
  string injection_id = 1;            // Matches InjectContext.injection_id
  bool accepted = 2;                  // Whether agent accepted the injection
  optional string reason = 3;         // Why rejected (if not accepted)
}

// Add to ServerMessage.payload oneof:
message ServerMessage {
  oneof payload {
    Welcome welcome = 1;
    SendMessage send_message = 2;
    Shutdown shutdown = 3;
    ToolApprovalResponse tool_approval = 4;
    RegistrationError registration_error = 5;
    InjectContext inject_context = 6;   // NEW: Push context to agent
    CancelRequest cancel_request = 7;   // NEW: See section 4
  }
}

// Add to AgentMessage.payload oneof:
message AgentMessage {
  oneof payload {
    RegisterAgent register = 1;
    MessageResponse response = 2;
    Heartbeat heartbeat = 3;
    InjectionAck injection_ack = 4;     // NEW: Acknowledge injection
  }
}
```

### Usage Pattern

System alert:
```
Gateway → Agent: inject_context {
  injection_id: "inj1",
  content: "SYSTEM: Budget is 80% consumed. 2000 tokens remaining.",
  priority: IMMEDIATE,
  source: "system"
}
Agent → Gateway: injection_ack { injection_id: "inj1", accepted: true }
```

Pack result arriving async:
```
Gateway → Agent: inject_context {
  injection_id: "inj2",
  content: "Tool result from pack:weather - Current temp: 72F, Clear skies",
  priority: NORMAL,
  source: "pack:weather"
}
```

### Agent Behavior

- **IMMEDIATE**: Agent should incorporate into current response if possible
- **NORMAL**: Agent queues for next natural point
- **DEFERRED**: Agent processes after current request completes

Agent may reject injection (e.g., context too large, invalid format).

---

## 4. Request Cancellation

### Problem

User hits "stop" in TUI - we rely on stream disconnect. No clean abort.

### Proto Changes

```protobuf
// Request cancellation (server → agent)
message CancelRequest {
  string request_id = 1;              // Request to cancel
  optional string reason = 2;         // Why cancelled (e.g., "user_requested")
}

// Cancellation acknowledgment - use existing MessageResponse with new event type
// Add to MessageResponse.event oneof:
message Cancelled {
  string reason = 1;                  // Echo back the reason
}

message MessageResponse {
  string request_id = 1;
  oneof event {
    // ... existing fields ...
    TokenUsage usage = 12;
    ToolStateUpdate tool_state = 13;
    Cancelled cancelled = 14;         // NEW: Request was cancelled
  }
}
```

### Usage Pattern

```
Gateway → Agent: send_message { request_id: "req1", content: "..." }
Agent → Gateway: text { request_id: "req1", text: "Let me..." }
Agent → Gateway: text { request_id: "req1", text: " think about..." }

[User hits STOP]

Gateway → Agent: cancel_request { request_id: "req1", reason: "user_requested" }
Agent → Gateway: cancelled { request_id: "req1", reason: "user_requested" }
```

### Agent Behavior

On receiving `cancel_request`:
1. Stop current LLM generation if possible
2. Clean up any in-flight tool executions
3. Send `cancelled` event (terminates the request, like `done` or `error`)

### Timeout Behavior

If agent doesn't respond to `cancel_request` within N seconds, gateway can:
- Consider request terminated
- Log timeout
- Optionally disconnect agent

---

## Complete Proto Diff

```protobuf
// ============================================
// NEW MESSAGES
// ============================================

// Token usage statistics from the LLM provider
message TokenUsage {
  int32 input_tokens = 1;
  int32 output_tokens = 2;
  int32 cache_read_tokens = 3;
  int32 cache_write_tokens = 4;
  int32 thinking_tokens = 5;
}

// Tool execution state
enum ToolState {
  TOOL_STATE_UNSPECIFIED = 0;
  TOOL_STATE_PENDING = 1;
  TOOL_STATE_AWAITING_APPROVAL = 2;
  TOOL_STATE_RUNNING = 3;
  TOOL_STATE_COMPLETED = 4;
  TOOL_STATE_FAILED = 5;
  TOOL_STATE_DENIED = 6;
  TOOL_STATE_TIMEOUT = 7;
  TOOL_STATE_CANCELLED = 8;
}

// Tool state transition notification
message ToolStateUpdate {
  string id = 1;
  ToolState state = 2;
  optional string detail = 3;
}

// Priority for context injection
enum InjectionPriority {
  INJECTION_PRIORITY_UNSPECIFIED = 0;
  INJECTION_PRIORITY_IMMEDIATE = 1;
  INJECTION_PRIORITY_NORMAL = 2;
  INJECTION_PRIORITY_DEFERRED = 3;
}

// Context injection request (server → agent)
message InjectContext {
  string injection_id = 1;
  string content = 2;
  InjectionPriority priority = 3;
  optional string source = 4;
}

// Context injection acknowledgment (agent → server)
message InjectionAck {
  string injection_id = 1;
  bool accepted = 2;
  optional string reason = 3;
}

// Request cancellation (server → agent)
message CancelRequest {
  string request_id = 1;
  optional string reason = 2;
}

// Cancellation acknowledgment (agent → server)
message Cancelled {
  string reason = 1;
}

// ============================================
// MODIFIED MESSAGES
// ============================================

// AgentMessage - add injection_ack
message AgentMessage {
  oneof payload {
    RegisterAgent register = 1;
    MessageResponse response = 2;
    Heartbeat heartbeat = 3;
    InjectionAck injection_ack = 4;     // NEW
  }
}

// MessageResponse - add usage, tool_state, cancelled
message MessageResponse {
  string request_id = 1;
  oneof event {
    string thinking = 2;
    string text = 3;
    ToolUse tool_use = 4;
    ToolResult tool_result = 5;
    Done done = 6;
    string error = 7;
    FileData file = 8;
    ToolApprovalRequest tool_approval_request = 9;
    SessionInit session_init = 10;
    SessionOrphaned session_orphaned = 11;
    TokenUsage usage = 12;              // NEW
    ToolStateUpdate tool_state = 13;    // NEW
    Cancelled cancelled = 14;           // NEW
  }
}

// ServerMessage - add inject_context, cancel_request
message ServerMessage {
  oneof payload {
    Welcome welcome = 1;
    SendMessage send_message = 2;
    Shutdown shutdown = 3;
    ToolApprovalResponse tool_approval = 4;
    RegistrationError registration_error = 5;
    InjectContext inject_context = 6;   // NEW
    CancelRequest cancel_request = 7;   // NEW
  }
}
```

---

## Implementation Order

### Phase 1: Token Usage (Low effort, high value)
1. Add `TokenUsage` message to proto
2. Update fold-agent to emit usage after LLM calls
3. Gateway stores in ledger, forwards to clients
4. TUI displays token counts

### Phase 2: Request Cancellation (Low effort, good UX)
1. Add `CancelRequest` and `Cancelled` to proto
2. TUI sends cancel on Ctrl+C / stop button
3. Gateway forwards to agent
4. Agent implements cancellation (best-effort)

### Phase 3: Structured Tool States (Medium effort, better UX)
1. Add `ToolState` enum and `ToolStateUpdate` message
2. Update fold-agent to emit state transitions
3. Gateway forwards to clients
4. TUI shows tool progress indicators

### Phase 4: Context Injection (Higher effort, enables async patterns)
1. Add injection messages to proto
2. Gateway implements injection routing
3. Agent implements injection handling
4. Build Pack→Gateway→Agent flow for async results

---

## Backward Compatibility

All changes are additive:
- New fields in oneof don't break existing parsers
- Old agents ignore new ServerMessage types
- Old clients ignore new MessageResponse events
- Gateway can detect agent capabilities via registration

Consider adding capability flags to `RegisterAgent`:
```protobuf
message RegisterAgent {
  string agent_id = 1;
  string name = 2;
  repeated string capabilities = 3;
  AgentMetadata metadata = 4;
  repeated string protocol_features = 5;  // NEW: ["token_usage", "tool_states", "injection", "cancellation"]
}
```

Gateway only sends new message types if agent advertises support.
