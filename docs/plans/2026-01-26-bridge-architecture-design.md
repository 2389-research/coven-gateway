# Bridge Architecture Design

Date: 2026-01-26

## Problem Statement

The current Matrix bridge (`cmd/coven-matrix/`) communicates with the gateway via HTTP POST + Server-Sent Events (SSE). This works but has drawbacks:

1. **Performance overhead** - HTTP/SSE requires JSON serialization, SSE parsing, and lacks proper backpressure
2. **Inconsistency** - Agents use gRPC bidirectional streaming; bridges use a different protocol
3. **Gateway coupling** - If bridges embed in the gateway, it becomes a platform client monolith instead of a clean router

As we add more bridges (Slack, Discord, WhatsApp, etc.), we need a scalable architecture.

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Protocol | gRPC (ClientService) | Eliminates HTTP/SSE overhead, consistent with agent protocol, typed contracts |
| Deployment | Separate processes | Gateway stays a pure router, bridges handle platform complexity independently |
| Language | Per-bridge (best library wins) | gRPC is the contract; implementation language doesn't matter |
| Configuration | Per-bridge config | Each bridge manages its own platform credentials and settings |
| Binding model | Channel → Agent | Unchanged from current design |

## Architecture

```
┌──────────────────┐
│  Matrix Bridge   │──┐
│     (Rust)       │  │
└──────────────────┘  │
┌──────────────────┐  │
│  Slack Bridge    │──┤      gRPC          ┌─────────────────┐      gRPC       ┌──────────────┐
│     (Go)         │  │   ClientService    │                 │   CovenControl  │              │
└──────────────────┘  ├───────────────────►│  coven-gateway  │◄────────────────│ coven-agent  │
┌──────────────────┐  │                    │     (router)    │                 │   (Rust)     │
│ WhatsApp Bridge  │──┤                    │                 │                 │              │
│  (TypeScript)    │  │                    └─────────────────┘                 └──────────────┘
└──────────────────┘  │                            │
┌──────────────────┐  │                            ▼
│  Discord Bridge  │──┤                     ┌─────────────┐
│   (Rust/Go)      │  │                     │   SQLite    │
└──────────────────┘  │                     │ (bindings,  │
┌──────────────────┐  │                     │  threads)   │
│   IRC Bridge     │──┘                     └─────────────┘
│     (???)        │
└──────────────────┘
```

### Gateway Role (Unchanged)

The gateway remains a **pure router**:

- Receives messages via gRPC (`ClientService.SendMessage`)
- Routes to bound agents via gRPC (`CovenControl`)
- Streams responses back to caller
- Persists conversations, bindings, threads
- No platform-specific code

### Bridge Role

Each bridge is a **platform adapter**:

- Connects to platform (Matrix, Slack, etc.)
- Authenticates with platform credentials
- Listens for messages in configured channels/rooms
- Translates platform events → gRPC `SendMessage` calls
- Translates gRPC `ConversationEvent` stream → platform messages
- Handles platform-specific formatting (Markdown → Slack blocks, HTML, etc.)
- Manages platform-specific features (typing indicators, threads, reactions)

## gRPC Contract

Bridges use the existing `ClientService` from `coven.proto`:

```protobuf
service ClientService {
  // Send a message and stream response events
  rpc SendMessage(ClientSendRequest) returns (stream ConversationEvent);

  // Stream events for a conversation
  rpc StreamEvents(StreamEventsRequest) returns (stream ConversationEvent);

  // List available agents
  rpc ListAgents(ListAgentsRequest) returns (ListAgentsResponse);
}

message ClientSendRequest {
  string thread_id = 1;
  string content = 2;
  string frontend = 3;      // "matrix", "slack", "discord", etc.
  string channel_id = 4;    // Platform-specific channel identifier
  string sender = 5;        // Platform user identifier
}

message ConversationEvent {
  oneof event {
    ThinkingEvent thinking = 1;
    TextEvent text = 2;
    ToolUseEvent tool_use = 3;
    ToolResultEvent tool_result = 4;
    DoneEvent done = 5;
    ErrorEvent error = 6;
  }
}
```

### Binding Management

Bridges use `AdminService` for binding management:

```protobuf
service AdminService {
  rpc CreateBinding(CreateBindingRequest) returns (CreateBindingResponse);
  rpc DeleteBinding(DeleteBindingRequest) returns (DeleteBindingResponse);
  rpc GetBinding(GetBindingRequest) returns (GetBindingResponse);
  rpc ListBindings(ListBindingsRequest) returns (ListBindingsResponse);
}
```

## Bridge Implementations

### Planned Bridges

| Platform | Language | Library | Notes |
|----------|----------|---------|-------|
| Matrix | Rust | matrix-sdk | Best-in-class Rust SDK, E2EE support |
| Slack | Go | slack-go | Mature, well-maintained |
| Google Chat | Go | Google Cloud SDK | Official SDK |
| WhatsApp | TypeScript | Baileys | De facto standard for unofficial WA |
| Discord | Rust or Go | serenity / discordgo | Both excellent options |
| IRC | TBD | TBD | Simple protocol, any language works |

### Bridge Structure (Example: Rust)

```
coven-matrix-bridge/
├── Cargo.toml
├── src/
│   ├── main.rs           # Entry point, config loading
│   ├── config.rs         # Configuration structs
│   ├── matrix.rs         # Matrix client, event handling
│   ├── gateway.rs        # gRPC client to gateway
│   └── formatter.rs      # Response → Matrix HTML conversion
└── config.example.toml
```

### Bridge Structure (Example: Go)

```
coven-slack-bridge/
├── go.mod
├── main.go               # Entry point, config loading
├── config.go             # Configuration structs
├── slack.go              # Slack client, event handling
├── gateway.go            # gRPC client to gateway
└── formatter.go          # Response → Slack blocks conversion
```

### Bridge Structure (Example: TypeScript)

```
coven-whatsapp-bridge/
├── package.json
├── src/
│   ├── index.ts          # Entry point
│   ├── config.ts         # Configuration
│   ├── whatsapp.ts       # Baileys client, event handling
│   ├── gateway.ts        # gRPC client to gateway
│   └── formatter.ts      # Response → WhatsApp formatting
└── config.example.json
```

## Configuration

Each bridge has its own configuration file. Example for Matrix:

```toml
# matrix-bridge.toml

[gateway]
address = "localhost:6666"
# auth_token = "..." # If gateway requires auth

[matrix]
homeserver = "https://matrix.org"
username = "@coven-bot:matrix.org"
password = "${MATRIX_PASSWORD}"  # Env var expansion

[bridge]
allowed_rooms = [
  "!abc123:matrix.org",
  "!def456:matrix.org"
]
command_prefix = "/coven"
typing_indicator = true

[crypto]  # Optional E2EE
enabled = false
recovery_key = "${MATRIX_RECOVERY_KEY}"
```

Example for Slack:

```toml
# slack-bridge.toml

[gateway]
address = "localhost:6666"

[slack]
bot_token = "${SLACK_BOT_TOKEN}"
app_token = "${SLACK_APP_TOKEN}"  # For Socket Mode

[bridge]
allowed_channels = ["C01234567", "C89ABCDEF"]
command_prefix = "/coven"
```

## Message Flow

### Inbound (Platform → Agent)

```
1. User sends message in Matrix room
2. Matrix bridge receives event via matrix-sdk sync
3. Bridge checks: allowed room? has binding?
4. Bridge calls gateway: ClientService.SendMessage(
     frontend: "matrix",
     channel_id: "!room:server",
     sender: "@user:server",
     content: "Hello agent"
   )
5. Gateway looks up binding: room → agent
6. Gateway forwards to agent via CovenControl stream
7. Agent processes, streams response events
8. Gateway streams ConversationEvents back to bridge
9. Bridge accumulates text, converts to HTML
10. Bridge sends formatted message to Matrix room
```

### Binding Management

```
1. User sends "/coven bind agent-123" in room
2. Bridge parses command
3. Bridge calls AdminService.CreateBinding(
     frontend: "matrix",
     channel_id: "!room:server",
     instance_id: "agent-123"
   )
4. Gateway creates binding in SQLite
5. Bridge confirms to user in room
```

## Deployment

### Docker Compose (Recommended)

```yaml
version: '3.8'

services:
  gateway:
    image: ghcr.io/2389/coven-gateway:latest
    ports:
      - "6666:6666"  # gRPC
      - "8080:8080"  # HTTP API
    volumes:
      - ./data:/data
      - ./config/gateway.yaml:/etc/coven/gateway.yaml

  matrix-bridge:
    image: ghcr.io/2389/coven-matrix-bridge:latest
    environment:
      - GATEWAY_ADDRESS=gateway:6666
      - MATRIX_PASSWORD=${MATRIX_PASSWORD}
    volumes:
      - ./config/matrix-bridge.toml:/etc/coven/matrix-bridge.toml
    depends_on:
      - gateway

  slack-bridge:
    image: ghcr.io/2389/coven-slack-bridge:latest
    environment:
      - GATEWAY_ADDRESS=gateway:6666
      - SLACK_BOT_TOKEN=${SLACK_BOT_TOKEN}
      - SLACK_APP_TOKEN=${SLACK_APP_TOKEN}
    volumes:
      - ./config/slack-bridge.toml:/etc/coven/slack-bridge.toml
    depends_on:
      - gateway
```

### Systemd (Alternative)

```ini
# /etc/systemd/system/coven-matrix-bridge.service
[Unit]
Description=Coven Matrix Bridge
After=coven-gateway.service

[Service]
ExecStart=/usr/local/bin/coven-matrix-bridge --config /etc/coven/matrix-bridge.toml
Restart=always
Environment=MATRIX_PASSWORD=...

[Install]
WantedBy=multi-user.target
```

## Migration Path

### Phase 1: Rewrite Matrix Bridge in Rust

1. Create `coven-matrix-bridge` crate in coven monorepo
2. Implement using matrix-sdk + tonic (gRPC)
3. Test against gateway's ClientService
4. Deploy alongside existing Go bridge
5. Deprecate `cmd/coven-matrix/`

### Phase 2: Add Slack Bridge

1. Create `coven-slack-bridge` Go module
2. Implement using slack-go + grpc-go
3. Deploy and test

### Phase 3: Additional Bridges

Add bridges as needed based on user demand.

## Security Considerations

### Bridge Authentication

Bridges should authenticate to the gateway. Options:

1. **mTLS** - Bridges present client certificates
2. **API tokens** - Bridges include token in gRPC metadata
3. **SSH keys** - Consistent with agent auth pattern

Recommend: SSH key authentication for consistency with agents.

### Platform Credentials

- Store in environment variables, not config files
- Use secret management (Vault, AWS Secrets Manager) in production
- Never log credentials

### Channel Authorization

- Bridges should only listen to explicitly allowed channels
- Gateway should validate `frontend` + `channel_id` against binding

## Open Questions

1. **Bridge registration** - Should bridges register with gateway like agents do? Would enable monitoring, health checks.

2. **Multi-tenant** - Could one bridge serve multiple gateway instances? Probably not needed.

3. **Rate limiting** - Should gateway rate-limit bridge requests? Platform rate limits are usually the bottleneck.

## Summary

This design keeps the gateway as a clean router while allowing bridges to be written in whatever language best suits each platform. The gRPC contract (ClientService) provides a consistent, performant interface. Bridges are independently deployable, scalable, and maintainable.
