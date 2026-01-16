# Client Protocol

This document describes the HTTP API for clients (TUIs, web apps, bots) to interact with fold-gateway.

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
  │<─── SSE: event: thinking ───────────────│<─── thinking ───────────│
  │<─── SSE: event: text ───────────────────│<─── text ───────────────│
  │<─── SSE: event: tool_use ───────────────│<─── tool_use ───────────│
  │<─── SSE: event: tool_result ────────────│<─── tool_result ────────│
  │<─── SSE: event: text ───────────────────│<─── text ───────────────│
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

**Response:**
```json
[
  {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "name": "mux-agent-1",
    "capabilities": ["chat"]
  },
  {
    "id": "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
    "name": "code-agent",
    "capabilities": ["chat", "code"]
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
| `sender` | string | No | Sender identifier |
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

event: thinking
data: {"text":"thinking..."}

event: text
data: {"text":"Hello! I'm an AI assistant. "}

event: text
data: {"text":"I can help you with various tasks including:"}

event: tool_use
data: {"id":"tool_1","name":"list_files","input_json":"{\"path\":\"..\"}"}

event: tool_result
data: {"id":"tool_1","output":"file1.txt\nfile2.txt","is_error":false}

event: text
data: {"text":"\n\nI found some files in your directory."}

event: done
data: {"full_response":"Hello! I'm an AI assistant..."}
```

**Status Codes:**
- `200`: Success (SSE stream)
- `400`: Bad request (invalid JSON, missing content)
- `404`: Agent not found (when `agent_id` specified but doesn't exist)
- `405`: Method not allowed (not POST)
- `503`: No agents available

**Error Response (non-SSE):**
```json
{
  "error": "no agents available"
}
```

## SSE Event Types

All SSE events have the format:
```
event: <event_type>
data: <json_payload>

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

### file

File output from the agent.

```
event: file
data: {"filename":"output.png","mime_type":"image/png"}
```

**Note:** File data is not included in SSE (too large). Future versions may provide a download URL.

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

## Implementation Examples

### curl

```bash
# List agents
curl http://localhost:8080/api/agents

# Send message (streaming)
curl -N -X POST http://localhost:8080/api/send \
  -H "Content-Type: application/json" \
  -d '{"content": "Hello!", "sender": "test"}'
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
    agent_id: agents[0]?.id  // Optional: target specific agent
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
    case 'text':
      process.stdout.write(data.text);
      break;
    case 'tool_use':
      console.log(`\n[Tool: ${data.name}]`);
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

### Go

```go
import (
    "bufio"
    "bytes"
    "encoding/json"
    "net/http"
    "strings"
)

// Send message
body, _ := json.Marshal(map[string]string{
    "content": "Hello!",
    "sender":  "go-client",
})

resp, _ := http.Post(
    "http://localhost:8080/api/send",
    "application/json",
    bytes.NewReader(body),
)
defer resp.Body.Close()

// Parse SSE
scanner := bufio.NewScanner(resp.Body)
var eventType string

for scanner.Scan() {
    line := scanner.Text()

    if strings.HasPrefix(line, "event: ") {
        eventType = strings.TrimPrefix(line, "event: ")
    } else if strings.HasPrefix(line, "data: ") && eventType != "" {
        data := strings.TrimPrefix(line, "data: ")

        switch eventType {
        case "text":
            var payload struct{ Text string `json:"text"` }
            json.Unmarshal([]byte(data), &payload)
            fmt.Print(payload.Text)
        case "done":
            fmt.Println("\n--- Done ---")
        case "error":
            var payload struct{ Error string `json:"error"` }
            json.Unmarshal([]byte(data), &payload)
            fmt.Println("Error:", payload.Error)
        }
        eventType = ""
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

        if event_type == 'text':
            print(data['text'], end='', flush=True)
        elif event_type == 'tool_use':
            print(f"\n[Tool: {data['name']}]")
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
- If `frontend` and `channel_id` are specified, the gateway looks up the channel binding to find the assigned agent
- If no agent can be determined, an error is returned
- Use `GET /api/agents` to discover available agents
- Use channel bindings for sticky routing (same channel always routes to same agent)

### Connection Handling

- SSE connections stay open until `done` or `error` event
- Client should handle connection drops gracefully
- Consider implementing retry logic for network failures

### Content Types

- Request: `application/json`
- SSE Response: `text/event-stream`
- Error Response: `application/json`

## Channel Bindings API

Channel bindings associate frontend channels with specific agents for sticky routing.

### GET /api/bindings

List all channel bindings.

**Response:**
```json
[
  {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "frontend": "slack",
    "channel_id": "C0123456789",
    "agent_id": "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
    "created_at": "2024-01-15T10:30:00Z"
  }
]
```

**Status Codes:**
- `200`: Success (may be empty array)
- `405`: Method not allowed

### POST /api/bindings

Create a new channel binding.

**Request:**
```json
{
  "frontend": "slack",
  "channel_id": "C0123456789",
  "agent_id": "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
}
```

**Request Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `frontend` | string | **Yes** | Frontend name (e.g., "slack", "matrix") |
| `channel_id` | string | **Yes** | Channel ID within the frontend |
| `agent_id` | string | **Yes** | UUID of the agent to bind to |

**Response:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "frontend": "slack",
  "channel_id": "C0123456789",
  "agent_id": "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
  "created_at": "2024-01-15T10:30:00Z"
}
```

**Status Codes:**
- `201`: Created successfully
- `400`: Bad request (missing fields, invalid JSON)
- `409`: Conflict (binding already exists for this frontend/channel)
- `405`: Method not allowed

### DELETE /api/bindings

Delete a channel binding.

**Request:**
```
DELETE /api/bindings?frontend=slack&channel_id=C0123456789
```

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `frontend` | string | **Yes** | Frontend name |
| `channel_id` | string | **Yes** | Channel ID within the frontend |

**Response:**
```
HTTP/1.1 204 No Content
```

**Status Codes:**
- `204`: Deleted successfully
- `400`: Bad request (missing parameters)
- `404`: Binding not found
- `405`: Method not allowed

### Binding Examples

```bash
# Create a binding (Slack channel → agent)
curl -X POST http://localhost:8080/api/bindings \
  -H "Content-Type: application/json" \
  -d '{"frontend":"slack","channel_id":"C0123456789","agent_id":"agent-uuid"}'

# Create a binding (Matrix room → agent)
curl -X POST http://localhost:8080/api/bindings \
  -H "Content-Type: application/json" \
  -d '{"frontend":"matrix","channel_id":"!room:matrix.org","agent_id":"agent-uuid"}'

# List all bindings
curl http://localhost:8080/api/bindings

# Delete a binding
curl -X DELETE "http://localhost:8080/api/bindings?frontend=slack&channel_id=C0123456789"

# Send message using binding (looks up agent from binding)
curl -N -X POST http://localhost:8080/api/send \
  -H "Content-Type: application/json" \
  -d '{"content":"Hello!","frontend":"slack","channel_id":"C0123456789"}'
```

## Thread History API

### GET /api/threads/{id}/messages

Get message history for a specific thread.

**Request:**
```http
GET /api/threads/{thread-id}/messages?limit=50 HTTP/1.1
```

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
      "created_at": "2024-01-15T10:30:00Z"
    },
    {
      "id": "msg-uuid-2",
      "thread_id": "550e8400-e29b-41d4-a716-446655440000",
      "sender": "agent",
      "content": "Hello! How can I help you?",
      "created_at": "2024-01-15T10:30:05Z"
    }
  ]
}
```

**Status Codes:**
- `200`: Success
- `404`: Thread not found
- `405`: Method not allowed

**Example:**
```bash
# Get thread messages
curl http://localhost:8080/api/threads/550e8400-e29b-41d4-a716-446655440000/messages

# Get last 10 messages
curl "http://localhost:8080/api/threads/550e8400-e29b-41d4-a716-446655440000/messages?limit=10"
```
