# Agent Identity and Client Flow Design

## Overview

This document defines the identity model for fold agents and the interaction model between clients (TUI/mobile) and agents. The goal is a "magical" experience where agents auto-register and clients can easily discover and communicate with them.

## Identity Model

### Concepts

| Concept | Example | Purpose |
|---------|---------|---------|
| **Principal** | UUID `a1b2c3d4-...` | Cryptographic identity (SSH key fingerprint), used for auth |
| **Identity name** | `"bob"` | Human-readable name from agent config, for display |
| **Instance name** | `"bob-projects-website"` | Identity + working_dir slug, unique per running agent |
| **Instance ID** | `a1b2c3d4` | Short code (first 8 chars of UUID), for binding commands |
| **Workspaces** | `["Code", "Personal"]` | Tags for filtering agents in TUI/mobile |

### Hierarchy

```
Machine (workstation)
  └── Identity 1 (SSH key → Principal "bob")
  │     └── /projects/website
  │     │     └── instance (bob-projects-website) [a1b2c3d4]
  │     │     └── instance (bob-projects-website-1) [e5f6g7h8]
  │     └── /projects/api
  │           └── instance (bob-projects-api) [i9j0k1l2]
  └── Identity 2 (SSH key → Principal "work-alice")
        └── /work/client-x
              └── instance (work-alice-work-client-x) [m3n4o5p6]
```

- One SSH key = one principal
- Multiple instances can share a principal (same key, different working dirs)
- Same identity + same working dir can have multiple instances (suffix: -1, -2, etc.)

## Agent Configuration

```toml
# ~/.config/fold/agents/bob.toml
name = "bob"
workspaces = ["Code", "Personal"]
server = "http://127.0.0.1:50051"
backend = "cli"
```

- `name`: Identity name, used for display and as prefix for instance names
- `workspaces`: Free-form tags for organizing agents (multiple allowed)
- `server`: Gateway address
- `backend`: "mux" (direct API) or "cli" (Claude CLI)

## Agent Registration Flow

### Current Flow (Broken)

1. Agent connects with SSH auth
2. Interceptor validates signature, looks up principal by fingerprint
3. AuthContext populated with `principal_id`
4. Agent sends `RegisterAgent` with self-declared `agent_id`
5. **Gateway uses self-declared ID, ignoring authenticated identity** ← BUG

### New Flow (Magical)

1. Agent connects with SSH auth
2. Interceptor validates signature, computes fingerprint
3. Gateway looks up principal by fingerprint:
   - **If found**: Use existing principal
   - **If not found**: Auto-create principal with status "approved" (configurable)
4. AuthContext populated with `principal_id`
5. Agent sends `RegisterAgent` with:
   - `agent_id`: Instance name (e.g., "bob-projects-website")
   - `name`: Identity name (e.g., "bob")
   - `metadata`: Working dir, workspaces, git info, etc.
6. **Gateway validates that agent's principal is approved**
7. Gateway registers connection using instance name for routing
8. Gateway returns `Welcome` with:
   - `agent_id`: Confirmed instance name
   - `instance_id`: Short code for binding commands
   - `principal_id`: For client reference

### Auto-Registration Modes

Configuration option `agent_auto_registration`:

| Mode | Behavior |
|------|----------|
| `approved` (default) | Auto-create principal with status "approved", agent can immediately receive messages |
| `pending` | Auto-create principal with status "pending", admin must approve before agent works |
| `disabled` | Reject unknown fingerprints, admin must pre-register |

## Agent TUI Display

```
┌─ fold-agent ─────────────────────────────────────────┐
│                                                      │
│  Identity: bob                                       │
│  Working dir: /projects/website                      │
│  Workspaces: Code, Personal                          │
│  Status: ● connected                                 │
│                                                      │
│  Instance ID: a1b2c3d4                               │
│                                                      │
│  To bind a channel to this agent:                    │
│    Slack:  /fold bind a1b2c3d4                       │
│    Matrix: !fold bind a1b2c3d4                       │
│                                                      │
└──────────────────────────────────────────────────────┘
```

The instance ID is a short code (first 8 chars of a UUID) that users copy to chat platforms for binding.

## Client (TUI/Mobile) Flow

### Authentication

- Clients authenticate via JWT (not SSH)
- JWT contains `principal_id` for client identity
- Client principals have type `"client"`

### Agent Discovery

- Clients see all online agents (v1: no permission filtering)
- Agents can be filtered by workspace
- Each agent shows: identity name, working dir, instance ID, workspaces

### Agent Selection

- Explicit selection each time
- Client picks agent from list to start conversation
- No "default agent" or sticky sessions (v1)

### Conversations

- One agent per conversation
- Switching agents requires starting new conversation
- Thread ID ties messages together

### TUI Display

```
┌─ fold-tui ─────────────────────────────────────────────┐
│                                                        │
│  Workspace: [Code ▼]                                   │
│                                                        │
│  Online Agents:                                        │
│  ● bob        /projects/website        a1b2c3d4       │
│  ● bob        /projects/api            e5f6g7h8       │
│  ○ alice      /work/client-x           (offline)      │
│                                                        │
│  [Select agent to start conversation]                  │
│                                                        │
└────────────────────────────────────────────────────────┘
```

## Data Model Changes

### Principals Table (Existing)

```sql
CREATE TABLE principals (
    principal_id TEXT PRIMARY KEY,
    type TEXT NOT NULL,              -- 'client' | 'agent' | 'pack'
    pubkey_fingerprint TEXT UNIQUE,  -- SHA-256 of SSH public key
    display_name TEXT NOT NULL,      -- Identity name
    status TEXT NOT NULL,            -- 'pending' | 'approved' | 'revoked' | 'offline' | 'online'
    created_at TEXT NOT NULL,
    last_seen TEXT,
    metadata_json TEXT               -- Workspaces, etc.
);
```

### Agent Metadata (in Registration)

```protobuf
message AgentMetadata {
    string working_directory = 1;
    string hostname = 2;
    string os = 3;
    repeated string workspaces = 4;
    GitInfo git = 5;
}
```

### Bindings Table (Future)

Bindings reference instance names, not principal IDs:

```sql
CREATE TABLE bindings (
    id TEXT PRIMARY KEY,
    frontend TEXT NOT NULL,          -- 'slack' | 'matrix' | 'telegram'
    channel_id TEXT NOT NULL,        -- Channel/room identifier
    agent_instance_name TEXT NOT NULL,  -- e.g., 'bob-projects-website'
    created_at TEXT NOT NULL,
    UNIQUE(frontend, channel_id)
);
```

## Gateway Implementation Changes

### grpc.go

```go
func (s *foldControlServer) AgentStream(stream pb.FoldControl_AgentStreamServer) error {
    // Extract authenticated principal from context
    authCtx := auth.FromContext(stream.Context())
    if authCtx == nil {
        return status.Error(codes.Unauthenticated, "no auth context")
    }

    // Wait for registration message
    msg, err := stream.Recv()
    // ... validation ...

    reg := msg.GetRegister()

    // Use instance name from registration, but principal is from auth
    instanceName := reg.GetAgentId()  // e.g., "bob-projects-website"
    principalID := authCtx.PrincipalID  // from SSH auth

    // Create connection with both identifiers
    conn := agent.NewConnection(
        instanceName,           // For routing
        principalID,            // For auth/audit
        reg.GetName(),          // Display name
        reg.GetCapabilities(),
        stream,
        s.logger,
    )

    // ... rest of handler
}
```

### Auto-create Principal (in auth/interceptor.go)

```go
// In extractAuth, when fingerprint not found:
if errors.Is(err, store.ErrPrincipalNotFound) {
    if config.AgentAutoRegistration == "disabled" {
        return nil, status.Error(codes.Unauthenticated, "unknown public key")
    }

    // Auto-create principal
    status := store.PrincipalStatusApproved
    if config.AgentAutoRegistration == "pending" {
        status = store.PrincipalStatusPending
    }

    principal = &store.Principal{
        ID:          uuid.New().String(),
        Type:        store.PrincipalTypeAgent,
        PubkeyFP:    fingerprint,
        DisplayName: "auto-registered",  // Updated on first registration
        Status:      status,
        CreatedAt:   time.Now(),
    }

    if err := principals.CreatePrincipal(ctx, principal); err != nil {
        return nil, status.Errorf(codes.Internal, "auto-creating principal: %v", err)
    }
}
```

## Agent (Rust) Changes

### Registration Message

```rust
tx.send(AgentMessage {
    payload: Some(agent_message::Payload::Register(RegisterAgent {
        agent_id: instance_name.clone(),  // "bob-projects-website"
        name: config.name.clone(),        // "bob"
        capabilities: vec!["chat".to_string()],
        metadata: Some(AgentMetadata {
            working_directory: working_dir.to_string_lossy().to_string(),
            hostname: hostname::get().unwrap_or_default().to_string_lossy().to_string(),
            os: std::env::consts::OS.to_string(),
            workspaces: config.workspaces.clone(),
            git: gather_git_info(&working_dir),
        }),
    })),
}).await?;
```

### Config Loading

```rust
#[derive(Deserialize)]
struct AgentConfig {
    name: String,
    #[serde(default)]
    workspaces: Vec<String>,
    server: String,
    backend: String,
}
```

## Deferred (Out of Scope)

- Binding creation via frontend commands (`/fold bind <instance-id>`)
- "Currently bound channels" display in agent TUI
- Permission model (workspace-based or ownership-based)
- Multi-agent conversations
- Agent handoff between instances

## Migration Path

1. **Gateway**: Add auto-registration, update grpc.go to use principal from auth
2. **Proto**: Add workspaces to AgentMetadata
3. **Agent**: Add workspaces to config, update registration
4. **TUI**: Add workspace filter, show agent details
5. **Bindings**: Update to use instance names (separate PR)
