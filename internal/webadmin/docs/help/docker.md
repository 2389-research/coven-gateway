# Docker Deployment

Run coven-gateway in containers for easy deployment and scaling.

## Quick Start

### 1. Create Configuration

```yaml
# config.yaml
tailscale:
  enabled: true
  hostname: "coven-gateway"
  auth_key: "${TS_AUTHKEY}"
  state_dir: "/app/tailscale"
  https: true

database:
  path: "/app/data/gateway.db"

auth:
  jwt_secret: "${COVEN_JWT_SECRET}"
  agent_auto_registration: "approved"

logging:
  level: "info"
  format: "json"
```

### 2. Create docker-compose.yml

```yaml
version: "3.8"

services:
  coven-gateway:
    image: ghcr.io/2389-research/coven-gateway:latest
    environment:
      - TS_AUTHKEY=${TS_AUTHKEY}
      - COVEN_JWT_SECRET=${COVEN_JWT_SECRET}
    volumes:
      - ./config.yaml:/app/config.yaml:ro
      - coven-data:/app/data
      - coven-tailscale:/app/tailscale
    restart: unless-stopped

volumes:
  coven-data:
  coven-tailscale:
```

### 3. Set Environment Variables

```bash
export TS_AUTHKEY="tskey-auth-xxxxx"
export COVEN_JWT_SECRET=$(openssl rand -base64 32)
```

### 4. Start the Gateway

```bash
docker-compose up -d
```

## Building from Source

```bash
# Build the image
docker build -t coven-gateway .

# Or with docker-compose
docker-compose build
```

## Volume Mounts

| Path | Purpose | Required |
|------|---------|----------|
| `/app/config.yaml` | Configuration file | Yes |
| `/app/data` | SQLite database | Yes (persist data) |
| `/app/tailscale` | Tailscale state | Yes (if using Tailscale) |

**Important**: Mount `/app/tailscale` as a volume to persist Tailscale state across restarts. Without this, you'll need a new auth key each time.

## Environment Variables

Pass secrets via environment variables:

```yaml
environment:
  - TS_AUTHKEY=${TS_AUTHKEY}
  - COVEN_JWT_SECRET=${COVEN_JWT_SECRET}
  - SLACK_APP_TOKEN=${SLACK_APP_TOKEN}
  - SLACK_BOT_TOKEN=${SLACK_BOT_TOKEN}
```

## Without Tailscale

For local or internal deployments:

```yaml
# config.yaml
server:
  grpc_addr: "0.0.0.0:50051"
  http_addr: "0.0.0.0:8080"

database:
  path: "/app/data/gateway.db"
```

```yaml
# docker-compose.yml
services:
  coven-gateway:
    image: coven-gateway
    ports:
      - "50051:50051"  # gRPC
      - "8080:8080"    # HTTP
    volumes:
      - ./config.yaml:/app/config.yaml:ro
      - coven-data:/app/data
```

## Health Checks

```yaml
services:
  coven-gateway:
    # ...
    healthcheck:
      test: ["CMD", "coven-gateway", "health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 10s
```

## Logging

For container environments, use JSON logging:

```yaml
logging:
  level: "info"
  format: "json"
```

View logs:

```bash
docker-compose logs -f coven-gateway
```

## Matrix Bridge Container

The Matrix bridge runs as a separate container:

```yaml
services:
  coven-gateway:
    # ... gateway config ...

  coven-matrix:
    image: ghcr.io/2389-research/coven-matrix:latest
    environment:
      - MATRIX_USERNAME=${MATRIX_USERNAME}
      - MATRIX_PASSWORD=${MATRIX_PASSWORD}
    volumes:
      - ./config.matrix.toml:/app/config.toml:ro
      - matrix-data:/app/data
    depends_on:
      - coven-gateway

volumes:
  matrix-data:
```

## Production Checklist

- [ ] Use named volumes for data persistence
- [ ] Set `restart: unless-stopped` or `always`
- [ ] Use JSON logging format
- [ ] Configure health checks
- [ ] Store secrets in environment variables (not config files)
- [ ] Use Tailscale for secure remote access
- [ ] Mount config as read-only (`:ro`)

## Resource Limits

```yaml
services:
  coven-gateway:
    # ...
    deploy:
      resources:
        limits:
          memory: 512M
        reservations:
          memory: 128M
```

## Updating

```bash
# Pull latest image
docker-compose pull

# Restart with new image
docker-compose up -d
```

## Backup

```bash
# Stop the gateway
docker-compose stop coven-gateway

# Backup the database
docker cp coven-gateway:/app/data/gateway.db ./backup-$(date +%Y%m%d).db

# Restart
docker-compose start coven-gateway
```

Or with volume mounts:

```bash
cp ./data/gateway.db ./backup-$(date +%Y%m%d).db
```
