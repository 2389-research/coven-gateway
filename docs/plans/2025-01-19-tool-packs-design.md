# Tool Packs Design

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Enable extensible tool provisioning through pluggable "packs" that connect to the gateway and provide tools to agents.

**Architecture:** Packs connect to gateway via gRPC, register tools with MCP-compatible schemas. Gateway routes tool calls, exposes MCP server for external agents (CC/ACP), delivers tools natively via gRPC for controlled agents (mux).

**Tech Stack:** Go (gateway), Rust (packs), gRPC, MCP over HTTP, fold-ssh for auth

---

## Architecture Overview

**The players:**
- **Gateway**: Central hub, routes all tool calls, exposes MCP server for external agents
- **Packs**: Connect to gateway via gRPC, provide tools, handle tool execution
- **Agents**: Request tools at registration, call tools through gateway

**Communication paths:**
```
┌──────────────┐                  ┌──────────────┐
│  Tool Pack   │◄───── gRPC ─────►│              │
└──────────────┘                  │              │
                                  │   Gateway    │
┌──────────────┐                  │              │
│  MCP Bridge  │◄───── gRPC ─────►│              │
│    Pack      │                  │              │
└──────────────┘                  └──────┬───────┘
       │                                 │
       ▼                           ┌─────┴─────┐
┌──────────────┐                   │           │
│ External MCP │            gRPC   │   MCP/HTTP
│   Server     │                   ▼           ▼
└──────────────┘            ┌──────────┐ ┌──────────┐
                            │   mux    │ │ CC / ACP │
                            │  agents  │ │  agents  │
                            └──────────┘ └──────────┘
```

**Key principle:** Gateway is always in the loop. Agents never talk directly to packs.

---

## Pack Registration

**Connection flow:**
1. Pack connects to gateway via gRPC
2. Pack authenticates using SSH key (fold-ssh, same as agents)
3. Pack sends manifest with all tool definitions
4. Gateway validates: no tool name collisions, valid schemas
5. Gateway accepts or rejects registration

**Manifest format (protobuf wrapping MCP JSON):**
```protobuf
message ToolDefinition {
  string name = 1;
  string description = 2;
  string input_schema_json = 3;  // MCP-compatible JSON Schema
  repeated string required_capabilities = 4;
}

message PackManifest {
  string pack_id = 1;
  string version = 2;
  repeated ToolDefinition tools = 3;
}

message PackRegistration {
  PackManifest manifest = 1;
  // SSH auth handled at connection level via fold-ssh
}
```

**Lifecycle:**
- Pack sends manifest once on connect (static)
- If pack's tools change, pack disconnects and reconnects with new manifest
- Gateway notifies connected agents to restart for new tools (push model)
- If pack disconnects unexpectedly, its tools become unavailable

**Collision handling:**
- Gateway rejects registration if any tool name already exists
- Operator must resolve conflicts before pack can register

---

## Tool Call Flow

**Message sequence:**
```
Agent                    Gateway                   Pack
  │                         │                        │
  │─── ToolCall ───────────►│                        │
  │    (tool_name, args)    │                        │
  │                         │─── ExecuteTool ───────►│
  │                         │    (tool_name, args)   │
  │                         │                        │
  │                         │◄── ToolResult ─────────│
  │                         │    (output or error)   │
  │◄── ToolResult ──────────│                        │
  │    (pass-through)       │                        │
```

**For mux agents (gRPC):**
- Tool definitions sent during agent registration
- Tool calls/results flow through existing gRPC stream
- Native integration, no protocol translation

**For CC/ACP agents (MCP over HTTP):**
- Gateway exposes MCP server endpoint
- Agent configured to use gateway as MCP server
- Gateway translates MCP requests to internal gRPC calls to packs
- Results translated back to MCP responses

**Error handling:**
- Pack returns error → gateway passes through as-is
- Pack timeout → gateway returns timeout error
- Pack disconnected → gateway returns unavailable error

**Timeouts:**
- Default: 30 seconds per tool call
- Pack can specify per-tool timeout in manifest
- No agent override (keeps it simple)

---

## Capability-Based Access

**How it works:**
- Each tool declares required capabilities (e.g., `["filesystem"]`)
- Each agent has granted capabilities (set at registration)
- Agent only sees tools where it has ALL required capabilities

**Example:**
```
Pack "file-tools" registers:
  - read_file    (capabilities: ["filesystem"])
  - write_file   (capabilities: ["filesystem"])
  - delete_file  (capabilities: ["filesystem", "destructive"])

Agent "research-bot" has: ["filesystem", "web"]
  → Sees: read_file, write_file
  → Hidden: delete_file (missing "destructive")

Agent "admin-bot" has: ["filesystem", "destructive"]
  → Sees: read_file, write_file, delete_file
```

**Capability strings:**
- Flat, simple names: `"filesystem"`, `"web"`, `"database"`, `"destructive"`
- No hierarchy or namespacing
- Defined by convention, not enforced schema

**When capabilities are checked:**
1. Agent registration: gateway filters tool list based on agent's capabilities
2. Tool call: gateway verifies agent still has required capabilities

**MCP bridge packs:**
- Bridge pack can assign capabilities to wrapped MCP tools
- Or inherit a default capability for all tools from that MCP server

---

## Proto Changes

**New messages for fold.proto:**

```protobuf
// Tool definitions
message ToolDefinition {
  string name = 1;
  string description = 2;
  string input_schema_json = 3;
  repeated string required_capabilities = 4;
  int32 timeout_seconds = 5;  // optional, default 30
}

message PackManifest {
  string pack_id = 1;
  string version = 2;
  repeated ToolDefinition tools = 3;
}

// Tool execution
message ExecuteToolRequest {
  string tool_name = 1;
  string input_json = 2;
  string request_id = 3;
}

message ExecuteToolResponse {
  string request_id = 1;
  oneof result {
    string output_json = 2;
    string error = 3;
  }
}

// Pack service
service PackService {
  rpc Connect(PackManifest) returns (stream ExecuteToolRequest);
  rpc ToolResult(ExecuteToolResponse) returns (Empty);
}
```

**Addition to existing AgentRegistration:**
```protobuf
message AgentRegistration {
  // ... existing fields ...
  repeated string capabilities = N;
}
```

**Gateway exposes:**
- Existing `FoldService` for agents (with tool definitions in registration response)
- New `PackService` for packs
- MCP HTTP endpoint for external agents (CC/ACP)

---

## Implementation Tasks

### Task 1: Proto Updates
- Add ToolDefinition, PackManifest messages to fold.proto
- Add ExecuteToolRequest, ExecuteToolResponse messages
- Add PackService definition
- Add capabilities field to AgentRegistration
- Regenerate Go and Rust code

### Task 2: Gateway Pack Registry
- Create internal/packs/registry.go
- Store connected packs and their tools
- Implement collision detection on registration
- Handle pack disconnect (remove tools)

### Task 3: Gateway PackService Implementation
- Create internal/packs/service.go
- Implement Connect RPC (validate manifest, register pack)
- Implement ToolResult RPC (route results back)
- SSH authentication via fold-ssh

### Task 4: Gateway Tool Router
- Create internal/packs/router.go
- Route tool calls from agents to appropriate pack
- Handle timeouts and errors
- Pass-through error responses

### Task 5: Agent Tool Filtering
- Modify agent registration to include capabilities
- Filter available tools based on agent capabilities
- Include filtered tool list in registration response

### Task 6: MCP Server for External Agents
- Create internal/mcp/server.go
- Expose HTTP endpoint implementing MCP protocol
- Translate MCP tool calls to internal pack calls
- Return MCP-formatted responses

### Task 7: Rust Pack SDK (fold-pack crate)
- Create new crate in fold-common
- Provide PackClient for connecting to gateway
- Helper for building manifests
- Tool execution trait/callback

### Task 8: MCP Bridge Pack
- Create example pack that wraps external MCP servers
- Demonstrate MCP-to-gRPC translation
- Test with a standard MCP server

---

## Design Decisions Summary

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Tool discovery | Push model | Agents restart for new tools, simple and predictable |
| Tool filtering | Capability-based | Agents only see tools they're authorized for |
| Tool routing | Gateway always in loop | Security, observability, control |
| External agent protocol | MCP over HTTP | Standard protocol, works with CC/ACP |
| Controlled agent protocol | gRPC | Native integration, no translation |
| Pack registration | Static manifest | Simple, reconnect for changes |
| Schema format | Protobuf wrapping MCP JSON | Type-safe transport, MCP compatibility |
| Capability names | Flat strings | Simple, MCP tools keep their own names |
| Error handling | Pass-through | Packs own their error messages |
| Pack auth | SSH keys (fold-ssh) | Consistent with agent auth |
| Name collisions | Reject at registration | Catch config errors early |
