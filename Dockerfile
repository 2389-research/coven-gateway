# ABOUTME: Multi-stage Dockerfile for fold-gateway
# ABOUTME: Builds a minimal container with CGO-enabled SQLite support

# Stage 1: Build
FROM golang:1.25-bookworm AS builder

WORKDIR /app

# Install build dependencies for CGO
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    libc6-dev \
    && rm -rf /var/lib/apt/lists/*

# Copy dependency files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build with CGO enabled for SQLite
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /fold-gateway ./cmd/fold-gateway
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /fold-admin ./cmd/fold-admin

# Stage 2: Runtime
FROM debian:bookworm-slim

# Install runtime dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    gosu \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN useradd --create-home --shell /bin/bash app

WORKDIR /app

# Copy binaries from builder
COPY --from=builder /fold-gateway /usr/local/bin/fold-gateway
COPY --from=builder /fold-admin /usr/local/bin/fold-admin

# Copy example config
COPY config.example.yaml /app/config.example.yaml

# Create data and tailscale directories
RUN mkdir -p /app/data /app/tailscale && chown -R app:app /app

# Copy entrypoint script
COPY docker-entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

# Expose gRPC and HTTP ports
EXPOSE 50051 8080

# Set config path via environment variable
ENV FOLD_CONFIG=/app/config.yaml

# Entrypoint fixes volume permissions then drops to app user
ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["fold-gateway", "serve"]
