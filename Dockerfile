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

# Stage 2: Runtime
FROM debian:bookworm-slim

# Install runtime dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN useradd --create-home --shell /bin/bash app

WORKDIR /app

# Copy binary from builder
COPY --from=builder /fold-gateway /usr/local/bin/fold-gateway

# Copy example config
COPY config.example.yaml /app/config.example.yaml

# Create data directory for SQLite
RUN mkdir -p /app/data && chown -R app:app /app

USER app

# Expose gRPC and HTTP ports
EXPOSE 50051 8080

# Default command
CMD ["fold-gateway", "serve", "--config", "/app/config.yaml"]
