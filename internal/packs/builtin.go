// ABOUTME: Built-in tool support for tools that execute in-process.
// ABOUTME: Allows gateway-embedded tools to coexist with external packs.

package packs

import (
	"context"
	"encoding/json"

	pb "github.com/2389/coven-gateway/proto/coven"
)

// ToolHandler is a function that executes a built-in tool.
// It receives the calling agent's ID and the tool input as JSON.
// Returns the result as JSON or an error.
type ToolHandler func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error)

// BuiltinTool represents a tool that executes in the gateway process.
type BuiltinTool struct {
	Definition *pb.ToolDefinition
	Handler    ToolHandler
}

// BuiltinPack is a collection of built-in tools with a pack ID.
type BuiltinPack struct {
	ID    string
	Tools []*BuiltinTool
}

// builtinEntry stores a builtin tool with its pack ID for registry lookup.
type builtinEntry struct {
	Tool   *BuiltinTool
	PackID string
}
