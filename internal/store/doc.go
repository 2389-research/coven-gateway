// Package store provides persistent storage for the gateway using SQLite.
//
// # Architecture
//
// The store package uses an interface-driven architecture with multiple specialized
// interfaces:
//
//   - Store: Core operations for threads, messages, events, and bindings
//   - BuiltinStore: Built-in tool data (logs, todos, BBS, notes, mail)
//   - AdminStore: Admin users, sessions, WebAuthn credentials
//   - UsageStore: Token usage tracking and statistics
//   - SecretsStore: Secret management
//   - LinkCodeStore: Device linking codes
//
// SQLiteStore implements all interfaces in a single struct, allowing easy
// composition while maintaining clear interface boundaries.
//
// # Data Models
//
// Core models:
//
//   - Thread: Conversation linking frontend channels to agents
//   - Message: Individual messages with type (message, tool_use, tool_result)
//   - LedgerEvent: Immutable event log for auditing
//   - Principal: Identity (agent, user, admin) with capabilities
//   - Binding: Channel-to-agent routing assignments
//
// Built-in tool models:
//
//   - LogEntry: Activity logs with tags
//   - Todo: Tasks with status/priority
//   - BBSPost/BBSThread: Discussion threads
//   - AgentMail: Inter-agent messaging
//   - AgentNote: Key-value storage per agent
//
// Admin models:
//
//   - AdminUser: Admin accounts with WebAuthn
//   - Session: Browser sessions
//   - WebAuthnCredential: Passkey credentials
//   - Token: API tokens for principals
//
// # SQLite Configuration
//
// The store uses SQLite with WAL mode for concurrent reads:
//
//	PRAGMA journal_mode=WAL;
//	PRAGMA foreign_keys=ON;
//
// Database file locations:
//
//   - Production: /var/lib/coven-gateway/gateway.db
//   - Development: ~/.local/share/coven/gateway.db
//   - Testing: :memory: (in-memory database)
//
// # Error Handling
//
// Common errors:
//
//   - ErrNotFound: Requested entity does not exist
//   - ErrDuplicateThread: Thread already exists
//
// All methods accept context.Context for cancellation support.
//
// # Testing
//
// Use NewMockStore() for unit tests:
//
//	store := store.NewMockStore()
//	// store implements all Store interfaces
//
// Use NewSQLiteStore(":memory:") for integration tests with real SQLite.
//
// # Migrations
//
// Migrations are embedded and run automatically on store initialization.
// Migration files are in internal/store/migrations/ with numeric prefixes.
package store
