# Client Protocol

This document describes the HTTP API for clients (TUIs, web apps, bots) to interact with coven-gateway.

## Overview

Clients communicate with the gateway via HTTP. Messages are sent via POST and responses stream back via Server-Sent Events (SSE).

```
Client                                   Gateway                    Agent
  │                                         │                         │
  │──── GET /api/agents ───────────────────>│                         │
  │<─── JSON: [{id, name, caps}] ───────────│                         │
  │                                         │                         │
  │──── POST /api/send ────────────────────>│                         │
  │     {content: "hello"}                  │──── SendMessage ───────>│
  │                                         │                         │
  │<─── SSE: event: started ────────────────│                         │
  │<─── SSE: event: thinking ───────────────│<─── thinking ───────────│
  │<─── SSE: event: text ───────────────────│<─── text ───────────────│
  │<─── SSE: event: tool_use ───────────────│<─── tool_use ───────────│
  │<─── SSE: event: tool_state ─────────────│<─── tool_state ─────────│
  │<─── SSE: event: tool_result ────────────│<─── tool_result ────────│
  │<─── SSE: event: usage ──────────────────│<─── usage ──────────────│
  │<─── SSE: event: done ───────────────────│<─── done ───────────────│
  │                                         │                         │
  └─────────────────────────────────────────┴─────────────────────────┘
```

## Base URL

Default: `http://localhost:8080`

## Endpoints

### GET /health

Liveness check. Returns 200 if gateway is running.

**Response:**
```
HTTP/1.1 200 OK
Content-Type: text/plain

OK
```

### GET /health/ready

Readiness check. Returns 200 if at least one agent is connected.

**Response (ready):**
```
HTTP/1.1 200 OK
Content-Type: text/plain

ready (2 agents)
```

**Response (not ready):**
```
HTTP/1.1 503 Service Unavailable
Content-Type: text/plain

no agents connected
```

### GET /api/agents

List all connected agents.

**Query Parameters:**
- `workspace` (optional): Filter by workspace tag

**Response:**
```json
[
  {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "instance_id": "abc123",
    "name": "mux-agent-1",
    "capabilities": ["chat", "base"],
    "workspaces": ["dev", "personal"],
    "working_dir": "/home/user/project",
    "backend": "mux"
  }
]
```

**Status Codes:**
- `200`: Success (may be empty array)
- `405`: Method not allowed (not GET)

### POST /api/send

Send a message to an agent and receive streaming response via SSE.

**Request:**
```http
POST /api/send HTTP/1.1
Content-Type: application/json

{
  "content": "Hello, what can you help me with?",
  "sender": "user@example.com",
  "thread_id": "optional-thread-id",
  "agent_id": "optional-specific-agent-id"
}
```

**Request Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `content` | string | **Yes** | Message content |
| `sender` | string | **Yes** | Sender identifier |
| `thread_id` | string | No | Conversation thread ID |
| `agent_id` | string | No | Target specific agent directly |
| `frontend` | string | No | Frontend name (e.g., "slack", "matrix") for binding lookup |
| `channel_id` | string | No | Channel ID within frontend for binding lookup |

**Note:** You can specify agent routing in two ways:
1. **Direct**: Set `agent_id` to route directly to a specific agent
2. **Binding Lookup**: Set `frontend` and `channel_id` to look up the bound agent for that channel

**Response (SSE Stream):**
```http
HTTP/1.1 200 OK
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive

event: started
data: {"thread_id":"550e8400-e29b-41d4-a716-446655440000"}

event: thinking
data: {"text":"thinking..."}

event: text
data: {"text":"Hello! I'm an AI assistant. "}

event: text
data: {"text":"I can help you with various tasks."}

event: tool_use
data: {"id":"tool_1","name":"list_files","input_json":"{\"path\":\"..\"}"}

event: tool_state
data: {"id":"tool_1","state":"running"}

event: tool_result
data: {"id":"tool_1","output":"file1.txt\nfile2.txt","is_error":false}

event: tool_state
data: {"id":"tool_1","state":"completed"}

event: usage
data: {"input_tokens":150,"output_tokens":75,"cache_read_tokens":0,"cache_write_tokens":50,"thinking_tokens":25}

event: done
data: {"full_response":"Hello! I'm an AI assistant..."}
```

**Status Codes:**
- `200`: Success (SSE stream)
- `400`: Bad request (invalid JSON, missing content/sender)
- `404`: Agent not found (when `agent_id` specified but doesn't exist)
- `405`: Method not allowed (not POST)
- `503`: No agents available

**Error Response (non-SSE):**
```json
{
  "error": "no agents available"
}
```

### POST /api/agents/{id}/send

Send a message directly to a specific agent by ID (alternative to POST /api/send).

**Request:**
```http
POST /api/agents/550e8400-e29b-41d4-a716-446655440000/send HTTP/1.1
Content-Type: application/json

{
  "message": "Hello!"
}
```

**Response:** Same SSE stream as POST /api/send.

### GET /api/agents/{id}/history

Get conversation history for a specific agent.

**Query Parameters:**
- `limit` (optional): Maximum events to return (default: 50, max: 500)
- `cursor` (optional): Pagination cursor from previous response

**Response:**
```json
{
  "agent_id": "550e8400-e29b-41d4-a716-446655440000",
  "events": [
    {
      "id": "event-uuid-1",
      "direction": "inbound_to_agent",
      "author": "user@example.com",
      "type": "message",
      "timestamp": "2024-01-15T10:30:00Z",
      "thread_id": "thread-uuid",
      "text": "Hello!"
    }
  ],
  "count": 1,
  "has_more": false,
  "next_cursor": "",
  "usage": {
    "total_input": 1500,
    "total_output": 750,
    "total_cache_read": 100,
    "total_cache_write": 200,
    "total_thinking": 50,
    "total_tokens": 2600,
    "request_count": 10
  }
}
```

## SSE Event Types

All SSE events have the format:
```
event: <event_type>
data: <json_payload>

```

### started

Stream started, thread ID assigned.

```
event: started
data: {"thread_id":"550e8400-e29b-41d4-a716-446655440000"}
```

### thinking

Agent is processing (status indicator).

```
event: thinking
data: {"text":"thinking..."}
```

### text

Text chunk from the agent's response. May arrive in multiple chunks.

```
event: text
data: {"text":"Here is part of the response..."}
```

### tool_use

Agent is invoking a tool.

```
event: tool_use
data: {"id":"tool_123","name":"read_file","input_json":"{\"path\":\"config.yaml\"}"}
```

**Fields:**
- `id`: Unique tool invocation ID
- `name`: Tool name
- `input_json`: Tool input as JSON string

### tool_state

Tool state transition.

```
event: tool_state
data: {"id":"tool_123","state":"running"}
```

**States:**
- `pending`: Tool identified, not yet started
- `awaiting_approval`: Waiting for human approval
- `running`: Actively executing
- `completed`: Finished successfully
- `failed`: Execution error
- `denied`: Approval denied
- `timeout`: Execution timed out
- `canceled`: Canceled by user/system

### tool_result

Result of a tool invocation.

```
event: tool_result
data: {"id":"tool_123","output":"file contents here...","is_error":false}
```

**Fields:**
- `id`: Matches `tool_use.id`
- `output`: Tool output
- `is_error`: Whether the tool failed

### tool_approval

Tool requires human approval before execution.

```
event: tool_approval
data: {"id":"tool_123","name":"run_command","input_json":"{\"command\":\"rm -rf /tmp/test\"}","request_id":"req_456"}
```

**Fields:**
- `id`: Tool invocation ID
- `name`: Tool name
- `input_json`: Tool input as JSON string
- `request_id`: Request ID for correlation

Use POST /api/tools/approve to approve or deny.

### file

File output from the agent.

```
event: file
data: {"filename":"output.png","mime_type":"image/png"}
```

**Note:** File data is not included in SSE (too large). Future versions may provide a download URL.

### session_init

Backend session initialized (for stateful backends).

```
event: session_init
data: {"session_id":"backend-session-id"}
```

### session_orphaned

Backend session was lost (need to restart).

```
event: session_orphaned
data: {"reason":"session expired"}
```

### usage

Token usage statistics from the LLM provider.

```
event: usage
data: {"input_tokens":150,"output_tokens":75,"cache_read_tokens":0,"cache_write_tokens":50,"thinking_tokens":25}
```

### done

Request completed successfully. **Terminates the stream.**

```
event: done
data: {"full_response":"Complete response text here..."}
```

### error

Request failed. **Terminates the stream.**

```
event: error
data: {"error":"Agent disconnected during processing"}
```

### canceled

Request was canceled. **Terminates the stream.**

```
event: canceled
data: {"reason":"user_requested"}
```

## Tool Approval API

### POST /api/tools/approve

Approve or deny a pending tool execution.

**Request:**
```json
{
  "agent_id": "550e8400-e29b-41d4-a716-446655440000",
  "tool_id": "tool_123",
  "approved": true,
  "approve_all": false
}
```

**Request Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `agent_id` | string | **Yes** | Agent requesting approval |
| `tool_id` | string | **Yes** | Tool invocation ID |
| `approved` | boolean | **Yes** | True = execute, False = skip |
| `approve_all` | boolean | No | Auto-approve remaining tools this request |

**Response:**
```json
{
  "success": true
}
```

## User Question API

### POST /api/questions/answer

Respond to a user question from the ask_user tool.

**Request:**
```json
{
  "agent_id": "550e8400-e29b-41d4-a716-446655440000",
  "question_id": "question_123",
  "selected": ["option1"],
  "custom_text": ""
}
```

**Request Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `agent_id` | string | **Yes** | Agent asking the question |
| `question_id` | string | **Yes** | Question ID from SSE event |
| `selected` | []string | **Yes** | Selected option label(s) |
| `custom_text` | string | No | Custom "Other" response text |

**Response:**
```json
{
  "success": true
}
```

## Channel Bindings API

Channel bindings associate frontend channels with specific agents for sticky routing.

### GET /api/bindings

List all channel bindings.

**Response:**
```json
{
  "bindings": [
    {
      "frontend": "slack",
      "channel_id": "C0123456789",
      "agent_id": "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
      "agent_name": "mux-agent-1",
      "agent_online": true,
      "working_dir": "/home/user/project",
      "created_at": "2024-01-15T10:30:00Z"
    }
  ]
}
```

### GET /api/bindings?frontend=X&channel_id=Y

Get a single binding.

**Response:**
```json
{
  "binding_id": "550e8400-e29b-41d4-a716-446655440000",
  "agent_name": "mux-agent-1",
  "working_dir": "/home/user/project",
  "online": true
}
```

### POST /api/bindings

Create a new channel binding. Uses `instance_id` (short code) to identify the agent.

**Request:**
```json
{
  "frontend": "slack",
  "channel_id": "C0123456789",
  "instance_id": "abc123"
}
```

**Response:**
```json
{
  "binding_id": "550e8400-e29b-41d4-a716-446655440000",
  "agent_name": "mux-agent-1",
  "working_dir": "/home/user/project",
  "rebound_from": null
}
```

**Status Codes:**
- `200`: Created successfully (or rebound existing)
- `400`: Bad request (missing fields, invalid JSON)
- `404`: Agent not found
- `405`: Method not allowed

### DELETE /api/bindings

Delete a channel binding.

**Request:**
```
DELETE /api/bindings?frontend=slack&channel_id=C0123456789
```

**Response:**
```
HTTP/1.1 204 No Content
```

## Thread History API

### GET /api/threads/{id}/messages

Get message history for a specific thread.

**Query Parameters:**
- `limit` (optional): Maximum messages to return (default: 100)

**Response:**
```json
{
  "thread_id": "550e8400-e29b-41d4-a716-446655440000",
  "messages": [
    {
      "id": "msg-uuid-1",
      "thread_id": "550e8400-e29b-41d4-a716-446655440000",
      "sender": "user@example.com",
      "content": "Hello!",
      "type": "message",
      "created_at": "2024-01-15T10:30:00Z"
    },
    {
      "id": "msg-uuid-2",
      "thread_id": "550e8400-e29b-41d4-a716-446655440000",
      "sender": "agent",
      "content": "Hello! How can I help you?",
      "type": "message",
      "created_at": "2024-01-15T10:30:05Z"
    }
  ]
}
```

### GET /api/threads/{id}/usage

Get token usage statistics for a specific thread.

**Response:**
```json
{
  "thread_id": "550e8400-e29b-41d4-a716-446655440000",
  "usage": [
    {
      "id": "usage_123",
      "message_id": "msg_456",
      "request_id": "req_789",
      "agent_id": "agent_001",
      "input_tokens": 150,
      "output_tokens": 75,
      "cache_read_tokens": 10,
      "cache_write_tokens": 20,
      "thinking_tokens": 5,
      "created_at": "2024-01-15T10:30:00Z"
    }
  ]
}
```

**Usage Record Fields:**
- `id`: Usage record ID
- `message_id`: Associated message ID (if available)
- `request_id`: Request ID
- `agent_id`: Agent that processed the request
- `input_tokens`, `output_tokens`: Token counts
- `cache_read_tokens`, `cache_write_tokens`: Cache token counts
- `thinking_tokens`: Thinking/reasoning tokens
- `created_at`: Timestamp

## Usage Statistics API

### GET /api/stats/usage

Get aggregated token usage statistics.

**Query Parameters:**
- `agent_id` (optional): Filter by agent
- `thread_id` (optional): Filter by thread
- `since` (optional): ISO-8601 start time
- `until` (optional): ISO-8601 end time

**Response:**
```json
{
  "total_input": 15000,
  "total_output": 7500,
  "total_cache_read": 1000,
  "total_cache_write": 2000,
  "total_thinking": 500,
  "total_tokens": 26000,
  "request_count": 100
}
```

## Implementation Examples

### curl

```bash
# List agents
curl http://localhost:8080/api/agents

# Send message (streaming)
curl -N -X POST http://localhost:8080/api/send \
  -H "Content-Type: application/json" \
  -d '{"content": "Hello!", "sender": "test"}'

# Approve a tool
curl -X POST http://localhost:8080/api/tools/approve \
  -H "Content-Type: application/json" \
  -d '{"agent_id":"<agent-id>","tool_id":"<tool-id>","approved":true}'
```

### JavaScript (Browser)

```javascript
// List agents
const agents = await fetch('/api/agents').then(r => r.json());
console.log('Available agents:', agents);

// Send message with SSE
const response = await fetch('/api/send', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    content: 'Hello!',
    sender: 'web-user',
    agent_id: agents[0]?.id
  })
});

const reader = response.body.getReader();
const decoder = new TextDecoder();
let buffer = '';

while (true) {
  const { done, value } = await reader.read();
  if (done) break;

  buffer += decoder.decode(value, { stream: true });

  // Parse SSE events
  const lines = buffer.split('\n');
  buffer = lines.pop(); // Keep incomplete line

  let eventType = null;
  for (const line of lines) {
    if (line.startsWith('event: ')) {
      eventType = line.slice(7);
    } else if (line.startsWith('data: ') && eventType) {
      const data = JSON.parse(line.slice(6));
      handleEvent(eventType, data);
      eventType = null;
    }
  }
}

function handleEvent(event, data) {
  switch (event) {
    case 'started':
      console.log('Thread:', data.thread_id);
      break;
    case 'text':
      process.stdout.write(data.text);
      break;
    case 'tool_use':
      console.log(`\n[Tool: ${data.name}]`);
      break;
    case 'tool_approval':
      // Show approval UI, then POST /api/tools/approve
      break;
    case 'usage':
      console.log(`\nTokens: ${data.input_tokens} in, ${data.output_tokens} out`);
      break;
    case 'done':
      console.log('\n--- Done ---');
      break;
    case 'error':
      console.error('Error:', data.error);
      break;
  }
}
```

### Python

```python
import requests
import json

# List agents
agents = requests.get('http://localhost:8080/api/agents').json()
print(f"Agents: {agents}")

# Send message with SSE
response = requests.post(
    'http://localhost:8080/api/send',
    json={'content': 'Hello!', 'sender': 'python'},
    stream=True
)

event_type = None
for line in response.iter_lines(decode_unicode=True):
    if line.startswith('event: '):
        event_type = line[7:]
    elif line.startswith('data: ') and event_type:
        data = json.loads(line[6:])

        if event_type == 'started':
            print(f"Thread: {data['thread_id']}")
        elif event_type == 'text':
            print(data['text'], end='', flush=True)
        elif event_type == 'tool_use':
            print(f"\n[Tool: {data['name']}]")
        elif event_type == 'usage':
            print(f"\nTokens: {data['input_tokens']} in, {data['output_tokens']} out")
        elif event_type == 'done':
            print("\n--- Done ---")
        elif event_type == 'error':
            print(f"Error: {data['error']}")

        event_type = None
```

## Notes

### Threading

- Each POST to `/api/send` is independent
- Multiple concurrent requests are supported
- Use `thread_id` to maintain conversation context across requests

### Agent Selection

- If `agent_id` is specified, the message goes directly to that agent
- If `frontend` and `channel_id` are specified, the gateway looks up the channel binding
- If no agent can be determined, an error is returned
- Use `GET /api/agents` to discover available agents
- Use channel bindings for sticky routing

### Connection Handling

- SSE connections stay open until `done`, `error`, or `canceled` event
- Client should handle connection drops gracefully
- Consider implementing retry logic for network failures

### Content Types

- Request: `application/json`
- SSE Response: `text/event-stream`
- Error Response: `application/json`

## gRPC Client Service

For more advanced client integrations, a gRPC `ClientService` is also available. See the proto definitions for:

- `GetEvents`: Get conversation history with pagination
- `GetMe`: Get authenticated principal info
- `SendMessage`: Send message with idempotency support
- `StreamEvents`: Real-time streaming of all events
- `ListAgents`: List available agents
- `RegisterAgent`: Self-register an agent
- `RegisterClient`: Self-register a client
- `ApproveTool`: Approve/deny tool execution
- `AnswerQuestion`: Answer user questions from ask_user tool

See `proto/coven-proto/coven.proto` for full message definitions.
