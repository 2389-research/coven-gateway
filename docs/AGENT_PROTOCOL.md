# Agent Protocol

This document describes the gRPC protocol for coven-agents connecting to coven-gateway.

## Overview

Agents connect to the gateway via a bidirectional gRPC stream. The connection lifecycle:

```
Agent                                    Gateway
  │                                         │
  │──────── TCP + HTTP/2 Connect ──────────>│
  │                                         │
  │<──────── Response Headers ──────────────│
  │                                         │
  │──────── RegisterAgent ─────────────────>│
  │                                         │
  │<──────── Welcome ───────────────────────│
  │                                         │
  │         ... ready for messages ...      │
  │                                         │
  │<──────── SendMessage ───────────────────│
  │                                         │
  │──────── MessageResponse (thinking) ────>│
  │──────── MessageResponse (text) ────────>│
  │──────── MessageResponse (tool_use) ────>│
  │──────── MessageResponse (tool_state) ──>│
  │──────── MessageResponse (tool_result) ─>│
  │──────── MessageResponse (usage) ───────>│
  │──────── MessageResponse (done) ────────>│
  │                                         │
  │──────── Heartbeat ─────────────────────>│
  │                                         │
  │<──────── InjectContext ─────────────────│
  │──────── InjectionAck ──────────────────>│
  │                                         │
  │<──────── CancelRequest ─────────────────│
  │──────── MessageResponse (cancelled) ───>│
  │                                         │
  │<──────── Shutdown ──────────────────────│
  │                                         │
  └─────────────────────────────────────────┘
```

## Connection

**Endpoint:** `grpc://<gateway-host>:50051`

**Service:** `coven.CovenControl`

**RPC:** `AgentStream` - bidirectional streaming

```protobuf
service CovenControl {
  rpc AgentStream(stream AgentMessage) returns (stream ServerMessage);
}
```

## Messages: Agent → Gateway

All messages from agent to gateway use the `AgentMessage` wrapper:

```protobuf
message AgentMessage {
  oneof payload {
    RegisterAgent register = 1;
    MessageResponse response = 2;
    Heartbeat heartbeat = 3;
    InjectionAck injection_ack = 4;    // Acknowledge context injection
    ExecutePackTool execute_pack_tool = 5; // Request pack tool execution
  }
}
```

### RegisterAgent

**Must be the first message sent after stream opens.**

```protobuf
message RegisterAgent {
  string agent_id = 1;              // Unique agent identifier (UUID recommended)
  string name = 2;                  // Human-readable name for logs/UI
  repeated string capabilities = 3; // List of capabilities (e.g., ["chat", "code"])
  AgentMetadata metadata = 4;       // Environment context
  repeated string protocol_features = 5; // Supported features
}

message AgentMetadata {
  string working_directory = 1;
  GitInfo git = 2;
  string hostname = 3;
  string os = 4;
  repeated string workspaces = 5;  // Workspace tags for filtering
  string backend = 6;              // Backend type: "mux", "cli", "acp", "direct"
}

message GitInfo {
  string branch = 1;
  string commit = 2;
  bool dirty = 3;
  string remote = 4;
  int32 ahead = 5;
  int32 behind = 6;
}
```

**Protocol Features:**

| Feature | Description |
|---------|-------------|
| `token_usage` | Agent will send TokenUsage events |
| `tool_states` | Agent will send ToolStateUpdate events |
| `injection` | Agent supports InjectContext messages |
| `cancellation` | Agent supports CancelRequest messages |

**Example:**
```json
{
  "register": {
    "agent_id": "550e8400-e29b-41d4-a716-446655440000",
    "name": "mux-agent-1",
    "capabilities": ["chat", "base", "notes"],
    "metadata": {
      "working_directory": "/home/user/project",
      "hostname": "dev-machine",
      "os": "linux",
      "workspaces": ["dev", "personal"],
      "backend": "mux"
    },
    "protocol_features": ["token_usage", "tool_states", "injection", "cancellation"]
  }
}
```

**Errors:**
- `INVALID_ARGUMENT`: Missing `agent_id`
- `ALREADY_EXISTS`: Agent with same ID already connected
- `RegistrationError`: Server rejects registration (e.g., not approved)

### MessageResponse

Sent in response to a `SendMessage` from the gateway. Multiple responses can be sent for a single request (streaming).

```protobuf
message MessageResponse {
  string request_id = 1;    // Must match SendMessage.request_id
  oneof event {
    string thinking = 2;            // Agent is thinking (optional status)
    string text = 3;                // Text chunk (stream incrementally)
    ToolUse tool_use = 4;           // Agent invoking a tool
    ToolResult tool_result = 5;     // Result of tool execution
    Done done = 6;                  // Final message, request complete
    string error = 7;               // Error occurred, request complete
    FileData file = 8;              // File output
    ToolApprovalRequest tool_approval_request = 9; // Needs human approval
    SessionInit session_init = 10;  // Backend session initialized
    SessionOrphaned session_orphaned = 11; // Backend session lost
    TokenUsage usage = 12;          // Token consumption update
    ToolStateUpdate tool_state = 13; // Tool lifecycle update
    Cancelled cancelled = 14;       // Request was cancelled
  }
}
```

**Event Types:**

| Event | Description | Terminates Request |
|-------|-------------|-------------------|
| `thinking` | Status indicator | No |
| `text` | Response text chunk | No |
| `tool_use` | Tool invocation starting | No |
| `tool_state` | Tool state transition | No |
| `tool_result` | Tool result | No |
| `tool_approval_request` | Needs human approval | No |
| `file` | File attachment | No |
| `session_init` | Backend session created | No |
| `session_orphaned` | Backend session lost | No |
| `usage` | Token usage statistics | No |
| `done` | Success completion | **Yes** |
| `error` | Error completion | **Yes** |
| `cancelled` | Cancelled by user/system | **Yes** |

**Important:** Every request must terminate with `done`, `error`, or `cancelled`.

### InjectionAck

Acknowledge receipt of a context injection.

```protobuf
message InjectionAck {
  string injection_id = 1;    // Matches InjectContext.injection_id
  bool accepted = 2;          // Whether agent accepted the injection
  optional string reason = 3; // Why rejected (if not accepted)
}
```

### ExecutePackTool

Request the gateway to execute a pack tool on behalf of the agent.

```protobuf
message ExecutePackTool {
  string request_id = 1;      // Unique ID for correlation
  string tool_name = 2;       // Name of the pack tool to execute
  string input_json = 3;      // Tool input as JSON
}
```

### Heartbeat

Optional keep-alive message. Send periodically if no other traffic.

```protobuf
message Heartbeat {
  int64 timestamp_ms = 1;  // Unix timestamp in milliseconds
}
```

## Messages: Gateway → Agent

All messages from gateway to agent use the `ServerMessage` wrapper:

```protobuf
message ServerMessage {
  oneof payload {
    Welcome welcome = 1;
    SendMessage send_message = 2;
    Shutdown shutdown = 3;
    ToolApprovalResponse tool_approval = 4;
    RegistrationError registration_error = 5;
    InjectContext inject_context = 6;
    CancelRequest cancel_request = 7;
    PackToolResult pack_tool_result = 8;
  }
}
```

### Welcome

Sent immediately after successful registration.

```protobuf
message Welcome {
  string server_id = 1;           // Gateway instance identifier
  string agent_id = 2;            // Confirmed agent ID (instance name)
  string instance_id = 3;         // Short code for binding commands
  string principal_id = 4;        // Principal UUID for reference
  repeated ToolDefinition available_tools = 5; // Pack tools available
  string mcp_token = 6;           // Token for MCP endpoint authentication
  string mcp_endpoint = 7;        // Base MCP endpoint URL
  map<string, string> secrets = 8; // Resolved env vars for this agent
}
```

**Tool Definition:**
```protobuf
message ToolDefinition {
  string name = 1;
  string description = 2;
  string input_schema_json = 3;   // MCP-compatible JSON Schema
  repeated string required_capabilities = 4;
  int32 timeout_seconds = 5;      // Default 30
}
```

### RegistrationError

Sent instead of Welcome when registration fails.

```protobuf
message RegistrationError {
  string reason = 1;           // Human-readable error message
  string suggested_id = 2;     // Optional: server-suggested alternative ID
}
```

### SendMessage

Request to process a user message.

```protobuf
message SendMessage {
  string request_id = 1;              // Unique ID, use in MessageResponse
  string thread_id = 2;               // Conversation thread ID
  string sender = 3;                  // Who sent the message
  string content = 4;                 // Message content
  repeated FileAttachment attachments = 5;
}

message FileAttachment {
  string filename = 1;
  string mime_type = 2;
  bytes data = 3;
}
```

**Required Response:** Agent must send one or more `MessageResponse` messages with matching `request_id`, ending with `done`, `error`, or `cancelled`.

### ToolApprovalResponse

Response to a tool approval request from the agent.

```protobuf
message ToolApprovalResponse {
  string id = 1;           // Correlates with ToolApprovalRequest.id
  bool approved = 2;       // True = execute, False = skip
  bool approve_all = 3;    // If true, auto-approve remaining tools this request
}
```

### InjectContext

Push context to the agent mid-turn (requires `injection` protocol feature).

```protobuf
message InjectContext {
  string injection_id = 1;        // Unique ID for acknowledgment
  string content = 2;             // Content to inject
  InjectionPriority priority = 3; // When to process
  optional string source = 4;     // Origin (e.g., "pack:elevenlabs", "system")
}

enum InjectionPriority {
  INJECTION_PRIORITY_UNSPECIFIED = 0;
  INJECTION_PRIORITY_IMMEDIATE = 1;   // Process before current turn completes
  INJECTION_PRIORITY_NORMAL = 2;      // Standard priority
  INJECTION_PRIORITY_DEFERRED = 3;    // Process after current work
}
```

### CancelRequest

Cancel an in-flight request (requires `cancellation` protocol feature).

```protobuf
message CancelRequest {
  string request_id = 1;        // Request to cancel
  optional string reason = 2;   // Why cancelled (e.g., "user_requested")
}
```

### PackToolResult

Result of a pack tool execution request.

```protobuf
message PackToolResult {
  string request_id = 1;        // Correlates with ExecutePackTool.request_id
  oneof result {
    string output_json = 2;     // Success: tool output as JSON
    string error = 3;           // Failure: error message
  }
}
```

### Shutdown

Graceful shutdown request. Agent should complete current work and disconnect.

```protobuf
message Shutdown {
  string reason = 1;  // Human-readable reason
}
```

## Supporting Types

### Tool Lifecycle

```protobuf
message ToolUse {
  string id = 1;          // Unique tool invocation ID
  string name = 2;        // Tool name
  string input_json = 3;  // Tool input as JSON string
}

message ToolResult {
  string id = 1;          // Matches ToolUse.id
  string output = 2;      // Tool output
  bool is_error = 3;      // True if tool failed
}

message ToolApprovalRequest {
  string id = 1;           // Correlates with ToolUse.id
  string name = 2;         // Tool name
  string input_json = 3;   // Tool input for display
}

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

message ToolStateUpdate {
  string id = 1;            // Tool invocation ID (matches ToolUse.id)
  ToolState state = 2;      // New state
  optional string detail = 3; // Optional detail (error message, etc.)
}
```

### Token Usage

```protobuf
message TokenUsage {
  int32 input_tokens = 1;       // Tokens in the prompt
  int32 output_tokens = 2;      // Tokens generated
  int32 cache_read_tokens = 3;  // Tokens read from cache (Anthropic)
  int32 cache_write_tokens = 4; // Tokens written to cache (Anthropic)
  int32 thinking_tokens = 5;    // Extended thinking tokens (Claude)
}
```

### Session Management

```protobuf
message SessionInit {
  string session_id = 1;  // Backend session ID assigned/confirmed
}

message SessionOrphaned {
  string reason = 1;      // Why session was lost
}

message Cancelled {
  string reason = 1;      // Echo back the cancellation reason
}
```

### File Data

```protobuf
message Done {
  string full_response = 1;  // Complete response text (optional)
}

message FileData {
  string filename = 1;
  string mime_type = 2;
  bytes data = 3;
}
```

## Implementation Notes

### Connection Handling

1. **TCP Connection**: Connect to gateway's gRPC port (default 50051)
2. **Stream Opening**: Call `AgentStream` RPC
3. **Wait for Headers**: The gateway sends HTTP/2 response headers immediately
4. **Register**: Send `RegisterAgent` as first message
5. **Handle Response**: Receive either `Welcome` (success) or `RegistrationError` (failure)
6. **Ready**: Agent is now registered and will receive `SendMessage` requests

### Error Recovery

- If stream disconnects, reconnect and re-register
- Use the same `agent_id` to maintain identity
- Gateway deregisters agents on disconnect

### Concurrency

- Process one `SendMessage` at a time per stream
- Stream all `MessageResponse` events for a request before starting the next
- Heartbeats can be sent anytime
- Handle `CancelRequest` to abort in-flight work

### Protocol Feature Negotiation

Agents declare supported features in `protocol_features` during registration:

1. Include features the agent supports (e.g., `["token_usage", "tool_states"]`)
2. Gateway tracks which features each agent supports
3. Gateway only sends messages for supported features (e.g., won't send `InjectContext` unless `injection` is declared)

### Example: Rust with Tonic

```rust
use tonic::transport::Channel;

let channel = Channel::from_static("http://localhost:50051")
    .connect()
    .await?;

let mut client = CovenControlClient::new(channel);

let (tx, rx) = mpsc::channel(100);
let outbound = ReceiverStream::new(rx);

let response = client.agent_stream(outbound).await?;
let mut inbound = response.into_inner();

// Send registration
tx.send(AgentMessage {
    payload: Some(Payload::Register(RegisterAgent {
        agent_id: uuid::Uuid::new_v4().to_string(),
        name: "my-agent".to_string(),
        capabilities: vec!["chat".to_string(), "base".to_string()],
        metadata: Some(AgentMetadata {
            working_directory: "/home/user/project".to_string(),
            hostname: "dev-machine".to_string(),
            os: "linux".to_string(),
            workspaces: vec!["dev".to_string()],
            backend: "mux".to_string(),
            ..Default::default()
        }),
        protocol_features: vec![
            "token_usage".to_string(),
            "tool_states".to_string(),
            "cancellation".to_string(),
        ],
    })),
}).await?;

// Wait for welcome or error
if let Some(msg) = inbound.next().await {
    match msg?.payload {
        Some(Payload::Welcome(w)) => {
            println!("Connected: {} (instance: {})", w.server_id, w.instance_id);
            println!("Available tools: {:?}", w.available_tools);
        }
        Some(Payload::RegistrationError(e)) => {
            panic!("Registration failed: {}", e.reason);
        }
        _ => panic!("Unexpected message"),
    }
}

// Process messages
while let Some(msg) = inbound.next().await {
    match msg?.payload {
        Some(Payload::SendMessage(req)) => {
            // Process message, send responses...
        }
        Some(Payload::CancelRequest(cancel)) => {
            // Abort current work if request_id matches
        }
        Some(Payload::InjectContext(inject)) => {
            // Handle context injection
            tx.send(AgentMessage {
                payload: Some(Payload::InjectionAck(InjectionAck {
                    injection_id: inject.injection_id,
                    accepted: true,
                    reason: None,
                })),
            }).await?;
        }
        Some(Payload::Shutdown(s)) => {
            println!("Shutdown requested: {}", s.reason);
            break;
        }
        _ => {}
    }
}
```

### Example: Go with grpc-go

```go
conn, _ := grpc.Dial("localhost:50051", grpc.WithInsecure())
client := pb.NewCovenControlClient(conn)

stream, _ := client.AgentStream(context.Background())

// Send registration
stream.Send(&pb.AgentMessage{
    Payload: &pb.AgentMessage_Register{
        Register: &pb.RegisterAgent{
            AgentId:      uuid.New().String(),
            Name:         "my-agent",
            Capabilities: []string{"chat", "base"},
            Metadata: &pb.AgentMetadata{
                WorkingDirectory: "/home/user/project",
                Hostname:         "dev-machine",
                Os:               "linux",
                Workspaces:       []string{"dev"},
                Backend:          "mux",
            },
            ProtocolFeatures: []string{"token_usage", "tool_states", "cancellation"},
        },
    },
})

// Wait for welcome or error
msg, _ := stream.Recv()
switch p := msg.Payload.(type) {
case *pb.ServerMessage_Welcome:
    fmt.Printf("Connected to %s (instance: %s)\n", p.Welcome.ServerId, p.Welcome.InstanceId)
case *pb.ServerMessage_RegistrationError:
    log.Fatalf("Registration failed: %s", p.RegistrationError.Reason)
}

// Process messages
for {
    msg, err := stream.Recv()
    if err == io.EOF {
        break
    }
    switch p := msg.Payload.(type) {
    case *pb.ServerMessage_SendMessage:
        // Handle message...
    case *pb.ServerMessage_CancelRequest:
        // Abort if matching request
    case *pb.ServerMessage_InjectContext:
        // Handle injection, send ack
    case *pb.ServerMessage_Shutdown:
        fmt.Printf("Shutdown: %s\n", p.Shutdown.Reason)
        return
    }
}
```
