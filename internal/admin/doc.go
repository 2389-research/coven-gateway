// Package admin provides administrative API handlers for the gateway.
//
// # Overview
//
// The admin package exposes internal administrative operations as HTTP handlers,
// used by the web admin UI and the coven-admin CLI tool.
//
// # Endpoints
//
// Principal management:
//
//   - GET /api/admin/principals - List all principals
//   - GET /api/admin/principals/:id - Get a principal
//   - PUT /api/admin/principals/:id - Update principal (status, capabilities)
//   - DELETE /api/admin/principals/:id - Delete a principal
//
// Agent management:
//
//   - GET /api/admin/agents - List agents with connection status
//   - POST /api/admin/agents/:id/approve - Approve pending agent
//   - POST /api/admin/agents/:id/revoke - Revoke agent access
//
// Token management:
//
//   - GET /api/admin/tokens - List API tokens
//   - POST /api/admin/tokens - Create a new token
//   - DELETE /api/admin/tokens/:id - Revoke a token
//
// Binding management:
//
//   - GET /api/admin/bindings - List channel bindings
//   - POST /api/admin/bindings - Create a binding
//   - DELETE /api/admin/bindings/:id - Delete a binding
//
// # Principal Types
//
// The system tracks three types of principals:
//
//   - agent: AI agents (coven-agent instances)
//   - user: Human users (via frontends)
//   - admin: Administrative users (web admin access)
//
// # Principal Status
//
// Agents have status affecting their connection ability:
//
//   - pending: Awaiting admin approval
//   - approved: Can connect and receive messages
//   - revoked: Access denied
//
// # Capabilities
//
// Capabilities control what tools a principal can use:
//
//   - base: Basic tools (logging, todos, BBS)
//   - notes: Key-value note storage
//   - mail: Inter-agent messaging
//   - admin: Administrative tools
//   - ui: User interaction tools
//
// # Authentication
//
// Admin endpoints require authentication:
//
//   - Web UI: Session cookie (from WebAuthn login)
//   - CLI: API token in Authorization header
//
// # Usage
//
// Create admin handlers:
//
//	adminAPI := admin.New(store, agentManager, logger)
//	router.Mount("/api/admin", adminAPI.Handler())
package admin
