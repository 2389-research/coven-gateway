// Package packs provides the tool pack system for extending agent capabilities.
//
// # Overview
//
// Tool packs are collections of related tools that agents can use. The system
// supports both built-in packs (provided by the gateway) and external packs
// (connected via gRPC).
//
// # Architecture
//
// The pack system has three main components:
//
//   - Registry: Tracks all registered packs and their tools
//   - Router: Routes tool calls to the appropriate pack
//   - Built-in packs: Gateway-provided tools (see internal/builtins)
//
// # Built-in Packs
//
// The gateway provides 5 built-in packs with 21 tools:
//
//	builtin:base   - Logging, todos, BBS (requires "base" capability)
//	builtin:notes  - Key-value notes storage (requires "notes" capability)
//	builtin:mail   - Inter-agent messaging (requires "mail" capability)
//	builtin:admin  - Administrative tools (requires "admin" capability)
//	builtin:ui     - User interaction (requires "ui" capability)
//
// # External Packs
//
// External packs connect via gRPC and register their tools:
//
//	pack.Register(conn, packInfo)
//	tools := pack.ListTools()
//
// External packs can be:
//   - MCP servers (via HTTP bridge)
//   - Custom gRPC services
//   - Sidecar processes
//
// # Tool Routing
//
// When an agent calls a tool, the router:
//
//  1. Looks up the tool by name in the registry
//  2. Finds the owning pack
//  3. Routes the call to the pack
//  4. Returns the result to the agent
//
// Tool names are globally unique. Built-in tools use simple names (e.g., "todo_add"),
// while external tools may use qualified names (e.g., "mypack:search").
//
// # Capabilities
//
// Tools require capabilities to use. Agents must have the required capability
// in their principal record. This provides fine-grained access control.
//
// # Usage
//
// Create a registry and router:
//
//	registry := packs.NewRegistry()
//	router := packs.NewRouter(registry, store)
//
// Register built-in packs:
//
//	builtins.RegisterAll(registry, store)
//
// Execute a tool:
//
//	result, err := router.Execute(ctx, agentID, toolName, params)
package packs
