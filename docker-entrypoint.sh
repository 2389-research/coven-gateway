#!/bin/bash
# ABOUTME: Docker entrypoint that fixes volume permissions before starting
# ABOUTME: Runs as root initially, then drops to app user

set -e

# Fix ownership of mounted volumes (they may be created as root)
chown -R app:app /app/data /app/tailscale 2>/dev/null || true

# Drop to app user and exec the command
exec gosu app "$@"
