// Package mcp implements the Model Context Protocol server for external tool access.
//
// # Overview
//
// MCP (Model Context Protocol) is a standard for AI tool integration. This package
// provides an MCP-compatible HTTP server that exposes gateway tools to external
// AI clients (like Claude Desktop, other LLMs, or custom applications).
//
// # Protocol
//
// The MCP server uses JSON-RPC 2.0 over Streamable HTTP transport. All requests
// are sent via POST to a single endpoint:
//
//   - POST /mcp - JSON-RPC requests (initialize, tools/list, tools/call, etc.)
//
// The server does not support server-initiated SSE streams (GET returns 405).
// Responses are returned directly in the POST response body.
//
// # Authentication
//
// The server uses token-based authentication:
//
//	Authorization: Bearer <token>
//
// Tokens are managed via the TokenStore and map to principals with specific
// capabilities. Only tools matching the principal's capabilities are exposed.
//
// # Tool Discovery
//
// Clients call tools/list to discover available tools:
//
//	{
//	  "jsonrpc": "2.0",
//	  "method": "tools/list",
//	  "id": 1
//	}
//
// Response includes tool schemas in JSON Schema format.
//
// # Tool Execution
//
// Clients call tools/call to execute a tool:
//
//	{
//	  "jsonrpc": "2.0",
//	  "method": "tools/call",
//	  "params": {
//	    "name": "todo_add",
//	    "arguments": {"description": "Buy groceries"}
//	  },
//	  "id": 2
//	}
//
// Results are returned in the response body.
//
// # Architecture
//
// Components:
//
//   - Server: HTTP server handling MCP requests
//   - TokenStore: Maps access tokens to principals
//   - Handler: Processes JSON-RPC methods
//
// # Usage
//
// Create and start the MCP server:
//
//	cfg := &mcp.Config{
//	    TokenStore:   tokenStore,
//	    PackRouter:   packRouter,
//	    Logger:       logger,
//	}
//	server := mcp.NewServer(cfg)
//	server.RegisterRoutes(router.PathPrefix("/mcp").Subrouter())
//
// Generate an access token:
//
//	token, err := tokenStore.CreateToken(principalID, capabilities)
//
// # Integration with Claude Desktop
//
// Add to Claude Desktop's MCP configuration:
//
//	{
//	  "mcpServers": {
//	    "coven": {
//	      "url": "http://localhost:8080/mcp",
//	      "authorization": "Bearer <token>"
//	    }
//	  }
//	}
package mcp
