# Built-in Tool Packs Design

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Embed useful productivity tools directly in the gateway so agents have default capabilities without requiring external tool packs.

**Architecture:** Built-in packs register at gateway startup using the existing `packs.Registry`. Tools execute in-process (Go function calls) rather than routing through gRPC. Agents see a unified tool list filtered by capabilities.

---

## Capability-to-Pack Mapping

| Capability | Pack | Description |
|------------|------|-------------|
| `base` | Base Pack | Logging, todos, bulletin board |
| `admin` | Admin Pack | Gateway administration, agent control |
| `mail` | Mail Pack | Agent-to-agent messaging |
| `notes` | Notes Pack | Key-value persistence |

An agent with `capabilities: ["base", "admin"]` gets Base + Admin tools.

---

## Tool Definitions

### Base Pack (`base`)

| Tool | Description | Inputs |
|------|-------------|--------|
| `log_entry` | Log an activity or event | `message`, `tags[]` |
| `log_search` | Search past log entries | `query?`, `since?`, `limit?` |
| `todo_add` | Create a todo | `description`, `priority?`, `due_date?`, `notes?` |
| `todo_list` | List todos | `status?`, `priority?` |
| `todo_update` | Update a todo | `id`, `status?`, `priority?`, `notes?`, `due_date?` |
| `todo_delete` | Delete a todo | `id` |
| `bbs_create_thread` | Create a new discussion thread | `subject`, `content` |
| `bbs_reply` | Reply to a thread | `thread_id`, `content` |
| `bbs_list_threads` | List discussion threads | `limit?` |
| `bbs_read_thread` | Read a thread with replies | `thread_id` |

### Admin Pack (`admin`)

| Tool | Description | Inputs |
|------|-------------|--------|
| `admin_list_agents` | List connected agents | *(none)* |
| `admin_agent_history` | Read an agent's message history | `agent_id`, `limit?` |
| `admin_send_message` | Send message to another agent | `agent_id`, `content` |

### Mail Pack (`mail`)

| Tool | Description | Inputs |
|------|-------------|--------|
| `mail_send` | Send message to another agent | `to_agent_id`, `subject`, `content` |
| `mail_inbox` | List received messages | `limit?`, `unread_only?` |
| `mail_read` | Read and mark message as read | `message_id` |

### Notes Pack (`notes`)

| Tool | Description | Inputs |
|------|-------------|--------|
| `note_set` | Store a note | `key`, `value` |
| `note_get` | Retrieve a note | `key` |
| `note_list` | List all note keys | *(none)* |
| `note_delete` | Delete a note | `key` |

---

## Data Model

New tables in existing SQLite store:

```sql
-- Log entries (chronicle-style activity logging)
CREATE TABLE log_entries (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    message TEXT NOT NULL,
    tags TEXT,                      -- JSON array of strings
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_log_entries_agent ON log_entries(agent_id);
CREATE INDEX idx_log_entries_created ON log_entries(created_at);

-- Todos (task management)
CREATE TABLE todos (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    description TEXT NOT NULL,
    status TEXT DEFAULT 'pending',  -- pending, in_progress, completed
    priority TEXT DEFAULT 'medium', -- low, medium, high
    notes TEXT,
    due_date TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_todos_agent ON todos(agent_id);
CREATE INDEX idx_todos_status ON todos(status);

-- BBS posts (bulletin board)
CREATE TABLE bbs_posts (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    thread_id TEXT,                 -- NULL for top-level posts (threads)
    subject TEXT,                   -- required for threads, NULL for replies
    content TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_bbs_posts_thread ON bbs_posts(thread_id);
CREATE INDEX idx_bbs_posts_created ON bbs_posts(created_at);

-- Agent mail (inter-agent messaging)
CREATE TABLE agent_mail (
    id TEXT PRIMARY KEY,
    from_agent_id TEXT NOT NULL,
    to_agent_id TEXT NOT NULL,
    subject TEXT NOT NULL,
    content TEXT NOT NULL,
    read_at TIMESTAMP,              -- NULL if unread
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_agent_mail_to ON agent_mail(to_agent_id);
CREATE INDEX idx_agent_mail_unread ON agent_mail(to_agent_id, read_at);

-- Agent notes (key-value storage)
CREATE TABLE agent_notes (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(agent_id, key)
);
CREATE INDEX idx_agent_notes_agent ON agent_notes(agent_id);
```

**Scoping:**
- Log entries: Shared (all agents can search all logs for visibility)
- Todos: Per-agent (agents only see their own todos)
- BBS: Shared (all agents see all posts - it's a bulletin board)
- Mail: Per-agent inbox (agents only see mail sent to them)
- Notes: Per-agent (agents only see their own notes)

---

## Architecture

### Registry Extension

The `packs.Registry` needs to support built-in tools that execute in-process:

```go
// BuiltinTool represents a tool that executes in the gateway process
type BuiltinTool struct {
    Definition *pb.ToolDefinition
    Handler    ToolHandler
}

// ToolHandler executes a built-in tool
type ToolHandler func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error)
```

The Registry distinguishes between:
- External pack tools → route to pack's gRPC channel
- Built-in tools → call Handler directly

### Built-in Pack Registration

At gateway startup:

```go
func (g *Gateway) registerBuiltinPacks() {
    // Base pack
    g.registry.RegisterBuiltinPack("builtin:base", []*BuiltinTool{
        {Definition: logEntryDef, Handler: g.builtins.LogEntry},
        {Definition: logSearchDef, Handler: g.builtins.LogSearch},
        // ... etc
    })

    // Admin pack
    g.registry.RegisterBuiltinPack("builtin:admin", []*BuiltinTool{
        {Definition: adminListAgentsDef, Handler: g.builtins.AdminListAgents},
        // ... etc
    })

    // ... other packs
}
```

### Tool Dispatch Flow

```
Agent calls tool "todo_add"
         │
         ▼
    Router.DispatchTool()
         │
         ├─► Is it a builtin tool?
         │        │
         │        YES → Call handler directly: handler(ctx, agentID, input)
         │        │
         │        └─► Return result
         │
         └─► NO → Route to external pack via gRPC channel (existing flow)
```

---

## Webadmin UI

New sections in webadmin to view agent-created content:

1. **Logs tab** - Searchable activity log with tag filtering
2. **Todos tab** - List of all todos across agents, grouped by agent
3. **Board tab** - BBS threads and replies
4. **Mail tab** - (Admin only?) View inter-agent messages

Each view shows which agent created the content and when.

---

## Implementation Tasks

### Task 1: Store Schema Extensions
- Add migration for new tables (log_entries, todos, bbs_posts, agent_mail, agent_notes)
- Add Store interface methods for each table
- Implement SQLite methods
- Add tests

### Task 2: Registry Builtin Support
- Add BuiltinTool struct and ToolHandler type
- Add RegisterBuiltinPack method to Registry
- Modify GetToolByName to return builtin indicator
- Update Router to dispatch builtins directly
- Add tests

### Task 3: Base Pack Implementation
- Create internal/builtins/base.go
- Implement log_entry, log_search handlers
- Implement todo_add, todo_list, todo_update, todo_delete handlers
- Implement bbs_create_thread, bbs_reply, bbs_list_threads, bbs_read_thread handlers
- Add tests

### Task 4: Admin Pack Implementation
- Create internal/builtins/admin.go
- Implement admin_list_agents (uses agent.Manager)
- Implement admin_agent_history (uses Store.GetThreadMessages or events)
- Implement admin_send_message (uses agent.Manager.SendMessage)
- Add tests

### Task 5: Mail Pack Implementation
- Create internal/builtins/mail.go
- Implement mail_send, mail_inbox, mail_read handlers
- Add tests

### Task 6: Notes Pack Implementation
- Create internal/builtins/notes.go
- Implement note_set, note_get, note_list, note_delete handlers
- Add tests

### Task 7: Gateway Integration
- Register all builtin packs at startup
- Wire up store and manager dependencies
- Add tests for end-to-end tool dispatch

### Task 8: Webadmin UI
- Add Logs tab with search/filter
- Add Todos tab with agent grouping
- Add Board tab with thread view
- Add routes and handlers
- Add tests

---

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Storage | Existing SQLite store | Single database, simple backup/query |
| Integration | Register with existing Registry | Unified tool dispatch, capability filtering works |
| Dispatch | In-process function calls | No gRPC overhead for builtin tools |
| Capability model | Use existing capabilities | Consistent, no special cases |
| Base tools | Require `base` capability | Explicit opt-in, allows minimal agents |
| Log/BBS visibility | Shared across agents | Collaboration, admin visibility |
| Todo/Notes visibility | Per-agent | Privacy, no cross-contamination |
