# Multi-Tenancy Design

## Overview

Transform coven-gateway from single-tenant to multi-tenant with Tailscale-style isolation. Each tenant is called an "org" and gets its own subdomain (`acme.coven.example.com`) and SQLite database.

## Key Decisions

| Decision | Choice |
|----------|--------|
| Tenant terminology | "org" |
| Auth model | Principal-based + subdomain routing |
| Agent ownership | Strictly one org per agent |
| Data isolation | Separate SQLite DB per org |
| Org creation | Self-service via web signup |
| Device linking | coven-link flow (existing Rust binary) |
| Teams | Future scope (v1 is orgs only) |

## Architecture

```
                    ┌─────────────────────────────────────────────────────┐
                    │                   coven-gateway                      │
                    │                                                      │
   acme.coven.io ──►│  ┌─────────────┐    ┌─────────────┐                 │
                    │  │ Org Router  │───►│ Org Context │                 │
   beta.coven.io ──►│  │ (hostname)  │    │ (per-req)   │                 │
                    │  └─────────────┘    └──────┬──────┘                 │
                    │                            │                         │
                    │         ┌──────────────────┼──────────────────┐     │
                    │         ▼                  ▼                  ▼     │
                    │  ┌─────────────┐    ┌─────────────┐    ┌──────────┐ │
                    │  │ acme.db     │    │ beta.db     │    │ system.db│ │
                    │  │ (principals,│    │ (principals,│    │ (orgs,   │ │
                    │  │  threads,   │    │  threads,   │    │  users,  │ │
                    │  │  bindings)  │    │  bindings)  │    │  links)  │ │
                    │  └─────────────┘    └─────────────┘    └──────────┘ │
                    └─────────────────────────────────────────────────────┘
```

**Request flow:**

1. Request arrives at `acme.coven.example.com`
2. Org router extracts `acme` from hostname
3. Validates org exists in `system.db`
4. Opens/reuses connection to `orgs/acme.db`
5. Attaches org context to request
6. All downstream queries use org-scoped store

## Database Schemas

### System Database (`data/system.db`)

```sql
CREATE TABLE users (
    user_id       TEXT PRIMARY KEY,
    email         TEXT UNIQUE NOT NULL,
    username      TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    created_at    TEXT NOT NULL
);

CREATE TABLE orgs (
    org_id        TEXT PRIMARY KEY,
    slug          TEXT UNIQUE NOT NULL,
    display_name  TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'active',
    created_at    TEXT NOT NULL,
    settings_json TEXT,

    CHECK (status IN ('active', 'suspended'))
);

CREATE TABLE org_memberships (
    user_id    TEXT NOT NULL REFERENCES users(user_id),
    org_id     TEXT NOT NULL REFERENCES orgs(org_id),
    role       TEXT NOT NULL,
    created_at TEXT NOT NULL,

    PRIMARY KEY (user_id, org_id),
    CHECK (role IN ('owner', 'admin', 'member'))
);

CREATE TABLE link_codes (
    code        TEXT PRIMARY KEY,
    org_id      TEXT NOT NULL REFERENCES orgs(org_id),
    machine_id  TEXT NOT NULL,
    pubkey_fp   TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    expires_at  TEXT NOT NULL,
    claimed_at  TEXT,
    claimed_by  TEXT REFERENCES users(user_id)
);

CREATE INDEX idx_link_codes_org ON link_codes(org_id);
CREATE INDEX idx_link_codes_expires ON link_codes(expires_at);
```

### Per-Org Database (`data/orgs/{slug}.db`)

Same schema as current `sqlite.go` with these changes:

- Remove `admin_users`, `admin_sessions`, `admin_invites` (now in system.db)
- All data implicitly scoped to this org

```sql
-- Org-specific tables (in addition to existing schema)
CREATE TABLE org_settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- Existing tables remain: principals, threads, messages, bindings,
-- ledger_events, roles, audit_log, channel_bindings, agent_state,
-- webauthn_credentials
```

## User Flows

### Sign-up (auto-org creation)

1. User visits `coven.example.com/signup`
2. Enters email, password, username
3. Gateway creates user + personal org (`{username}`)
4. User becomes org owner
5. Redirect to dashboard: "Your org is harper.coven.example.com"

### Device Linking (coven-link)

```
User Machine                  Browser                    Gateway
  │                              │                          │
  ├─ coven-link acme.coven.io   │                          │
  │                              │                          │
  ├─ POST /api/link/init ───────────────────────────────────►│
  │   {machine_id, pubkey_fp}   │                          │
  │◄─────────────────────────── {code: "ABC-123"} ─────────┤
  │                              │                          │
  │  "Enter ABC-123 at          │                          │
  │   acme.coven.io/link"       │                          │
  │                              │                          │
  │                              ├─ Login + enter code ────►│
  │                              │                          ├─ Validate code
  │                              │                          ├─ Create principal
  │                              │◄─── "Device linked!" ───┤
  │                              │                          │
  ├─ Poll /api/link/status ─────────────────────────────────►│
  │◄─────────────────────────── {status: "approved"} ──────┤
  │                              │                          │
  │  "Linked! Run coven-agent"  │                          │
```

### Agent Connection

1. Agent connects to `acme.coven.example.com:50051` (gRPC)
2. Gateway extracts org from `:authority` header
3. Validates org exists and is active
4. Validates agent's pubkey is approved principal in org
5. If unknown pubkey + `allow_pending: true` → create pending principal
6. If unknown pubkey + `allow_pending: false` → reject

### Pending Approval (fallback)

```
Agent                                                    Gateway
  │                                                         │
  ├─ Connect (unknown pubkey) ─────────────────────────────►│
  │                                                         ├─ Check allow_pending
  │                                                         ├─ Create pending principal
  │◄─────────────────────────── Rejected: pending ─────────┤
  │                                                         │
  │   (owner approves in web UI or CLI)                     │
  │                                                         │
  ├─ Reconnect ────────────────────────────────────────────►│
  │◄─────────────────────────── Welcome ───────────────────┤
```

## API Endpoints

### Link Protocol (for coven-link)

```
POST /api/link/init
Request:  { "machine_id": "...", "pubkey_fingerprint": "SHA256:..." }
Response: { "code": "ABC-123", "expires_in": 600 }

GET /api/link/status?machine_id=...
Response: { "status": "pending" | "approved" | "expired" }
          If approved: { "status": "approved", "principal_id": "..." }
```

### Web UI

```
GET  /signup            # Sign-up form
POST /signup            # Create user + auto-org

GET  /login             # Login form
POST /login             # Create session

GET  /link              # Enter link code (org-scoped)
POST /link/approve      # Approve device { "code": "ABC-123" }

GET  /dashboard         # Org overview
GET  /agents            # List agents
POST /agents/:id/approve  # Approve pending agent
```

## CLI Commands

### coven-admin (org management, not creation)

```bash
# List your orgs
$ coven-admin org list
SLUG      ROLE    AGENTS
harper    owner   2

# List agents in org
$ coven-admin agents list
NAME              STATUS    LAST SEEN
harper-macbook    online    now
harper-server     offline   2h ago

# See pending devices
$ coven-admin agents pending
MACHINE ID        PUBKEY                REQUESTED
a8f8c2d...        SHA256:xK7m...        5 min ago

# Approve from CLI
$ coven-admin agents approve a8f8c2d
```

## Implementation Plan

### New Code

| Component | Location | Description |
|-----------|----------|-------------|
| System store | `internal/org/system_store.go` | CRUD for orgs, users, link codes |
| Org store pool | `internal/org/pool.go` | Lazy-load per-org SQLite connections |
| Org router | `internal/org/router.go` | HTTP middleware: hostname → org context |
| gRPC org interceptor | `internal/org/grpc.go` | Same for gRPC streams |
| Link service | `internal/link/service.go` | Code generation, polling, approval |
| Web auth | `internal/webadmin/auth.go` | Signup, login, sessions |
| Web link UI | `internal/webadmin/link.go` | Device approval page |

### Existing Code Modifications

| File | Change |
|------|--------|
| `internal/auth/context.go` | Add `OrgID`, `OrgSlug` fields |
| `internal/gateway/gateway.go` | Wire org router before handlers |
| `internal/gateway/grpc.go` | Validate agent's org on connect |
| `internal/gateway/api.go` | Get store from org context |
| `internal/agent/manager.go` | Partition connections by org |
| `cmd/coven-gateway/main.go` | Initialize system store + org pool |
| `cmd/coven-admin/` | Rework for user-scoped org management |

### Proto Changes (coven-agent side)

| Field | Where | Purpose |
|-------|-------|---------|
| `bootstrap_token` | `RegisterAgent` | First agent claims org (if needed) |
| `org_slug` | Server metadata | Confirm which org agent connected to |

### Database Layout

```
data/
├── system.db           # orgs, users, link_codes
└── orgs/
    ├── harper.db       # harper's personal org
    └── acme.db         # acme org
```

## Org Settings

Per-org configuration stored in `org_settings` table:

| Key | Default | Description |
|-----|---------|-------------|
| `allow_pending` | `true` | Allow unknown agents to land in pending queue |

## Future Considerations

- **Teams**: Query-level isolation within an org (v2)
- **Org invites**: Invite users to join org with specific role
- **Org transfer**: Transfer ownership to another user
- **Org deletion**: Soft-delete with grace period
