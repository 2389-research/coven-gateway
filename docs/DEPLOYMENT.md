# Deployment Guide

This guide covers deploying coven-gateway to production environments.

## Overview

coven-gateway is a single binary with an embedded SQLite database. It can run:
- Directly on a server or VM
- In a container (Docker/Podman)
- On a Tailscale network with automatic TLS

## Quick Deployment

### Single Server

```bash
# Download or build the binary
make build

# Create data directory
mkdir -p /var/lib/coven-gateway

# Copy config
cp config.example.yaml /etc/coven/gateway.yaml

# Edit configuration
vim /etc/coven/gateway.yaml

# Create initial admin
./bin/coven-gateway bootstrap --name "Admin"

# Run
./bin/coven-gateway serve --config /etc/coven/gateway.yaml
```

### Systemd Service

Create `/etc/systemd/system/coven-gateway.service`:

```ini
[Unit]
Description=Coven Gateway
After=network.target

[Service]
Type=simple
User=coven
ExecStart=/usr/local/bin/coven-gateway serve --config /etc/coven/gateway.yaml
Restart=on-failure
RestartSec=5
Environment=COVEN_JWT_SECRET=your-secret-here

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable coven-gateway
sudo systemctl start coven-gateway
```

## Configuration

### Production Config Example

```yaml
server:
  grpc_addr: "0.0.0.0:50051"
  http_addr: "0.0.0.0:8080"

database:
  path: "/var/lib/coven-gateway/gateway.db"

auth:
  jwt_secret: "${COVEN_JWT_SECRET}"  # 32+ bytes, never commit!
  agent_auto_registration: "pending" # Require approval in production

agents:
  heartbeat_interval: "30s"
  heartbeat_timeout: "90s"
  reconnect_grace_period: "5m"

logging:
  level: "info"
  format: "json"  # JSON for log aggregation
```

### Environment Variables

| Variable | Required | Purpose |
|----------|----------|---------|
| `COVEN_JWT_SECRET` | Yes | JWT signing secret (32+ bytes) |
| `COVEN_CONFIG` | No | Config file path override |
| `TS_AUTHKEY` | If Tailscale | Tailscale auth key |

Generate a secure JWT secret:
```bash
openssl rand -base64 32
```

## Database

### File Location

Choose a persistent location for the SQLite database:
- `/var/lib/coven-gateway/gateway.db` (recommended)
- User data dir: `~/.local/share/coven/gateway.db`

### Backups

SQLite with WAL mode allows hot backups:

```bash
# Safe backup while gateway is running
sqlite3 /var/lib/coven-gateway/gateway.db ".backup /backup/gateway.db"
```

Or use filesystem snapshots if available.

### Migration

Migrations run automatically on startup. The gateway handles schema versioning internally.

## Tailscale Deployment

Tailscale provides automatic TLS and secure networking without port forwarding.

### Configuration

```yaml
tailscale:
  enabled: true
  hostname: "coven-gateway"  # Becomes coven-gateway.your-tailnet.ts.net
  auth_key: "${TS_AUTHKEY}"
  state_dir: "/var/lib/coven-gateway/tailscale"
  ephemeral: false
  https: true
  funnel: false  # Set true for public access
```

### Getting an Auth Key

1. Go to https://login.tailscale.com/admin/settings/keys
2. Generate a key (reusable if needed)
3. Set as `TS_AUTHKEY` environment variable

### With Funnel (Public Access)

Funnel exposes the gateway publicly via Tailscale's CDN:

```yaml
tailscale:
  enabled: true
  https: true
  funnel: true
```

Requires Funnel to be enabled in your tailnet ACLs.

## Container Deployment

### Dockerfile

```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY . .
RUN make build

FROM alpine:latest
COPY --from=builder /app/bin/coven-gateway /usr/local/bin/
COPY config.example.yaml /etc/coven/gateway.yaml
VOLUME /var/lib/coven-gateway
EXPOSE 50051 8080
CMD ["coven-gateway", "serve", "--config", "/etc/coven/gateway.yaml"]
```

### Docker Compose

```yaml
version: "3.8"
services:
  gateway:
    image: coven-gateway:latest
    ports:
      - "50051:50051"
      - "8080:8080"
    volumes:
      - coven-data:/var/lib/coven-gateway
      - ./config.yaml:/etc/coven/gateway.yaml:ro
    environment:
      - COVEN_JWT_SECRET=${COVEN_JWT_SECRET}
    restart: unless-stopped

volumes:
  coven-data:
```

## TLS Configuration

### With Tailscale

Tailscale provides automatic TLS certificates when `https: true`.

### Without Tailscale

Put a reverse proxy (nginx, Caddy, Traefik) in front of the gateway:

```nginx
server {
    listen 443 ssl http2;
    server_name coven.example.com;

    ssl_certificate /etc/ssl/coven.crt;
    ssl_certificate_key /etc/ssl/coven.key;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

For gRPC:
```nginx
server {
    listen 50051 ssl http2;
    server_name coven.example.com;

    ssl_certificate /etc/ssl/coven.crt;
    ssl_certificate_key /etc/ssl/coven.key;

    location / {
        grpc_pass grpc://127.0.0.1:50051;
    }
}
```

## Health Checks

The gateway exposes health endpoints:

```bash
# Liveness (is the server running?)
curl http://localhost:8080/health

# Readiness (is the server ready to serve?)
curl http://localhost:8080/health/ready
```

Use these for container orchestration and load balancer health checks.

## Security Checklist

- [ ] Generate strong JWT secret (32+ bytes)
- [ ] Never commit secrets to version control
- [ ] Use `pending` agent registration in production
- [ ] Enable TLS (Tailscale or reverse proxy)
- [ ] Restrict network access to gRPC port
- [ ] Regular database backups
- [ ] Monitor logs for errors

## Troubleshooting

### Gateway won't start

```bash
# Check config syntax
./bin/coven-gateway config-check

# Check port availability
lsof -i :8080
lsof -i :50051
```

### Database locked

- Only run one gateway instance per database file
- Don't use network filesystems (NFS, SMB)

### Agents can't connect

1. Verify gRPC port is accessible: `nc -zv hostname 50051`
2. Check agent registration mode
3. Review gateway logs for connection attempts

### High memory usage

- Large number of active conversations
- Restart gateway to clear in-memory caches
- Check for memory leaks (report as bug)

## Monitoring

### Logs

JSON format recommended for production:

```yaml
logging:
  level: "info"
  format: "json"
```

Filter with jq:
```bash
journalctl -u coven-gateway | jq 'select(.level == "ERROR")'
```

### Metrics

Prometheus metrics endpoint is planned but not yet implemented.

Current monitoring options:
- Health check endpoints
- Log aggregation
- Database file size
- Process monitoring (CPU, memory)
