# coven-tmux-adapter

Bridge existing Claude Code sessions running in tmux into coven-gateway as agents.

## How It Works

```text
┌──────────────────────────────────────────────────────────┐
│  tmux                                                    │
│  ┌─────────────────┐  ┌─────────────────┐               │
│  │ claude (pane %0) │  │ claude (pane %1) │  ...         │
│  └────────┬────────┘  └────────┬────────┘               │
│           │ pipe-pane / send-keys                        │
└───────────┼─────────────────────┼────────────────────────┘
            │                     │
     ┌──────▼─────────────────────▼──────┐
     │       coven-tmux-adapter          │
     │  discovers panes, bridges I/O     │
     └──────────────┬────────────────────┘
                    │ gRPC (bidirectional stream)
             ┌──────▼──────┐
             │coven-gateway│
             └─────────────┘
```

The adapter:

1. Scans tmux for panes running `claude` or `claude-sim`
2. Registers each pane as a coven agent (ID: `tmux-{session}-{pane}`)
3. Forwards messages from the gateway to the pane via `send-keys`
4. Captures terminal output via `pipe-pane` and streams it back as response events
5. Detects response boundaries (thinking → text → horizontal rule → prompt)
6. Re-scans periodically to pick up new sessions and clean up dead ones

## Prerequisites

- Go 1.22+ (to build)
- tmux (the adapter talks to tmux via its CLI)
- A running coven-gateway instance
- One or more tmux sessions running `claude` (Claude Code CLI)

## Quick Start

```bash
# Build everything
make build

# Terminal 1: start gateway (no auth for local use)
./bin/coven-gateway serve

# Terminal 2: start Claude Code in a tmux session
tmux new-session -s my-project
claude  # starts Claude Code in the pane

# Terminal 3: start the adapter
./bin/coven-tmux-adapter --gateway localhost:50051 --verbose

# Terminal 4: send a message to your Claude session
curl -N -X POST http://localhost:8080/api/send \
  -H "Content-Type: application/json" \
  -d '{"agent_id": "tmux-my-project-0", "sender": "me", "content": "what files are in this repo?"}'
```

## CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--gateway` | `localhost:50051` | coven-gateway gRPC address |
| `--scan-interval` | `10s` | How often to re-scan tmux for new/dead sessions |
| `--one-shot` | `false` | Discover once and block (no re-scanning) |
| `--verbose` | `false` | Enable debug logging |

## Agent ID Format

Each discovered pane gets an agent ID: `tmux-{session_name}-{pane_number}`

- `tmux-my-project-0` — session "my-project", pane %0
- `tmux-work-docs-3` — session "work-docs", pane %3

Use this ID in the `agent_id` field when sending messages via the gateway API.

To find your agent IDs, check the adapter's startup output:

```
coven-tmux-adapter: 2 session(s) bridged to localhost:50051
```

Or use `--verbose` to see each discovered session:

```
level=INFO msg="new session discovered, starting bridge" agent_id=tmux-my-project-0 session=my-project binary=claude cwd=/home/user/my-project
```

## Sending Messages

Use the gateway's HTTP API. The response streams back as Server-Sent Events:

```bash
curl -N -X POST http://localhost:8080/api/send \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "tmux-my-project-0",
    "sender": "user",
    "content": "explain the main function"
  }'
```

SSE events you'll see:

```
event: started
data: {"thread_id":"..."}

event: thinking
data: {"text":"Processing..."}

event: text
data: {"text":"The main function initializes..."}

event: text
data: {"text":"It then starts the HTTP server..."}

event: done
data: {"full_response":"The main function initializes...\nIt then starts the HTTP server..."}
```

## Authentication

The adapter connects to the gateway's gRPC port without authentication. This works when:

- `COVEN_JWT_SECRET` is **not set** (gateway runs in no-auth mode)
- You're running locally or on a trusted network

For production use with auth enabled, the adapter would need SSH key or token authentication (not yet implemented).

## Testing with claude-sim

The `claude-sim` binary simulates a Claude Code session for testing without a real API key:

```bash
# Build
make build

# Start a simulator in tmux
tmux new-session -d -s test-sim ./bin/claude-sim

# Start the adapter
./bin/coven-tmux-adapter --gateway localhost:50051 --verbose

# Send a test message
curl -N -X POST http://localhost:8080/api/send \
  -H "Content-Type: application/json" \
  -d '{"agent_id": "tmux-test-sim-0", "sender": "test", "content": "hello"}'
```

## Smoke Test

A full end-to-end smoke test is included:

```bash
./scripts/smoke-test-adapter.sh
```

This builds binaries, starts an isolated gateway (ports 50151/8180), creates a tmux session with claude-sim, starts the adapter, sends a message, and verifies the SSE response. It cleans up everything on exit.

## How Response Detection Works

The adapter watches terminal output through tmux's `pipe-pane` and uses a state machine (ResponseTracker) to detect response boundaries:

```
Idle → [sees "Thinking..."] → Thinking → [sees content] → Responding → [sees ────── + ❯] → Done
```

- **Input echo filtering**: The message you sent appears in the terminal as echo — it's filtered out
- **Spinner filtering**: Claude's `⠋ Thinking...` animation frames are ignored
- **Horizontal rule**: Claude prints `─────────` between the response and the next prompt — this marks the end
- **Busy detection**: If Claude doesn't start thinking within 30 seconds, the adapter returns an error instead of hanging

## Troubleshooting

**Adapter discovers no sessions**

Check that your tmux panes are actually running `claude` or `claude-sim`:

```bash
tmux list-panes -a -F '#{session_name} #{pane_id} #{pane_current_command}'
```

The adapter looks for panes whose current command resolves to a binary named `claude` or `claude-sim`.

**Agent registers but messages get no response**

- Check `--verbose` output for pipe-pane errors
- Verify Claude is at the `❯` prompt (not mid-operation)
- The 30s busy detection will return an error if Claude doesn't respond

**"claude appears busy" error**

Claude was already processing something when the adapter sent input. Wait for Claude to finish its current operation (look for the `❯` prompt) and try again.

**Agent ID shows "already registered"**

The gateway still thinks the previous adapter instance is connected. The adapter retries with exponential backoff (up to 10 retries, 30s max backoff). If it persists, restart the gateway to clear stale connections.
