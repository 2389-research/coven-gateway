// Package conversation provides high-level conversation management services.
//
// # Overview
//
// The conversation package sits between the HTTP/WebSocket handlers and the
// agent manager, providing conversation-level abstractions like thread
// management, message routing, and response broadcasting.
//
// # Service
//
// The Service coordinates conversation operations:
//
//	svc := conversation.NewService(store, agentManager, broadcaster)
//
// Key operations:
//
//   - SendMessage(ctx, req): Send a user message and return streaming response
//   - GetThread(ctx, id): Retrieve a conversation thread
//   - ListThreads(ctx): List recent threads
//
// # Thread Management
//
// Threads link frontend conversations to agents:
//
//   - Thread ID: Unique identifier (UUID)
//   - Frontend Name: "webadmin", "matrix", "slack", etc.
//   - External ID: Frontend-specific ID (channel ID, room ID)
//   - Agent ID: The agent handling this conversation
//
// When a message arrives:
//
//  1. Look up existing thread by frontend + external ID
//  2. If not found, create a new thread with an assigned agent
//  3. Route the message to the thread's agent
//  4. Store the exchange in the ledger
//
// # Event Broadcasting
//
// The service broadcasts response events for real-time updates:
//
//	svc.Subscribe(threadID) -> <-chan Event
//
// Events include:
//   - Thinking: Agent is processing
//   - Text: Text chunk received
//   - ToolUse: Agent is using a tool
//   - ToolResult: Tool execution complete
//   - Done: Response complete
//   - Error: Error occurred
//
// # Ledger Events
//
// All conversation events are persisted as LedgerEvents for:
//
//   - Audit trails
//   - Conversation history
//   - Analytics
//
// Event types: user_message, assistant_text, tool_use, tool_result, etc.
//
// # Usage in Web Admin
//
// The web admin chat uses the conversation service:
//
//  1. User sends message via WebSocket
//  2. Service routes to agent
//  3. Response events broadcast back
//  4. UI updates in real-time
//
// # Usage in API
//
// External clients use POST /api/send:
//
//  1. Request includes agent_id or binding info
//  2. Service creates/retrieves thread
//  3. Response streamed as SSE
package conversation
