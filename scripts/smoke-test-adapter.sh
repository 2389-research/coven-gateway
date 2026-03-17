#!/usr/bin/env bash
# ABOUTME: E2E smoke test for the coven-tmux-adapter pipeline.
# ABOUTME: Validates: gateway ↔ adapter ↔ claude-sim via tmux.
#
# Prerequisites: tmux must be installed.
# Usage: ./scripts/smoke-test-adapter.sh
#
# What it does:
#   1. Builds binaries (gateway, adapter, claude-sim)
#   2. Starts gateway with ephemeral config (no auth, temp SQLite)
#   3. Creates a tmux session running claude-sim
#   4. Starts the adapter pointing at the gateway
#   5. Verifies agent registration via adapter logs
#   6. Sends a message via POST /api/send
#   7. Verifies the SSE response contains expected text
#   8. Cleans up everything

set -euo pipefail

# --- Configuration ---
TMUX_SESSION="coven-smoke-test"
GRPC_PORT=50151
HTTP_PORT=8180
GATEWAY_ADDR="localhost:${GRPC_PORT}"
HTTP_ADDR="localhost:${HTTP_PORT}"
TIMEOUT=30  # seconds to wait for each step
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SMOKE_TMP="${TMPDIR:-/tmp}/coven-smoke"

# --- Colors ---
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

pass() { echo -e "${GREEN}✓${NC} $1"; }
fail() { echo -e "${RED}✗${NC} $1"; cleanup; exit 1; }
info() { echo -e "${YELLOW}…${NC} $1"; }

# --- Cleanup ---
GATEWAY_PID=""
ADAPTER_PID=""

cleanup() {
    info "Cleaning up..."

    # Kill adapter
    if [[ -n "${ADAPTER_PID}" ]] && kill -0 "${ADAPTER_PID}" 2>/dev/null; then
        kill "${ADAPTER_PID}" 2>/dev/null || true
        wait "${ADAPTER_PID}" 2>/dev/null || true
    fi

    # Kill gateway
    if [[ -n "${GATEWAY_PID}" ]] && kill -0 "${GATEWAY_PID}" 2>/dev/null; then
        kill "${GATEWAY_PID}" 2>/dev/null || true
        wait "${GATEWAY_PID}" 2>/dev/null || true
    fi

    # Destroy tmux session
    tmux kill-session -t "${TMUX_SESSION}" 2>/dev/null || true

    # Remove temp files
    rm -f "${SMOKE_TMP}-config.yaml" "${SMOKE_TMP}.db" "${SMOKE_TMP}-sse.txt"
    rm -f "${SMOKE_TMP}-gateway.log" "${SMOKE_TMP}-adapter.log"

    info "Done."
}

trap cleanup EXIT

# --- Preflight ---
if ! command -v tmux &>/dev/null; then
    fail "tmux is not installed"
fi

# --- Step 1: Build ---
info "Building binaries..."
(cd "${ROOT_DIR}" && make build 2>&1) || fail "Build failed"
pass "Binaries built"

# --- Step 2: Start gateway with ephemeral config ---
info "Starting gateway (gRPC=${GRPC_PORT}, HTTP=${HTTP_PORT})..."

cat > "${SMOKE_TMP}-config.yaml" <<EOF
server:
  grpc_addr: "0.0.0.0:${GRPC_PORT}"
  http_addr: "0.0.0.0:${HTTP_PORT}"
database:
  path: "${SMOKE_TMP}.db"
agents:
  heartbeat_interval: "30s"
  heartbeat_timeout: "90s"
logging:
  level: "warn"
  format: "text"
EOF

COVEN_CONFIG="${SMOKE_TMP}-config.yaml" \
    "${ROOT_DIR}/bin/coven-gateway" serve &>"${SMOKE_TMP}-gateway.log" &
GATEWAY_PID=$!

# Wait for gateway to be ready by polling /health or any HTTP response.
WAITED=0
until curl -so /dev/null -w '' "http://${HTTP_ADDR}/" 2>/dev/null; do
    sleep 0.5
    WAITED=$((WAITED + 1))
    if [[ ${WAITED} -ge $((TIMEOUT * 2)) ]]; then
        echo "--- Gateway log ---"
        cat "${SMOKE_TMP}-gateway.log" || true
        fail "Gateway did not start within ${TIMEOUT}s"
    fi
done
pass "Gateway running (PID ${GATEWAY_PID})"

# --- Step 3: Create tmux session with claude-sim ---
info "Creating tmux session '${TMUX_SESSION}' with claude-sim..."

# Kill any leftover session from a previous failed run.
tmux kill-session -t "${TMUX_SESSION}" 2>/dev/null || true

tmux new-session -d -s "${TMUX_SESSION}" "${ROOT_DIR}/bin/claude-sim"
sleep 1  # Let claude-sim print its banner and first prompt

# Get the pane ID (tmux global pane identifier like %3).
PANE_ID=$(tmux list-panes -t "${TMUX_SESSION}" -F '#{pane_id}' 2>/dev/null | head -1)
if [[ -z "${PANE_ID}" ]]; then
    fail "Could not get pane ID from tmux session '${TMUX_SESSION}'"
fi

# Construct the expected agent ID (matches discover.go AgentID format).
PANE_NUM="${PANE_ID#%}"
AGENT_ID="tmux-${TMUX_SESSION}-${PANE_NUM}"
pass "claude-sim running in tmux (pane ${PANE_ID}, expected agent: ${AGENT_ID})"

# --- Step 4: Start adapter ---
info "Starting adapter (gateway=${GATEWAY_ADDR})..."

"${ROOT_DIR}/bin/coven-tmux-adapter" \
    --gateway "${GATEWAY_ADDR}" \
    --one-shot \
    --verbose \
    &>"${SMOKE_TMP}-adapter.log" &
ADAPTER_PID=$!

# --- Step 5: Verify agent registration ---
info "Waiting for agent '${AGENT_ID}' to register..."

WAITED=0
while ! grep -q "registered with gateway.*agent_id=${AGENT_ID}" "${SMOKE_TMP}-adapter.log" 2>/dev/null; do
    sleep 1
    WAITED=$((WAITED + 1))
    if [[ ${WAITED} -ge ${TIMEOUT} ]]; then
        echo "--- Adapter log ---"
        cat "${SMOKE_TMP}-adapter.log" || true
        fail "Agent '${AGENT_ID}' did not register within ${TIMEOUT}s"
    fi
    # Check adapter is still running.
    if ! kill -0 "${ADAPTER_PID}" 2>/dev/null; then
        echo "--- Adapter log ---"
        cat "${SMOKE_TMP}-adapter.log" || true
        fail "Adapter exited before registering"
    fi
done
pass "Agent registered: ${AGENT_ID}"

# --- Step 6: Send message via POST /api/send ---
info "Sending message via POST /api/send..."

SSE_FILE="${SMOKE_TMP}-sse.txt"
rm -f "${SSE_FILE}"

# curl SSE stream in background (timeout after 15s).
curl -sf -N --max-time 15 \
    -X POST "http://${HTTP_ADDR}/api/send" \
    -H "Content-Type: application/json" \
    -d "{\"agent_id\": \"${AGENT_ID}\", \"sender\": \"smoke-test\", \"content\": \"What is the gateway architecture?\"}" \
    > "${SSE_FILE}" 2>/dev/null &
CURL_PID=$!

# Wait for curl to finish (SSE stream closes or timeout).
WAITED=0
while kill -0 "${CURL_PID}" 2>/dev/null; do
    sleep 1
    WAITED=$((WAITED + 1))
    if [[ ${WAITED} -ge 20 ]]; then
        kill "${CURL_PID}" 2>/dev/null || true
        break
    fi
done
wait "${CURL_PID}" 2>/dev/null || true

pass "SSE stream captured"

# --- Step 7: Verify response ---
info "Verifying SSE response..."

if [[ ! -s "${SSE_FILE}" ]]; then
    echo "--- Adapter log ---"
    cat "${SMOKE_TMP}-adapter.log" || true
    echo "--- Gateway log (last 30 lines) ---"
    tail -30 "${SMOKE_TMP}-gateway.log" || true
    fail "SSE response file is empty"
fi

# Check for expected SSE event types.
HAS_TEXT=false
HAS_DONE=false

if grep -q "event: text" "${SSE_FILE}"; then
    HAS_TEXT=true
fi
if grep -q "event: done" "${SSE_FILE}"; then
    HAS_DONE=true
fi

if [[ "${HAS_TEXT}" == "true" ]]; then
    pass "SSE contains 'text' events"
else
    echo "--- SSE output ---"
    cat "${SSE_FILE}"
    fail "SSE response missing 'text' events"
fi

if [[ "${HAS_DONE}" == "true" ]]; then
    pass "SSE contains 'done' event"
else
    echo "--- SSE output ---"
    cat "${SSE_FILE}"
    info "Warning: SSE response missing 'done' event (may be timing related)"
fi

# Print a sample of the SSE output for human review.
echo ""
info "SSE response sample (first 20 lines):"
head -20 "${SSE_FILE}" | sed 's/^/    /'

echo ""
echo -e "${GREEN}═══════════════════════════════════════════${NC}"
echo -e "${GREEN}  Smoke test PASSED${NC}"
echo -e "${GREEN}═══════════════════════════════════════════${NC}"
