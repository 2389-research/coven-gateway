// Package client implements gRPC server handlers for the ClientService.
//
// # Overview
//
// This package provides the server-side implementation of the ClientService
// gRPC service, which handles requests from frontend clients (TUI, web admin,
// bridges). Despite the name, this is a server package - "client" refers to
// the clients that connect to these handlers.
//
// # Service Methods
//
// The ClientService implements these gRPC methods:
//
//   - ListAgents: Returns currently connected agents
//   - SendMessage: Sends a message to an agent and streams responses
//   - StreamEvents: Subscribes to real-time events for a thread
//   - GetHistory: Retrieves conversation history
//   - AnswerQuestion: Responds to agent questions (tool approval, prompts)
//   - ApproveTool: Approves a pending tool execution
//   - Me: Returns the authenticated principal's info
//   - RegisterAgent: Handles agent registration
//
// # Event Types
//
// StreamEvents delivers these event types:
//
//   - started: Response started
//   - thinking: Agent is processing
//   - text: Text chunk
//   - tool_use: Tool invocation
//   - tool_result: Tool completion
//   - tool_state: Tool state update
//   - tool_approval: Tool needs approval
//   - usage: Token usage statistics
//   - done: Response complete
//   - error: Error occurred
//   - canceled: Request canceled
//
// # Authentication
//
// Requests include authentication via gRPC metadata:
//
//	md := metadata.Pairs("authorization", "Bearer <token>")
//	ctx := metadata.NewOutgoingContext(ctx, md)
//
// # Usage
//
// The ClientService is typically created by the gateway:
//
//	service := client.NewClientService(agentManager, store, logger)
//	pb.RegisterClientServiceServer(grpcServer, service)
package client
