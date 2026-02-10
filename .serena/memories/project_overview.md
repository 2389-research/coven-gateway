# Coven Gateway - Project Overview

## Purpose
coven-gateway is the production control plane for coven agents. It manages coven-agent connections via gRPC, routes messages from frontends (HTTP clients, Matrix, web UI) to agents, and streams responses back in real-time.

## Tech Stack
- **Language**: Go 1.21+
- **Database**: SQLite (via modernc.org/sqlite - pure Go)
- **Networking**: gRPC for agent connections, HTTP/SSE for clients
- **Tailscale**: tsnet for embedded Tailscale networking with HTTPS
- **Protocol**: Protobuf (shared with coven-agent Rust project)
- **Authentication**: JWT for clients, SSH key signatures for agents
- **Web**: WebAuthn passkeys, embedded SPA

## Related Projects
- `../coven-agent` - Rust agent that connects to this gateway
- Shared protobuf at `proto/coven.proto`

## Key Binaries
- `coven-gateway` - Main server (gRPC + HTTP + optional Tailscale)
- `coven-tui` - Interactive terminal client
- `coven-admin` - CLI for monitoring gateway status
