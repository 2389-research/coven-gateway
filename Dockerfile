# ABOUTME: Multi-stage Dockerfile for coven-gateway
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
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /coven-gateway ./cmd/coven-gateway
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /coven-admin ./cmd/coven-admin

# Stage 2: Runtime
FROM debian:bookworm-slim

# Install runtime dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy binaries from builder
COPY --from=builder /coven-gateway /usr/local/bin/coven-gateway
COPY --from=builder /coven-admin /usr/local/bin/coven-admin

# Copy example config
COPY config.example.yaml /app/config.example.yaml

# Create data and tailscale directories
RUN mkdir -p /app/data /app/tailscale

# Expose gRPC and HTTP ports
EXPOSE 50051 8080

# Set config path via environment variable
ENV COVEN_CONFIG=/app/config.yaml

CMD ["coven-gateway", "serve"]
