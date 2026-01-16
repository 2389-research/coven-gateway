# Affinity Routing Design

## Overview

Channel-level affinity routing binds frontend channels (Slack channels, Matrix rooms) to specific agents. All messages from a bound channel route to that agent. If the agent is offline, the request fails.

## Data Model

### ChannelBinding

```go
type ChannelBinding struct {
    FrontendName string    // "slack", "matrix", "discord"
    ChannelID    string    // External channel identifier
    AgentID      string    // The bound agent's UUID
    CreatedAt    time.Time
    UpdatedAt    time.Time
}
```

Primary key: `(FrontendName, ChannelID)`

### Storage Schema

```sql
CREATE TABLE channel_bindings (
    frontend TEXT NOT NULL,
    channel_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    PRIMARY KEY (frontend, channel_id)
);
```

## API Endpoints

### Binding Management

```
POST   /api/bindings          Create a binding
GET    /api/bindings          List all bindings
DELETE /api/bindings/:id      Remove a binding (id = frontend:channel_id)
```

#### Create Binding

```
POST /api/bindings
{
  "frontend": "slack",
  "channel_id": "C0123456789",
  "agent_id": "agent-uuid-here"
}
```

Response:
```json
{
  "frontend": "slack",
  "channel_id": "C0123456789",
  "agent_id": "agent-uuid-here",
  "created_at": "2026-01-15T10:30:00Z"
}
```

Binding to an offline agent is allowed (for setup before agent connects).

#### List Bindings

```
GET /api/bindings
```

Response:
```json
{
  "bindings": [
    {
      "frontend": "slack",
      "channel_id": "C0123456789",
      "agent_id": "agent-uuid",
      "agent_online": true,
      "created_at": "2026-01-15T10:30:00Z"
    }
  ]
}
```

#### Delete Binding

```
DELETE /api/bindings/slack:C0123456789
```

### Message Routing

Update `/api/send` to accept frontend context:

```json
{
  "content": "Hello!",
  "sender": "user",
  "frontend": "slack",
  "channel_id": "C0123456789"
}
```

## Routing Logic

Two paths for message routing:

### 1. Frontend Integration Path

When `frontend` + `channel_id` provided:
1. Look up binding for that channel
2. No binding → 400 "channel not bound to agent"
3. Binding exists, agent online → route to agent
4. Binding exists, agent offline → 503 "agent unavailable"

### 2. Direct API Path

When `agent_id` provided directly (TUI, direct API users):
1. Route to that agent
2. Agent not found → 404
3. Agent offline → 503 "agent unavailable"

### 3. Neither Provided

Return 400 "must specify frontend+channel_id or agent_id"

## Store Interface Changes

Add to `Store` interface:

```go
// Channel bindings
CreateBinding(ctx context.Context, binding *ChannelBinding) error
GetBinding(ctx context.Context, frontend, channelID string) (*ChannelBinding, error)
ListBindings(ctx context.Context) ([]*ChannelBinding, error)
DeleteBinding(ctx context.Context, frontend, channelID string) error
```

## Removals

- Delete `internal/agent/router.go` (round-robin router)
- Delete `internal/agent/router_test.go`
- Remove `routing.strategy` from config

## Error Responses

| Condition | HTTP Status | Message |
|-----------|-------------|---------|
| Channel not bound | 400 | "channel not bound to agent" |
| Agent not found | 404 | "agent not found" |
| Agent offline | 503 | "agent unavailable" |
| Missing routing info | 400 | "must specify frontend+channel_id or agent_id" |
