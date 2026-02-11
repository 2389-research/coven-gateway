// Package builtins provides built-in tool packs for agents.
//
// # Overview
//
// Built-in tools are gateway-provided tools that agents can use without
// external dependencies. They provide core functionality like logging,
// task management, inter-agent communication, and user interaction.
//
// # Tool Packs
//
// The package provides 5 packs with 21 tools:
//
// Base Pack (builtin:base) - requires "base" capability:
//
//   - log_entry: Log an activity or event
//   - log_search: Search past log entries
//   - todo_add: Create a todo
//   - todo_list: List todos (filter by status/priority)
//   - todo_update: Update a todo's status, priority, or notes
//   - todo_delete: Delete a todo
//   - bbs_create_thread: Create a new discussion thread
//   - bbs_reply: Reply to a thread
//   - bbs_list_threads: List discussion threads
//   - bbs_read_thread: Read a thread with replies
//
// Notes Pack (builtin:notes) - requires "notes" capability:
//
//   - note_set: Store a note
//   - note_get: Retrieve a note
//   - note_list: List all note keys
//   - note_delete: Delete a note
//
// Mail Pack (builtin:mail) - requires "mail" capability:
//
//   - mail_send: Send message to another agent
//   - mail_inbox: List received messages
//   - mail_read: Read and mark message as read
//
// Admin Pack (builtin:admin) - requires "admin" capability:
//
//   - admin_list_agents: List connected agents
//   - admin_agent_messages: Read all messages for an agent
//   - admin_send_message: Send message to another agent
//
// UI Pack (builtin:ui) - requires "ui" capability:
//
//   - ask_user: Ask the user a question and wait for response
//
// # Registration
//
// Register all built-in packs:
//
//	builtins.RegisterAll(registry, store)
//
// Register individual packs:
//
//	builtins.RegisterBasePack(registry, store)
//	builtins.RegisterNotesPack(registry, store)
//
// # Tool Implementation
//
// Each tool is a function with signature:
//
//	func(ctx context.Context, agentID string, params map[string]any) (any, error)
//
// Tools receive:
//   - Context for cancellation
//   - Agent ID for scoping data
//   - Parameters from the agent's tool call
//
// # Data Persistence
//
// Built-in tools use BuiltinStore for persistence:
//
//   - Logs, todos, BBS posts scoped by agent ID
//   - Notes are key-value pairs per agent
//   - Mail is inter-agent (not scoped)
//
// # User Interaction
//
// The ask_user tool enables agents to request input from users:
//
//  1. Agent calls ask_user with question
//  2. Gateway broadcasts question to connected clients
//  3. User answers via web UI or API
//  4. Answer returned to agent
//
// Questions have timeout (default 5 minutes) and can include
// suggested options for the user.
package builtins
