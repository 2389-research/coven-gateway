# coven-gateway

Production control plane for coven agents. Manages agent connections via gRPC, routes messages from frontends to agents, and streams responses back in real-time.

## Related Projects

coven-gateway is part of the Coven ecosystem:

| Repository | Language | Purpose |
|------------|----------|---------|
| **coven-gateway** (this repo) | Go | Control plane server (`coven-gateway`, `coven-admin`) |
| [coven](https://github.com/2389-research/coven) | Rust | Agent platform (`coven-agent`, `coven-tui`, `coven-swarm`) |
| [coven-proto](https://github.com/2389-research/coven-proto) | Protobuf | Shared protocol definitions |

To connect agents to this gateway, install from the [coven](https://github.com/2389-research/coven) project.

```
┌─────────────────────────────────────────────────────────────────┐
│                         coven-gateway                            │
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
   │  coven-agent   │        │           │   Clients     │
   │  coven-agent   │        │           │  - TUI        │
   │  coven-agent   │        │           │  - Web        │
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
- A running agent from [coven](https://github.com/2389-research/coven)

### Build

```bash
# Clone
git clone https://github.com/2389-research/coven-gateway.git
cd coven-gateway

# Build (regenerates proto and compiles)
make

# Or build individual binaries without proto regeneration
go build -o bin/coven-gateway ./cmd/coven-gateway
go build -o bin/coven-admin ./cmd/coven-admin
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
./bin/coven-gateway serve

# In another terminal, start a coven-agent pointing at the gateway
cd /path/to/coven-agent
cargo run -p coven-agent -- --server http://127.0.0.1:50051 --name "my-agent"

# In a third terminal, interact via TUI
./bin/coven-tui
```

## Features

### Implemented (Phase 1)

- **gRPC Agent Management**: Bidirectional streaming for agent registration and messaging
- **HTTP API**: RESTful endpoints with Server-Sent Events for streaming responses
- **Agent Routing**: Channel binding routing with sticky agent assignment
- **SQLite Persistence**: Thread and message storage (pure Go, no CGO)
- **TUI Client**: Interactive terminal client for testing
- **Health Endpoints**: Liveness and readiness checks
- **Graceful Shutdown**: Clean termination with configurable timeout
- **Tailscale Integration**: Run as a node on your tailnet via [tsnet](https://tailscale.com/kb/1244/tsnet)
- **Matrix Bridge**: See [coven-matrix](https://github.com/2389-research/coven-matrix) (Rust) for Matrix integration

### Planned

- Slack frontend integration
- Prometheus metrics
- mTLS agent authentication

## Usage

### Gateway Commands

```bash
# Create config file interactively (first-time setup)
./bin/coven-gateway init

# Create initial owner principal and get a JWT token (first-time setup)
./bin/coven-gateway bootstrap --name "Your Name"

# Start the gateway server
./bin/coven-gateway serve
# Config via: COVEN_CONFIG=path/to/config.yaml or ~/.config/coven/gateway.yaml

# Check gateway health
./bin/coven-gateway health

# List connected agents
./bin/coven-gateway agents
```

### TUI Client

The TUI client (`coven-tui`) is part of the companion [coven](https://github.com/2389-research/coven) Rust project. Install it from there, then:

```bash
# Connect to gateway
coven-tui [-server http://localhost:8080]

# Inside TUI:
#   Type message + Enter  → Send to agent
#   /agents               → List connected agents
#   /use <agent-id>       → Target specific agent
#   /help                 → Show commands
#   Ctrl+C                → Exit
```

### Admin CLI

```bash
# Set up authentication
export COVEN_TOKEN="<your-jwt-token>"
export COVEN_GATEWAY_HOST="localhost"  # or your-gateway.tailnet.ts.net

# Show your identity
./bin/coven-admin me

# Show gateway status
./bin/coven-admin status

# List channel bindings
./bin/coven-admin bindings

# List registered agents
./bin/coven-admin agents

# Create a binding
./bin/coven-admin bindings create --frontend matrix --channel '!room:example.org' --agent <agent-id>

# Create a JWT token for a principal
./bin/coven-admin token create --principal <id>

# Create an admin invite link
./bin/coven-admin invite create

# Chat with an agent (one-shot)
./bin/coven-admin chat <agent-id> "Hello!"

# Chat with an agent (interactive REPL)
./bin/coven-admin chat <agent-id>
```

**Environment variables:**
- `COVEN_TOKEN` - JWT authentication token (required for most commands)
- `COVEN_GATEWAY_HOST` - Gateway hostname (derives gRPC :50051 URL)

### Matrix Bridge

For Matrix integration, see [coven-matrix](https://github.com/2389-research/coven-matrix) - a standalone Rust bridge connecting Matrix rooms to coven agents with E2EE support.

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

### Channel Bindings

Bind frontend channels (Slack, Matrix, etc.) to specific agents for sticky routing.

```bash
# Create a binding (channel → agent)
curl -X POST http://localhost:8080/api/bindings \
  -H "Content-Type: application/json" \
  -d '{"frontend":"slack","channel_id":"C0123456789","agent_id":"agent-uuid"}'

# List all bindings
curl http://localhost:8080/api/bindings

# Delete a binding
curl -X DELETE "http://localhost:8080/api/bindings?frontend=slack&channel_id=C0123456789"
```

## Configuration

```yaml
server:
  grpc_addr: "0.0.0.0:50051"  # Agent connections
  http_addr: "0.0.0.0:8080"   # HTTP API

database:
  path: "./coven-gateway.db"   # SQLite path (":memory:" for testing)

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

## Tailscale Integration

Run coven-gateway as a node on your [Tailscale](https://tailscale.com) network. This enables secure access from anywhere on your tailnet without port forwarding.

### Setup

1. Get an auth key from [Tailscale Admin Console](https://login.tailscale.com/admin/settings/keys)

2. Configure `config.yaml`:

```yaml
tailscale:
  enabled: true
  hostname: "coven-gateway"    # Becomes coven-gateway.tailnet-xxx.ts.net
  auth_key: "${TS_AUTHKEY}"   # Or set via environment
  ephemeral: false            # Keep node after restart
  funnel: false               # Expose HTTP publicly (optional)
```

3. Start the gateway:

```bash
export TS_AUTHKEY="tskey-auth-xxx"
./bin/coven-gateway serve
```

The gateway will:
- Join your tailnet as `coven-gateway`
- Listen for gRPC on `:50051` (tailnet only)
- Listen for HTTP on `:80` (or `:443` with Funnel)
- Log its MagicDNS name and Tailscale IP

### Connecting Agents via Tailscale

```bash
# Agent connects using MagicDNS name
cargo run -p coven-agent -- \
  --server http://coven-gateway.tailnet-xxx.ts.net:50051 \
  --name "remote-agent"
```

### Connecting Clients via Tailscale

```bash
# TUI connects using MagicDNS
./bin/coven-tui -server http://coven-gateway.tailnet-xxx.ts.net

# Or use Tailscale IP
./bin/coven-tui -server http://100.x.y.z
```

### Funnel (Public Access)

Enable [Tailscale Funnel](https://tailscale.com/kb/1223/funnel) to expose the HTTP API publicly:

```yaml
tailscale:
  enabled: true
  hostname: "coven-gateway"
  funnel: true  # Exposes https://coven-gateway.tailnet-xxx.ts.net publicly
```

**Note:** Funnel requires HTTPS and only works on ports 443, 8443, or 10000. gRPC remains tailnet-only.

## Architecture

```
coven-gateway/
├── cmd/
│   ├── coven-gateway/     # Gateway server binary
│   └── coven-tui/         # Interactive TUI client
├── internal/
│   ├── agent/            # Agent connection management
│   │   ├── manager.go    # Connection registry, message routing, and channel bindings
│   │   └── connection.go # Single agent stream handler
│   ├── config/           # YAML configuration loading
│   ├── gateway/          # Main orchestrator
│   │   ├── gateway.go    # Server lifecycle management
│   │   ├── grpc.go       # CovenControl gRPC service
│   │   └── api.go        # HTTP API handlers
│   └── store/            # SQLite persistence
├── proto/coven/          # Generated protobuf code
├── docs/
│   ├── AGENT_PROTOCOL.md # gRPC protocol for agents
│   └── CLIENT_PROTOCOL.md # HTTP API for clients
└── config.example.yaml
```

## Protocol Documentation

- **[Agent Protocol](docs/AGENT_PROTOCOL.md)**: gRPC bidirectional streaming protocol for coven-agents
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

Requires the coven-agent repo checked out as a sibling:

```bash
# Install protoc plugins (one time)
make proto-deps

# Regenerate proto
make proto
```

### Makefile Targets

| Target | Description |
|--------|-------------|
| `make` | Generate proto + build all binaries (default) |
| `make build` | Build all binaries |
| `make build-gateway` | Build coven-gateway only |
| `make build-tui` | Build coven-tui only |
| `make build-admin` | Build coven-admin only |
| `make proto` | Generate protobuf code |
| `make proto-deps` | Install protoc plugins (one-time) |
| `make test` | Run all tests |
| `make lint` | Run golangci-lint |
| `make fmt` | Format code with go fmt |
| `make clean` | Remove build artifacts |
| `make run` | Build and run gateway |
| `make dev` | Development mode instructions |

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
let mut client = CovenControlClient::new(channel);

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

- [coven-agent](https://github.com/2389-research/coven-agent) - The coven agent framework (Rust)
- [mux](https://github.com/2389-research/mux-rs) - Claude API streaming library
