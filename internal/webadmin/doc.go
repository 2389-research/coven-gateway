// Package webadmin provides the web-based administration interface.
//
// # Overview
//
// The web admin provides a browser-based interface for:
//
//   - Chat UI: Real-time conversations with agents
//   - Agent Management: View connected agents, approve pending agents
//   - Settings: Configure bindings, view tools, manage credentials
//   - Help Documentation: Embedded help pages
//
// # Architecture
//
// Components:
//
//   - Admin: Main struct coordinating handlers and templates
//   - Templates: HTML templates embedded in the binary
//   - Handlers: HTTP handlers for each page/action
//   - Auth: Session management and WebAuthn
//
// # Authentication
//
// The web admin supports multiple auth methods:
//
//   - Password: Initial bootstrap only
//   - WebAuthn/Passkeys: Primary auth (requires HTTPS)
//   - JWT Sessions: Browser session tokens
//
// Bootstrap flow:
//
//  1. Run `coven-gateway bootstrap --name "Admin"`
//  2. Open the printed setup URL
//  3. Register a passkey
//  4. Login with passkey thereafter
//
// # Chat Interface
//
// The chat UI provides real-time messaging:
//
//   - Agent selection sidebar
//   - Message input with markdown support
//   - Streaming responses with thinking indicators
//   - Tool usage display
//   - Token usage statistics
//
// Implementation:
//   - Server-Sent Events for streaming
//   - HTMX for dynamic updates
//   - Alpine.js for interactivity
//
// # Settings Pages
//
// Settings sections:
//
//   - Agents: List, approve, revoke agents
//   - Tools: View available tools from all packs
//   - Bindings: Manage channel-to-agent bindings
//   - Credentials: Manage WebAuthn credentials
//
// # Help Documentation
//
// Embedded help pages in templates/help/:
//
//   - getting-started.md
//   - agents.md
//   - tools.md
//   - configuration.md
//   - keyboard-shortcuts.md
//   - troubleshooting.md
//
// Help pages are rendered as HTML with markdown processing.
//
// # Templates
//
// Templates use Go's html/template with custom functions:
//
//   - Base layout: templates/layout.html
//   - Chat: templates/chat.html
//   - Settings: templates/settings/*.html
//   - Help: templates/help/*.md
//
// Templates are embedded using //go:embed for single-binary deployment.
//
// # CSRF Protection
//
// All form submissions require CSRF tokens:
//
//	<input type="hidden" name="csrf_token" value="{{.CSRFToken}}">
//
// Tokens are validated on POST/PUT/DELETE requests.
//
// # Usage
//
// Create and mount the admin:
//
//	admin := webadmin.New(store, agentManager, config, logger)
//	http.Handle("/", admin.Handler())
//
// The admin mounts at "/" and handles all web UI routes.
package webadmin
