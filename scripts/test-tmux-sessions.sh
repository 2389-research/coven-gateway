#!/bin/bash
# Spin up tmux sessions running claude-sim for adapter testing.
# Usage: ./scripts/test-tmux-sessions.sh [start|stop|status]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(dirname "$SCRIPT_DIR")"
SIM_BIN="$REPO_ROOT/bin/claude-sim"
SESSION_PREFIX="claude-test"

start() {
    if [[ ! -x "$SIM_BIN" ]]; then
        echo "Building claude-sim..."
        (cd "$REPO_ROOT" && go build -o bin/claude-sim ./cmd/claude-sim/)
    fi

    echo "Starting test tmux sessions..."

    # Session 1: simulating work on coven-gateway
    tmux new-session -d -s "${SESSION_PREFIX}-gateway" \
        -x 200 -y 50 \
        "cd $REPO_ROOT && $SIM_BIN"
    echo "  ✓ ${SESSION_PREFIX}-gateway"

    # Session 2: simulating work on a frontend project
    tmux new-session -d -s "${SESSION_PREFIX}-frontend" \
        -x 200 -y 50 \
        "cd $REPO_ROOT/web && $SIM_BIN"
    echo "  ✓ ${SESSION_PREFIX}-frontend"

    # Session 3: simulating work on docs
    tmux new-session -d -s "${SESSION_PREFIX}-docs" \
        -x 200 -y 50 \
        "cd $REPO_ROOT/docs && $SIM_BIN"
    echo "  ✓ ${SESSION_PREFIX}-docs"

    echo ""
    echo "Sessions running:"
    tmux list-sessions -F "  #{session_name} (#{session_windows} windows, created #{t:session_created})" \
        | grep "$SESSION_PREFIX" || true
    echo ""
    echo "To attach: tmux attach -t ${SESSION_PREFIX}-gateway"
    echo "To stop:   $0 stop"
}

stop() {
    echo "Stopping test tmux sessions..."
    for sess in $(tmux list-sessions -F "#{session_name}" 2>/dev/null | grep "^${SESSION_PREFIX}" || true); do
        tmux kill-session -t "$sess" 2>/dev/null && echo "  ✗ $sess" || true
    done
    echo "Done."
}

status() {
    echo "Test tmux sessions:"
    tmux list-panes -a -F "  session=#{session_name}  pane=#{pane_id}  cmd=#{pane_current_command}  pid=#{pane_pid}  cwd=#{pane_current_path}  dead=#{pane_dead}" 2>/dev/null \
        | grep "$SESSION_PREFIX" || echo "  (none running)"
}

case "${1:-start}" in
    start)  start ;;
    stop)   stop ;;
    status) status ;;
    *)      echo "Usage: $0 [start|stop|status]"; exit 1 ;;
esac
