# Bindings Flow Design

## Overview

This document defines how users bind chat channels to agents using the `/fold` command in frontends (Matrix, Slack). Bindings route messages from a channel to a specific agent running in a specific working directory.

## Data Model

### Binding

A binding maps a channel to an agent instance (identity + working directory):

```sql
bindings (
  binding_id    TEXT PRIMARY KEY,
  frontend      TEXT NOT NULL,     -- "matrix", "slack"
  channel_id    TEXT NOT NULL,     -- room/channel identifier
  principal_id  TEXT NOT NULL,     -- SSH key identity (FK → principals)
  working_dir   TEXT NOT NULL,     -- agent's working directory
  created_at    TEXT NOT NULL,
  created_by    TEXT,              -- principal who created it
  UNIQUE(frontend, channel_id)
)
```

### Constraints

- One channel → one agent (unique on frontend + channel_id)
- One agent ← many channels (no constraint on agent side)
- Agent identity = principal_id + working_dir (same principal in different directories = different logical agent)

## Chat Commands

Users interact via `/fold` commands in their chat platform:

| Command | Description |
|---------|-------------|
| `/fold bind <instance-id>` | Bind this channel to the agent with that instance ID |
| `/fold unbind` | Remove binding from this channel |
| `/fold status` | Show current binding (agent name + working dir) |
| `/fold agents` | List online agents with their instance IDs |

### Example Flow

1. User runs `/fold agents` in Matrix room:
   ```
   Online agents:
   • bob at /projects/website (0fb8187d-c06)
   • bob at /projects/api (a3f2e1d0-b89)
   • alice at /work/client (7c4b2a91-e3f)
   ```

2. User runs `/fold bind 0fb8187d-c06`

3. Bot confirms:
   ```
   ✓ Bound to agent bob at /projects/website
   ```

4. Future messages in this room route to that agent.

## Gateway API

### Create Binding

```
POST /api/bindings
{
  "frontend": "matrix",
  "channel_id": "!roomid:server",
  "instance_id": "0fb8187d-c06"
}
```

Gateway:
1. Looks up connected agent by instance_id
2. Extracts principal_id + working_dir from connection
3. Creates binding (or updates if channel already bound)
4. Returns agent info for confirmation message

Response:
```json
{
  "binding_id": "uuid",
  "agent_name": "bob",
  "working_dir": "/projects/website",
  "rebound_from": null
}
```

### Delete Binding

```
DELETE /api/bindings?frontend=matrix&channel_id=!roomid:server
```

### Get Binding (Status)

```
GET /api/bindings?frontend=matrix&channel_id=!roomid:server
```

Response:
```json
{
  "binding_id": "uuid",
  "agent_name": "bob",
  "working_dir": "/projects/website",
  "online": true
}
```

### List Online Agents

```
GET /api/agents
```

Response:
```json
{
  "agents": [
    {
      "instance_id": "0fb8187d-c06",
      "name": "bob",
      "working_dir": "/projects/website",
      "workspaces": ["Code", "Personal"]
    }
  ]
}
```

## Error Handling

| Scenario | HTTP | Response |
|----------|------|----------|
| Instance ID not found | 404 | `{"error": "no agent online with instance_id 'xxx'"}` |
| Already bound to same agent | 200 | `{"message": "already bound to this agent"}` |
| Already bound to different | 200 | `{"rebound_from": {"name": "alice", "working_dir": "..."}}` |
| Unbind when not bound | 404 | `{"error": "no binding for this channel"}` |
| Status when not bound | 404 | `{"error": "no binding for this channel"}` |
| No agents online | 200 | `{"agents": []}` |

## Frontend Integration

### Matrix Bridge

The Matrix bridge (`fold-matrix`) handles `/fold` commands:

1. Receives message in room
2. Detects `/fold` prefix
3. Parses command (bind/unbind/status/agents)
4. Calls gateway API
5. Sends response message to room

The bridge is stateless regarding bindings - all storage and routing is in the gateway.

### Slack (Future)

Same pattern - Slack bot receives slash command, calls gateway API.

## Admin Interface

Operators can manage bindings via:

- **Web Admin** (`/admin/bindings`) - List, create, delete bindings
- **fold-admin CLI** - `fold-admin bindings list`, `fold-admin bindings delete`

Admin can see all bindings across all frontends, filter by agent or frontend.

## Migration

The current `bindings` table stores only `agent_id` (principal_id). We need to:

1. Add `working_dir` column to bindings table
2. Existing bindings get `working_dir = NULL` (route to any instance of that principal)
3. New bindings require working_dir

## Routing Logic

When a message arrives for a bound channel:

1. Look up binding by (frontend, channel_id)
2. Find online agent matching (principal_id, working_dir)
3. If no exact match but working_dir is NULL, find any online instance of that principal
4. If no online agent, queue or reject (TBD)

## Out of Scope

- Binding to offline agents (must be online to get instance_id)
- Multiple agents per channel
- Permission checks on who can bind (v1: anyone in the channel can bind)
