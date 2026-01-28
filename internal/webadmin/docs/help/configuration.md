# Configuration Reference

Complete reference for `gateway.yaml` configuration options.

## File Format

Configuration uses YAML format with environment variable expansion:

```yaml
auth:
  jwt_secret: "${COVEN_JWT_SECRET}"  # Expands from environment
```

## Server Settings

```yaml
server:
  grpc_addr: "0.0.0.0:50051"   # Agent connections (gRPC)
  http_addr: "0.0.0.0:8080"    # API and admin UI (HTTP)
```

When Tailscale is enabled, these are optional (Tailscale handles networking).

## Database

```yaml
database:
  path: "/path/to/gateway.db"
```

Supports:
- Absolute paths: `/app/data/gateway.db`
- Relative paths: `./gateway.db`
- In-memory (testing): `:memory:`

## Authentication

```yaml
auth:
  jwt_secret: "${COVEN_JWT_SECRET}"
  agent_auto_registration: "approved"
```

### JWT Secret

- Minimum 32 bytes, base64 encoded
- Generate with: `openssl rand -base64 32`
- **Never commit to version control** - use environment variables

### Agent Auto-Registration

| Value | Behavior |
|-------|----------|
| `approved` | New agents can receive messages immediately |
| `pending` | New agents require admin approval |
| `disabled` | Unknown agents are rejected |

## Tailscale Integration

```yaml
tailscale:
  enabled: false
  hostname: "coven-gateway"
  auth_key: "${TS_AUTHKEY}"
  state_dir: ""
  ephemeral: false
  https: true
  funnel: false
```

| Option | Description |
|--------|-------------|
| `enabled` | Enable Tailscale networking |
| `hostname` | Tailnet hostname (becomes `hostname.tailnet.ts.net`) |
| `auth_key` | From [Tailscale admin](https://login.tailscale.com/admin/settings/keys) |
| `state_dir` | Where to store Tailscale state (default: `~/.local/share/coven-gateway/tailscale`) |
| `ephemeral` | Remove from tailnet on shutdown |
| `https` | Auto-provision TLS certificates |
| `funnel` | Expose publicly via Tailscale Funnel |

## Agent Timing

```yaml
agents:
  heartbeat_interval: "30s"
  heartbeat_timeout: "90s"
  reconnect_grace_period: "5m"
```

Duration format: Go syntax (`30s`, `1m30s`, `2h`)

## Logging

```yaml
logging:
  level: "info"    # debug, info, warn, error
  format: "text"   # text (colorized) or json
```

## Metrics

```yaml
metrics:
  enabled: true
  path: "/metrics"
```

Prometheus-compatible metrics endpoint.

## Web Admin

```yaml
webadmin:
  base_url: ""
```

Auto-detected if empty. Set explicitly for:
- Reverse proxy setups
- Custom domain names
- Tailscale with Funnel

## Frontend Integrations

### Slack

```yaml
frontends:
  slack:
    enabled: false
    app_token: "${SLACK_APP_TOKEN}"    # xapp-...
    bot_token: "${SLACK_BOT_TOKEN}"    # xoxb-...
    allowed_channels: []               # Empty = all
```

### Matrix (Embedded)

```yaml
frontends:
  matrix:
    enabled: false
    homeserver: "https://matrix.org"
    user_id: "@bot:matrix.org"
    access_token: "${MATRIX_ACCESS_TOKEN}"
    allowed_users: []
    allowed_rooms: []
```

## Complete Example

```yaml
server:
  grpc_addr: "0.0.0.0:50051"
  http_addr: "0.0.0.0:8080"

database:
  path: "~/.local/share/coven/gateway.db"

auth:
  jwt_secret: "${COVEN_JWT_SECRET}"
  agent_auto_registration: "approved"

agents:
  heartbeat_interval: "30s"
  heartbeat_timeout: "90s"
  reconnect_grace_period: "5m"

logging:
  level: "info"
  format: "text"

metrics:
  enabled: true
  path: "/metrics"
```

## Environment Variables

| Variable | Purpose |
|----------|---------|
| `COVEN_CONFIG` | Config file path override |
| `COVEN_JWT_SECRET` | JWT signing secret |
| `TS_AUTHKEY` | Tailscale auth key |
| `SLACK_APP_TOKEN` | Slack app token |
| `SLACK_BOT_TOKEN` | Slack bot token |
| `MATRIX_ACCESS_TOKEN` | Matrix access token |
