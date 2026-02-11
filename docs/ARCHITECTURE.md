# coven-gateway Architecture

This document describes the actual implemented architecture of coven-gateway, the production control plane for coven agents.

## Overview

coven-gateway manages coven-agent connections via gRPC, routes messages from clients (HTTP API, web admin) to agents, and streams responses back in real-time.

```
                    ┌─────────────────────────────────────────────────────────┐
                    │                    coven-gateway                         │
                    │                                                         │
                    │  ┌─────────────┐  ┌──────────────┐  ┌───────────────┐  │
HTTP Clients ──────►│  │   HTTP API  │  │ Pack Router  │  │  MCP Server   │  │
(SSE streaming)     │  │   (api.go)  │  │              │  │               │  │
                    │  └──────┬──────┘  └──────┬───────┘  └───────┬───────┘  │
                    │         │                │                  │          │
Web Admin ─────────►│  ┌──────▼────────────────▼──────────────────▼───────┐  │
(Chat UI)           │  │              Agent Manager                        │  │
                    │  │  (tracks connections, routes messages)            │  │
                    │  └──────────────────────┬────────────────────────────┘  │
                    │                         │                               │
                    │  ┌──────────────────────▼────────────────────────────┐  │
                    │  │                    Store                          │  │
                    │  │  (SQLite: threads, messages, events, bindings)    │  │
                    │  └───────────────────────────────────────────────────┘  │
                    └─────────────────────────┬───────────────────────────────┘
                                              │ gRPC (bidirectional stream)
                                    ┌─────────▼─────────┐
                                    │   coven-agent(s)  │
                                    │   (Rust, Claude)  │
                                    └───────────────────┘
```

## Package Structure

```
coven-gateway/
├── cmd/
│   ├── coven-gateway/        # Main server binary
│   └── coven-admin/          # CLI admin tool
├── internal/
│   ├── gateway/              # Orchestrator: Gateway struct, HTTP handlers
│   ├── agent/                # Agent lifecycle: Manager, Connection
│   ├── store/                # SQLite persistence
│   ├── conversation/         # Conversation service
│   ├── packs/                # Tool pack registry and router
│   ├── builtins/             # Built-in tool packs (base, notes, mail, admin, ui)
│   ├── mcp/                  # MCP server for external tool access
│   ├── webadmin/             # Web admin UI (embedded templates, handlers)
│   ├── auth/                 # Principal-based authentication
│   ├── config/               # Configuration loading
│   ├── dedupe/               # Message deduplication
│   ├── contract/             # Protocol contract tests
│   ├── client/               # gRPC client (for testing/admin)
│   └── admin/                # Admin operations
├── proto/
│   └── coven-proto/          # Protobuf definitions (git submodule)
└── docs/
```

## Core Components

### Gateway (Orchestrator)

The main struct that coordinates all components (`internal/gateway/gateway.go`):

```go
type Gateway struct {
    config         *config.Config
    agentManager   *agent.Manager
    store          store.Store
    conversation   *conversation.Service
    grpcServer     *grpc.Server
    httpServer     *http.Server
    tsnetServer    *tsnet.Server       // Tailscale integration
    webAdmin       *webadmin.Admin
    logger         *slog.Logger
    serverID       string
    dedupe         *dedupe.Cache       // Bridge message dedup
    packRegistry   *packs.Registry     // Tool pack tracking
    packRouter     *packs.Router       // Tool call routing
    mcpTokens      *mcp.TokenStore     // MCP access tokens
    mcpServer      *mcp.Server         // MCP HTTP server
    mcpEndpoint    string              // MCP base URL
    questionRouter *QuestionRouter     // User interaction routing
    eventBroadcaster *EventBroadcaster // SSE event broadcasting
}
```

### Agent Manager

Manages connected agents (`internal/agent/manager.go`):

- Tracks active connections by agent ID
- Routes messages to appropriate agents via bindings
- Handles agent registration and disconnection
- Correlates requests with responses by request_id

### Store Interfaces

The persistence layer uses multiple specialized interfaces (`internal/store/`):

| Interface | Purpose |
|-----------|---------|
| `Store` | Core: threads, messages, events, bindings (~40 methods) |
| `BuiltinStore` | Built-in tool data: logs, todos, BBS, notes, mail |
| `AdminStore` | Admin users, WebAuthn credentials, sessions |
| `UsageStore` | Token usage tracking and statistics |
| `SecretsStore` | Secret management |
| `LinkCodeStore` | Device linking codes |

### Tool Packs

External processes that provide tools to agents (`internal/packs/`):

- **Registry**: Tracks connected packs and their tools
- **Router**: Routes tool calls to the appropriate pack
- **Built-in packs**: 5 packs with 21 tools (base, notes, mail, admin, ui)

### MCP Server

Model Context Protocol server for external AI tool access (`internal/mcp/`):

- Token-based authentication
- HTTP/SSE transport
- Exposes gateway tools to external clients

### Web Admin

Browser-based admin interface (`internal/webadmin/`):

- Chat UI for conversations
- Agent management
- Settings and configuration
- WebAuthn/passkey authentication

## Message Flow

### HTTP Client sends message:

```
1. POST /api/send with JSON {agent_id, content, sender, ...}
2. api.go validates and looks up agent (direct ID or binding)
3. Manager.SendMessage creates request, sends via Connection's gRPC stream
4. Connection correlates responses by request_id
5. Agent processes with Claude, streams response events
6. api.go streams events as SSE: thinking, text, tool_use, done
7. Store saves message and events for history
```

### Web Admin chat:

```
1. User types message in chat UI
2. WebSocket/SSE endpoint receives message
3. Conversation service manages thread context
4. Message routed to agent via Manager
5. Response events streamed back in real-time
6. UI updates with thinking indicators, text, tool usage
```

## gRPC Protocol

Uses `CovenControl` service with bidirectional streaming:

### Agent → Gateway (AgentMessage)

| Message | Purpose |
|---------|---------|
| `RegisterAgent` | Registration with metadata, capabilities, protocol_features |
| `MessageResponse` | Response events: Thinking, Text, ToolUse, ToolResult, Done, Error |
| `Heartbeat` | Keep-alive |
| `InjectionAck` | Context injection acknowledgment |
| `PackToolResult` | Tool execution result from pack |
| `ToolStateUpdate` | Tool approval state changes |
| `Cancelled` | Request cancellation acknowledgment |

### Gateway → Agent (ServerMessage)

| Message | Purpose |
|---------|---------|
| `Welcome` | Registration confirmation with server_id, request_id |
| `SendMessage` | Request to process user message |
| `ExecutePackTool` | Execute a pack tool |
| `InjectContext` | Inject context into agent |
| `CancelRequest` | Cancel a pending request |
| `ToolApprovalResponse` | Approve/deny tool execution |
| `Shutdown` | Graceful shutdown request |

## Authentication

### Agent Authentication

- SSH public key fingerprint in registration
- Auto-registration modes: approved, pending, disabled
- Principal-based identity system

### Admin Authentication

- JWT tokens with HS256 signing
- WebAuthn/passkey support (requires HTTPS)
- CSRF protection for forms
- Optional: when `jwt_secret` not configured, auth is disabled

## Configuration

YAML-based with environment variable expansion:

```yaml
server:
  grpc_addr: "0.0.0.0:50051"
  http_addr: "0.0.0.0:8080"

database:
  path: "~/.local/share/coven/gateway.db"

auth:
  jwt_secret: "${COVEN_JWT_SECRET}"
  agent_auto_registration: "approved"

tailscale:
  enabled: false
  hostname: "coven-gateway"
  auth_key: "${TS_AUTHKEY}"

agents:
  heartbeat_interval: "30s"
  heartbeat_timeout: "90s"
  reconnect_grace_period: "5m"

logging:
  level: "info"
  format: "text"
```

## Built-in Tool Packs

5 packs providing 21 tools to agents:

| Pack | Capability | Tools |
|------|------------|-------|
| `builtin:base` | base | log_entry, log_search, todo_*, bbs_* |
| `builtin:notes` | notes | note_set, note_get, note_list, note_delete |
| `builtin:mail` | mail | mail_send, mail_inbox, mail_read |
| `builtin:admin` | admin | admin_list_agents, admin_agent_messages, admin_send_message |
| `builtin:ui` | ui | ask_user |

## Key Design Decisions

### Why SQLite?

- Pure Go implementation (no CGO)
- Single file database, easy deployment
- WAL mode for concurrent reads
- Sufficient for typical workloads

### Why Bidirectional Streaming?

- Real-time response streaming
- Agent-initiated events (heartbeat, tool state)
- Long-lived connections with keepalive
- Natural fit for Claude's streaming API

### Why Principal-Based Auth?

- Unified identity for agents, users, admins
- Flexible capability assignment
- Works with SSH keys, JWT, WebAuthn
- Audit trail via principal IDs

### Why Tailscale Integration?

- Automatic TLS certificates
- Easy remote access without port forwarding
- Funnel for public exposure
- Mesh networking for distributed agents
