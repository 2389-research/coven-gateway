// Package client provides a Go client for interacting with coven-gateway.
//
// # Overview
//
// The client package provides programmatic access to the gateway's API,
// supporting both gRPC (agent connections) and HTTP (message sending, management).
//
// # Use Cases
//
// Primary uses:
//
//   - Testing: Integration tests that need to interact with the gateway
//   - CLI tools: coven-admin uses this for gateway management
//   - Custom clients: Building automation or custom frontends
//
// # HTTP Client
//
// For HTTP API interactions:
//
//	client := client.NewHTTPClient("http://localhost:8080")
//
//	// Send a message and stream response
//	events, err := client.Send(ctx, &SendRequest{
//	    AgentID: "agent-123",
//	    Message: "Hello!",
//	    Sender:  "user@example.com",
//	})
//	for event := range events {
//	    switch event.Type {
//	    case "text":
//	        fmt.Print(event.Text)
//	    case "done":
//	        break
//	    }
//	}
//
// # gRPC Client
//
// For agent-like connections:
//
//	conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
//	client := pb.NewCovenControlClient(conn)
//	stream, err := client.AgentStream(ctx)
//
// # Event Types
//
// The client handles all SSE event types:
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
// HTTP requests can include authentication:
//
//	client.SetToken("jwt-token-here")
//
// Or per-request:
//
//	client.Send(ctx, req, WithAuth("Bearer token"))
//
// # Error Handling
//
// The client distinguishes between:
//
//   - Network errors (connection refused, timeout)
//   - API errors (400 bad request, 401 unauthorized)
//   - Stream errors (unexpected disconnect)
//
// Example:
//
//	events, err := client.Send(ctx, req)
//	if err != nil {
//	    if client.IsNetworkError(err) {
//	        // Retry logic
//	    }
//	    return err
//	}
package client
