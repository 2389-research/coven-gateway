# fold-gateway

Production control plane for [fold](https://github.com/2389-research/fold) agents. Manages agent connections via gRPC, routes messages from frontends to agents, and streams responses back in real-time.

```
┌─────────────────────────────────────────────────────────────────┐
│                         fold-gateway                            │
│                                                                 │
│   ┌─────────────┐    ┌──────────┐    ┌───────────────────┐     │
│   │   Agent     │    │  Router  │    │    HTTP API       │     │
│   │   Manager   │◄──►│          │◄──►│  (SSE Streaming)  │     │
│   │   (gRPC)    │    │          │    │                   │     │
│   └──────┬──────┘    └────┬─────┘    └─────────┬─────────┘     │
│          │                │                    │               │
└──────────┼────────────────┼────────────────────┼───────────────┘
           │                │                    │
   ┌───────▼───────┐        │           ┌───────▼───────┐
   │  fold-agent   │        │           │   Clients     │
   │  fold-agent   │        │           │  - TUI        │
   │  fold-agent   │        │           │  - Web        │
   └───────────────┘        │           │  - Bots       │
                            │           └───────────────┘
                   ┌────────▼────────┐
                   │    SQLite       │
                   └─────────────────┘
```

## Quick Start

### Prerequisites

- Go 1.22+
- Protocol Buffers compiler (`protoc`)
- A running [fold-agent](https://github.com/2389-research/fold)

### Build

```bash
# Clone
git clone https://github.com/2389-research/fold-gateway.git
cd fold-gateway

# Build (regenerates proto and compiles)
make

# Or just build without proto regeneration
go build -o bin/fold-gateway ./cmd/fold-gateway
go build -o bin/fold-tui ./cmd/fold-tui
```

### Configure

```bash
# Copy example config
cp config.example.yaml config.yaml

# Edit as needed (defaults work for local development)
```

### Run

```bash
# Start the gateway
./bin/fold-gateway serve

# In another terminal, start a fold-agent pointing at the gateway
cd /path/to/fold
cargo run -p fold-agent -- --server http://127.0.0.1:50051 --name "my-agent"

# In a third terminal, interact via TUI
./bin/fold-tui
```

## Features

### Implemented (Phase 1)

- **gRPC Agent Management**: Bidirectional streaming for agent registration and messaging
- **HTTP API**: RESTful endpoints with Server-Sent Events for streaming responses
- **Agent Routing**: Round-robin routing across connected agents
- **SQLite Persistence**: Thread and message storage (pure Go, no CGO)
- **TUI Client**: Interactive terminal client for testing
- **Health Endpoints**: Liveness and readiness checks
- **Graceful Shutdown**: Clean termination with configurable timeout

### Planned

- Affinity routing (same thread → same agent)
- Slack/Matrix frontend integrations
- Prometheus metrics
- mTLS agent authentication

## Usage

### Gateway Commands

```bash
# Start the gateway server
./bin/fold-gateway serve [--config path/to/config.yaml]

# Check gateway health
./bin/fold-gateway health [--addr http://localhost:8080]

# List connected agents
./bin/fold-gateway agents [--addr http://localhost:8080]
```

### TUI Client

```bash
# Connect to gateway
./bin/fold-tui [--server http://localhost:8080]

# Inside TUI:
#   Type message + Enter  → Send to agent
#   /agents               → List connected agents
#   /use <agent-id>       → Target specific agent
#   /help                 → Show commands
#   Ctrl+C                → Exit
```

### HTTP API

```bash
# List agents
curl http://localhost:8080/api/agents

# Send message (SSE streaming response)
curl -N -X POST http://localhost:8080/api/send \
  -H "Content-Type: application/json" \
  -d '{"content": "Hello!", "sender": "user"}'

# Health checks
curl http://localhost:8080/health        # Liveness
curl http://localhost:8080/health/ready  # Readiness (has agents)
```

## Configuration

```yaml
server:
  grpc_addr: "0.0.0.0:50051"  # Agent connections
  http_addr: "0.0.0.0:8080"   # HTTP API

database:
  path: "./fold-gateway.db"   # SQLite path (":memory:" for testing)

routing:
  strategy: "round_robin"     # round_robin, affinity (planned)

agents:
  heartbeat_interval: "30s"
  heartbeat_timeout: "90s"

logging:
  level: "info"               # debug, info, warn, error
  format: "text"              # text, json
```

Environment variables can be used with `${VAR}` syntax:

```yaml
slack:
  bot_token: "${SLACK_BOT_TOKEN}"
```

## Architecture

```
fold-gateway/
├── cmd/
│   ├── fold-gateway/     # Gateway server binary
│   └── fold-tui/         # Interactive TUI client
├── internal/
│   ├── agent/            # Agent connection management
│   │   ├── manager.go    # Connection registry and message routing
│   │   ├── connection.go # Single agent stream handler
│   │   └── router.go     # Routing strategy implementation
│   ├── config/           # YAML configuration loading
│   ├── gateway/          # Main orchestrator
│   │   ├── gateway.go    # Server lifecycle management
│   │   ├── grpc.go       # FoldControl gRPC service
│   │   └── api.go        # HTTP API handlers
│   └── store/            # SQLite persistence
├── proto/fold/           # Generated protobuf code
├── docs/
│   ├── AGENT_PROTOCOL.md # gRPC protocol for agents
│   └── CLIENT_PROTOCOL.md # HTTP API for clients
└── config.example.yaml
```

## Protocol Documentation

- **[Agent Protocol](docs/AGENT_PROTOCOL.md)**: gRPC bidirectional streaming protocol for fold-agents
- **[Client Protocol](docs/CLIENT_PROTOCOL.md)**: HTTP API with SSE for frontends and clients

## Development

### Build & Test

```bash
# Full build (proto + binary)
make

# Run tests
go test ./...

# Run tests with race detection
go test -race ./...

# Run specific package tests
go test ./internal/agent/...
```

### Proto Generation

Requires the fold repo checked out as a sibling:

```bash
# Install protoc plugins (one time)
make proto-deps

# Regenerate proto
make proto
```

### Pre-commit Hooks

```bash
# Install pre-commit
pip install pre-commit
pre-commit install

# Hooks run: go fmt, go vet, go test, go mod tidy
```

## Connecting Agents

Agents connect via gRPC and must:

1. Open bidirectional stream to `AgentStream` RPC
2. Send `RegisterAgent` message with unique ID
3. Receive `Welcome` confirmation
4. Process `SendMessage` requests, respond with `MessageResponse` stream

See [Agent Protocol](docs/AGENT_PROTOCOL.md) for full details.

### Example: Rust Agent (tonic)

```rust
let channel = Channel::from_static("http://localhost:50051")
    .connect().await?;
let mut client = FoldControlClient::new(channel);

let (tx, rx) = mpsc::channel(100);
let response = client.agent_stream(ReceiverStream::new(rx)).await?;

// Send registration
tx.send(AgentMessage {
    payload: Some(Payload::Register(RegisterAgent {
        agent_id: uuid::Uuid::new_v4().to_string(),
        name: "my-agent".to_string(),
        capabilities: vec!["chat".to_string()],
    })),
}).await?;

// Process incoming messages...
```

## License

MIT

## Related Projects

- [fold](https://github.com/2389-research/fold) - The fold agent framework (Rust)
- [mux](https://github.com/2389-research/mux-rs) - Claude API streaming library
