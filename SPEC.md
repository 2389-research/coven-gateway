# fold-gateway Specification

## Overview

fold-gateway is the production control plane for fold agents. It manages agent connections via GRPC, routes messages from various frontends (Slack, Matrix, web) to agents, and handles streaming responses back to users.

```
┌─────────────────────────────────────────────────────────────────┐
│                        fold-gateway                              │
│  ┌─────────────────┐  ┌──────────────┐  ┌───────────────────┐   │
│  │  Agent Manager  │  │   Router     │  │  Frontend Manager │   │
│  │  (GRPC Server)  │  │              │  │                   │   │
│  └────────┬────────┘  └──────┬───────┘  └─────────┬─────────┘   │
│           │                  │                    │             │
└───────────┼──────────────────┼────────────────────┼─────────────┘
            │                  │                    │
    ┌───────▼───────┐          │          ┌────────▼────────┐
    │  fold-agent   │          │          │    Frontends    │
    │  fold-agent   │          │          │  - Slack Bot    │
    │  fold-agent   │          │          │  - Matrix Bot   │
    └───────────────┘          │          │  - Web UI       │
                               │          └─────────────────┘
                               │
                      ┌────────▼────────┐
                      │    SQLite DB    │
                      │  (threads, etc) │
                      └─────────────────┘
```

## Core Responsibilities

1. **Agent Management**: Accept GRPC connections from fold-agents, track their state, route messages to them
2. **Frontend Integration**: Connect to Slack, Matrix, and other chat platforms to receive user messages
3. **Message Routing**: Route incoming messages to appropriate agents based on capabilities, load, or affinity
4. **Response Streaming**: Stream agent responses back to the originating frontend in real-time
5. **Thread Persistence**: Store conversation threads for context continuity
6. **Health Monitoring**: Track agent health, handle disconnections, reconnection logic

## Technology Stack

- **Language**: Go 1.22+
- **GRPC**: google.golang.org/grpc
- **Protobuf**: google.golang.org/protobuf
- **Database**: SQLite via modernc.org/sqlite (pure Go, no CGO)
- **Slack**: github.com/slack-go/slack
- **Matrix**: maunium.net/go/mautrix
- **Config**: YAML via gopkg.in/yaml.v3
- **Logging**: log/slog (stdlib)
- **Metrics**: prometheus/client_golang (optional)

## GRPC Protocol

Uses the existing `proto/fold.proto` from the fold repository. The gateway implements the `FoldControl` service:

```protobuf
service FoldControl {
  rpc AgentStream(stream AgentMessage) returns (stream ServerMessage);
}
```

### Agent Messages (Agent → Gateway)

| Message | Purpose |
|---------|---------|
| `RegisterAgent` | Agent announces itself with ID, name, capabilities |
| `MessageResponse` | Streaming response events (thinking, text, tool_use, done, error) |
| `Heartbeat` | Keep-alive signal |

### Server Messages (Gateway → Agent)

| Message | Purpose |
|---------|---------|
| `Welcome` | Confirms registration, provides server ID |
| `SendMessage` | Request agent to process a user message |
| `Shutdown` | Graceful shutdown request |

## Architecture

### Package Structure

```
fold-gateway/
├── cmd/
│   └── fold-gateway/
│       └── main.go           # Entry point
├── internal/
│   ├── agent/
│   │   ├── manager.go        # Agent connection management
│   │   ├── connection.go     # Single agent connection handler
│   │   └── router.go         # Message routing logic
│   ├── frontend/
│   │   ├── frontend.go       # Frontend interface
│   │   ├── slack/
│   │   │   └── slack.go      # Slack bot implementation
│   │   ├── matrix/
│   │   │   └── matrix.go     # Matrix bot implementation
│   │   └── web/
│   │       └── web.go        # Web UI (future)
│   ├── store/
│   │   ├── store.go          # Thread/message persistence
│   │   └── sqlite.go         # SQLite implementation
│   ├── config/
│   │   └── config.go         # Configuration loading
│   └── gateway/
│       └── gateway.go        # Main orchestrator
├── proto/
│   └── fold/
│       └── fold.pb.go        # Generated from fold.proto
├── config.example.yaml
├── go.mod
├── go.sum
└── SPEC.md
```

### Core Components

#### 1. Gateway (Orchestrator)

The main struct that ties everything together:

```go
type Gateway struct {
    config        *config.Config
    agentManager  *agent.Manager
    frontends     []frontend.Frontend
    store         store.Store
    grpcServer    *grpc.Server
}

func (g *Gateway) Run(ctx context.Context) error
func (g *Gateway) Shutdown(ctx context.Context) error
```

#### 2. Agent Manager

Manages all connected agents:

```go
type Manager struct {
    agents    map[string]*Connection  // agent_id -> connection
    mu        sync.RWMutex
    router    *Router
}

func (m *Manager) Register(agent *Connection) error
func (m *Manager) Unregister(agentID string)
func (m *Manager) SendMessage(ctx context.Context, req *SendRequest) (<-chan *Response, error)
func (m *Manager) ListAgents() []*AgentInfo
```

#### 3. Agent Connection

Handles a single agent's bidirectional stream:

```go
type Connection struct {
    ID           string
    Name         string
    Capabilities []string
    stream       pb.FoldControl_AgentStreamServer
    pending      map[string]chan *pb.MessageResponse  // request_id -> response channel
    mu           sync.RWMutex
}

func (c *Connection) Send(msg *pb.ServerMessage) error
func (c *Connection) HandleMessage(msg *pb.AgentMessage) error
```

#### 4. Router

Routes messages to appropriate agents:

```go
type Router struct {
    strategy RoutingStrategy
}

type RoutingStrategy interface {
    SelectAgent(agents []*Connection, req *SendRequest) (*Connection, error)
}

// Strategies:
// - RoundRobin: distribute load evenly
// - Affinity: same thread goes to same agent
// - Capability: match required capabilities
// - Random: random selection
```

#### 5. Frontend Interface

Common interface for all frontends:

```go
type Frontend interface {
    Name() string
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    SetMessageHandler(handler MessageHandler)
}

type MessageHandler func(ctx context.Context, msg *IncomingMessage) (<-chan *OutgoingEvent, error)

type IncomingMessage struct {
    FrontendName string
    ThreadID     string
    Sender       string
    Content      string
    Attachments  []Attachment
}

type OutgoingEvent struct {
    Type    EventType  // Thinking, Text, ToolUse, ToolResult, Done, Error, File
    Payload any
}
```

#### 6. Store Interface

Thread and message persistence:

```go
type Store interface {
    // Threads
    CreateThread(ctx context.Context, thread *Thread) error
    GetThread(ctx context.Context, id string) (*Thread, error)
    UpdateThread(ctx context.Context, thread *Thread) error

    // Messages (for audit/history)
    SaveMessage(ctx context.Context, msg *Message) error
    GetThreadMessages(ctx context.Context, threadID string, limit int) ([]*Message, error)

    // Agent state (optional)
    SaveAgentState(ctx context.Context, agentID string, state []byte) error
    GetAgentState(ctx context.Context, agentID string) ([]byte, error)
}
```

## Configuration

```yaml
# config.yaml
server:
  grpc_addr: "0.0.0.0:50051"
  http_addr: "0.0.0.0:8080"  # health checks, metrics

database:
  path: "./fold-gateway.db"

routing:
  strategy: "affinity"  # round_robin, affinity, capability, random

agents:
  heartbeat_interval: "30s"
  heartbeat_timeout: "90s"
  reconnect_grace_period: "5m"

frontends:
  slack:
    enabled: true
    app_token: "${SLACK_APP_TOKEN}"
    bot_token: "${SLACK_BOT_TOKEN}"
    allowed_channels:
      - "C0123456789"

  matrix:
    enabled: true
    homeserver: "https://matrix.org"
    user_id: "@fold:matrix.org"
    access_token: "${MATRIX_ACCESS_TOKEN}"
    allowed_users:
      - "@harper:matrix.org"
    allowed_rooms:
      - "!room:matrix.org"

logging:
  level: "info"  # debug, info, warn, error
  format: "json"  # json, text

metrics:
  enabled: true
  path: "/metrics"
```

## Message Flow

### User sends message via Slack:

```
1. Slack Frontend receives message
2. Frontend calls MessageHandler with IncomingMessage
3. Gateway looks up or creates thread
4. Router selects agent (affinity: same thread → same agent if available)
5. Gateway sends SendMessage to agent via GRPC stream
6. Agent processes message with Claude
7. Agent streams MessageResponse events back
8. Gateway forwards events to Frontend
9. Frontend sends Slack messages for each text chunk
10. On Done event, Frontend sends final message
11. Gateway saves thread state to store
```

### Sequence Diagram:

```
User         Slack        Gateway       Router        Agent         Claude
  │            │             │            │             │             │
  │──message──▶│             │            │             │             │
  │            │──Incoming──▶│            │             │             │
  │            │             │──select───▶│             │             │
  │            │             │◀──agent────│             │             │
  │            │             │────────SendMessage──────▶│             │
  │            │             │             │            │──prompt────▶│
  │            │             │             │            │◀──stream────│
  │            │◀───────────Text──────────────────────│             │
  │◀──typing───│             │             │            │             │
  │            │◀───────────Text──────────────────────│             │
  │◀──message──│             │             │            │             │
  │            │◀───────────Done──────────────────────│             │
  │◀──message──│             │             │            │             │
```

## Routing Strategies

### 1. Round Robin
Distributes messages evenly across all available agents. Simple but doesn't preserve thread context.

### 2. Affinity (Recommended)
Routes messages from the same thread to the same agent when possible. Falls back to round robin if the preferred agent is unavailable. Preserves conversation context within agents.

```go
type AffinityRouter struct {
    affinities map[string]string  // thread_id -> agent_id
    mu         sync.RWMutex
}
```

### 3. Capability-Based
Routes based on required capabilities (e.g., "code", "image", "search"). Useful when agents have different tool configurations.

### 4. Random
Random selection. Useful for testing load distribution.

## Error Handling

### Agent Disconnection
1. Mark agent as disconnected
2. Start grace period timer (configurable, default 5m)
3. If agent reconnects within grace period, restore state
4. If grace period expires, reassign pending requests to other agents
5. Notify frontends of any failed requests

### Frontend Errors
1. Log error with context
2. If transient (network), retry with backoff
3. If permanent (auth), disable frontend and alert

### Request Timeout
1. Default timeout: 5 minutes per request
2. On timeout, send error to frontend
3. Log for debugging
4. Consider agent health status

## Health Checks

### HTTP Endpoints

```
GET /health          # Basic liveness
GET /health/ready    # Readiness (has agents, frontends connected)
GET /metrics         # Prometheus metrics
```

### Metrics to Track

- `fold_agents_connected` (gauge): Number of connected agents
- `fold_requests_total` (counter): Total requests processed, labeled by frontend
- `fold_request_duration_seconds` (histogram): Request latency
- `fold_errors_total` (counter): Errors by type
- `fold_agent_messages_total` (counter): Messages sent to agents

## Security Considerations

### GRPC
- Support mTLS for agent authentication (optional, configurable)
- Agent ID validation on registration
- Rate limiting per agent

### Frontends
- Allowlist for Slack channels
- Allowlist for Matrix users/rooms
- Input sanitization before forwarding to agents

### Configuration
- Environment variable expansion for secrets
- No secrets in config files
- Support for secret management (e.g., Vault) in future

## Implementation Tasks

### Phase 1: Core Infrastructure
1. [ ] Project setup (go.mod, directory structure)
2. [ ] Generate Go code from fold.proto
3. [ ] Implement config loading
4. [ ] Implement SQLite store
5. [ ] Implement Agent Manager + Connection
6. [ ] Implement GRPC server (FoldControl service)
7. [ ] Basic integration test with fold-agent

### Phase 2: Routing & Reliability
8. [ ] Implement routing strategies (affinity first)
9. [ ] Implement health checks
10. [ ] Add graceful shutdown
11. [ ] Add reconnection handling
12. [ ] Add request timeout handling

### Phase 3: Frontends
13. [ ] Implement Frontend interface
14. [ ] Implement Slack frontend
15. [ ] Implement Matrix frontend
16. [ ] End-to-end testing with real agents

### Phase 4: Production Readiness
17. [ ] Add Prometheus metrics
18. [ ] Add structured logging
19. [ ] Add mTLS support
20. [ ] Documentation and deployment guide

## CLI Interface

```bash
# Run the gateway
fold-gateway serve --config config.yaml

# Check health
fold-gateway health

# List connected agents (connects to running gateway)
fold-gateway agents list

# Send test message
fold-gateway test --agent <id> --message "Hello"
```

## Testing Strategy

### Unit Tests
- Router strategy selection
- Config parsing
- Store operations
- Message serialization

### Integration Tests
- GRPC server with mock agent
- Frontend message handling
- Full request/response cycle

### End-to-End Tests
- Gateway + real fold-agent
- Gateway + Slack test workspace
- Gateway + Matrix test server

## Future Considerations

- **Web UI**: React-based admin interface
- **Agent Groups**: Logical grouping for routing
- **Multi-tenancy**: Isolate threads/agents by tenant
- **Caching**: Response caching for repeated queries
- **Queueing**: Message queue for high load (NATS, Redis)
- **Clustering**: Multiple gateway instances for HA
