// Package gateway orchestrates the coven-gateway server components.
//
// # Overview
//
// The gateway package is the central coordinator of the coven-gateway server.
// It owns and manages all major components: gRPC server, HTTP server, agent
// manager, data store, and web admin interface.
//
// # Gateway Struct
//
// The Gateway struct is the main entry point:
//
//	type Gateway struct {
//	    config           *config.Config
//	    agentManager     *agent.Manager
//	    store            store.Store
//	    conversation     *conversation.Service
//	    grpcServer       *grpc.Server
//	    httpServer       *http.Server
//	    webAdmin         *webadmin.Admin
//	    packRegistry     *packs.Registry
//	    packRouter       *packs.Router
//	    mcpServer        *mcp.Server
//	    questionRouter   *builtins.InMemoryQuestionRouter
//	    eventBroadcaster *conversation.EventBroadcaster
//	    // ... and more
//	}
//
// # HTTP API
//
// The gateway exposes HTTP endpoints in api.go:
//
//   - POST /api/send - Send message to an agent (SSE streaming response)
//   - GET /api/agents - List connected agents
//   - GET /api/threads - List conversation threads
//   - GET /api/bindings - List channel bindings
//   - POST /api/bindings - Create a binding
//   - GET /health - Liveness check
//   - GET /health/ready - Readiness check
//
// # SSE Streaming
//
// Responses are streamed as Server-Sent Events:
//
//	event: thinking
//	data: {"thinking": "..."}
//
//	event: text
//	data: {"text": "Hello!"}
//
//	event: done
//	data: {"request_id": "..."}
//
// Event types: started, thinking, text, tool_use, tool_result, tool_state,
// tool_approval, usage, done, error, canceled, session_init, session_orphaned.
//
// # gRPC Service
//
// The gateway implements the CovenControl gRPC service:
//
//	service CovenControl {
//	    rpc AgentStream(stream AgentMessage) returns (stream ServerMessage);
//	}
//
// Agents connect via bidirectional streaming and maintain long-lived connections.
//
// # Question Routing
//
// The QuestionRouter handles interactive user questions from agents:
//
//  1. Agent calls ask_user tool
//  2. QuestionRouter broadcasts question to connected clients
//  3. Client answers via /api/questions/answer
//  4. Answer is delivered back to the agent
//
// # Event Broadcasting
//
// EventBroadcaster fans out events to all interested clients:
//
//	broadcaster.Subscribe(threadID) -> <-chan Event
//	broadcaster.Publish(threadID, event)
//
// Used for real-time updates in the web admin chat interface.
//
// # Lifecycle
//
// Start the gateway:
//
//	gw, err := gateway.New(cfg)
//	ctx, cancel := context.WithCancel(context.Background())
//	go gw.Run(ctx)
//
// Graceful shutdown:
//
//	cancel()
//	gw.Shutdown(shutdownCtx)
//
// # Key Files
//
//   - gateway.go: Gateway struct, initialization, Run/Shutdown
//   - api.go: HTTP handlers and SSE streaming
//   - grpc.go: gRPC service implementation
//   - question_router.go: Interactive question handling
//   - event_broadcaster.go: Real-time event fanout
package gateway
