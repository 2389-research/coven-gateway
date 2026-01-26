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
  │──────── MessageResponse (tool_result) ─>│
  │──────── MessageResponse (text) ────────>│
  │──────── MessageResponse (done) ────────>│
  │                                         │
  │──────── Heartbeat ─────────────────────>│
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
}
```

**Example:**
```json
{
  "register": {
    "agent_id": "550e8400-e29b-41d4-a716-446655440000",
    "name": "mux-agent-1",
    "capabilities": ["chat"]
  }
}
```

**Errors:**
- `INVALID_ARGUMENT`: Missing `agent_id`
- `ALREADY_EXISTS`: Agent with same ID already connected

### MessageResponse

Sent in response to a `SendMessage` from the gateway. Multiple responses can be sent for a single request (streaming).

```protobuf
message MessageResponse {
  string request_id = 1;    // Must match SendMessage.request_id
  oneof event {
    string thinking = 2;    // Agent is thinking (optional status)
    string text = 3;        // Text chunk (stream incrementally)
    ToolUse tool_use = 4;   // Agent invoking a tool
    ToolResult tool_result = 5; // Result of tool execution
    Done done = 6;          // Final message, request complete
    string error = 7;       // Error occurred, request complete
    FileData file = 8;      // File output
  }
}
```

**Event Types:**

| Event | Description | Terminates Request |
|-------|-------------|-------------------|
| `thinking` | Status indicator | No |
| `text` | Response text chunk | No |
| `tool_use` | Tool invocation | No |
| `tool_result` | Tool result | No |
| `file` | File attachment | No |
| `done` | Success completion | **Yes** |
| `error` | Error completion | **Yes** |

**Important:** Every request must terminate with either `done` or `error`.

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
  }
}
```

### Welcome

Sent immediately after successful registration.

```protobuf
message Welcome {
  string server_id = 1;   // Gateway instance identifier
  string agent_id = 2;    // Confirmed agent ID
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

**Required Response:** Agent must send one or more `MessageResponse` messages with matching `request_id`, ending with `done` or `error`.

### Shutdown

Graceful shutdown request. Agent should complete current work and disconnect.

```protobuf
message Shutdown {
  string reason = 1;  // Human-readable reason
}
```

## Supporting Types

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
5. **Await Welcome**: Gateway responds with `Welcome` on success
6. **Ready**: Agent is now registered and will receive `SendMessage` requests

### Error Recovery

- If stream disconnects, reconnect and re-register
- Use the same `agent_id` to maintain identity
- Gateway deregisters agents on disconnect

### Concurrency

- Process one `SendMessage` at a time per stream
- Stream all `MessageResponse` events for a request before starting the next
- Heartbeats can be sent anytime

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
        capabilities: vec!["chat".to_string()],
    })),
}).await?;

// Wait for welcome
if let Some(msg) = inbound.next().await {
    match msg?.payload {
        Some(Payload::Welcome(w)) => println!("Connected: {}", w.server_id),
        _ => panic!("Expected Welcome"),
    }
}

// Process messages
while let Some(msg) = inbound.next().await {
    // Handle SendMessage, respond with MessageResponse...
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
            Capabilities: []string{"chat"},
        },
    },
})

// Wait for welcome
msg, _ := stream.Recv()
welcome := msg.GetWelcome()
fmt.Printf("Connected to %s\n", welcome.ServerId)

// Process messages
for {
    msg, err := stream.Recv()
    if err == io.EOF {
        break
    }
    // Handle SendMessage...
}
```
