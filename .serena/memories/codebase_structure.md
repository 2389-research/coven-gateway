# Codebase Structure

## Directory Layout
```
coven-gateway/
├── cmd/                    # Main packages for binaries
│   ├── coven-admin/        # Admin CLI tool
│   ├── coven-gateway/      # Main gateway server
│   └── coven-tui/          # Terminal UI client
├── internal/               # Private packages
│   ├── agent/              # Agent connection management
│   │   ├── manager.go      # Tracks all connected agents
│   │   └── connection.go   # Individual agent stream handling
│   ├── api/                # HTTP API handlers
│   ├── auth/               # Authentication (JWT, SSH, WebAuthn)
│   │   ├── interceptor.go  # gRPC auth interceptors
│   │   ├── ssh.go          # SSH key signature verification
│   │   └── webauthn.go     # Passkey authentication
│   ├── gateway/            # Core orchestrator
│   │   ├── gateway.go      # Main Gateway struct, setup
│   │   ├── api.go          # HTTP handlers
│   │   ├── mcp.go          # MCP server integration
│   │   └── webadmin.go     # Web admin interface
│   ├── mcp/                # Model Context Protocol
│   ├── router/             # Message routing
│   ├── store/              # Database layer
│   │   ├── store.go        # Interface definitions
│   │   ├── sqlite.go       # SQLite implementation
│   │   └── mock_store.go   # In-memory mock for tests
│   └── toolpack/           # Tool pack system
├── proto/                  # Protobuf definitions
│   └── coven.proto         # Shared with coven-agent
├── web/                    # Embedded web assets
└── Makefile               # Build automation
```

## Key Files
- `internal/gateway/gateway.go` - Central orchestrator (705 lines)
- `internal/agent/manager.go` - Agent tracking (493 lines)
- `internal/store/store.go` - Store interface (238 lines)
- `internal/auth/interceptor.go` - Auth middleware (307 lines)

## Message Flow
1. HTTP client POSTs to `/api/send`
2. `api.go` looks up agent (via binding or direct ID)
3. `Manager.SendMessage` sends via agent's gRPC stream
4. `Connection` correlates responses by request_id
5. `api.go` streams responses as SSE events
