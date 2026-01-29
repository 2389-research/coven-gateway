// ABOUTME: Tests for built-in tool registration and lookup.
// ABOUTME: Verifies builtin tools work alongside external pack tools.

package packs

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	pb "github.com/2389/coven-gateway/proto/coven"
)

func TestRegisterBuiltinPack(t *testing.T) {
	t.Run("registers tools successfully", func(t *testing.T) {
		registry := NewRegistry(slog.Default())

		pack := &BuiltinPack{
			ID: "builtin:base",
			Tools: []*BuiltinTool{
				{
					Definition: &pb.ToolDefinition{
						Name:                 "log_entry",
						Description:          "Log an activity",
						RequiredCapabilities: []string{"base"},
					},
					Handler: func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
						return []byte(`{"ok": true}`), nil
					},
				},
			},
		}

		err := registry.RegisterBuiltinPack(pack)
		if err != nil {
			t.Fatalf("RegisterBuiltinPack: %v", err)
		}

		// Check IsBuiltin
		if !registry.IsBuiltin("log_entry") {
			t.Error("expected log_entry to be builtin")
		}

		// Check GetBuiltinTool
		tool := registry.GetBuiltinTool("log_entry")
		if tool == nil {
			t.Fatal("expected to find log_entry tool")
		}
		if tool.Definition.GetName() != "log_entry" {
			t.Errorf("unexpected name: %s", tool.Definition.GetName())
		}
	})

	t.Run("registers multiple tools in pack", func(t *testing.T) {
		registry := NewRegistry(slog.Default())

		pack := &BuiltinPack{
			ID: "builtin:multi",
			Tools: []*BuiltinTool{
				{
					Definition: &pb.ToolDefinition{Name: "tool_a"},
					Handler: func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
						return nil, nil
					},
				},
				{
					Definition: &pb.ToolDefinition{Name: "tool_b"},
					Handler: func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
						return nil, nil
					},
				},
				{
					Definition: &pb.ToolDefinition{Name: "tool_c"},
					Handler: func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
						return nil, nil
					},
				},
			},
		}

		err := registry.RegisterBuiltinPack(pack)
		if err != nil {
			t.Fatalf("RegisterBuiltinPack: %v", err)
		}

		for _, name := range []string{"tool_a", "tool_b", "tool_c"} {
			if !registry.IsBuiltin(name) {
				t.Errorf("expected %s to be builtin", name)
			}
		}
	})
}

func TestIsBuiltin(t *testing.T) {
	t.Run("returns true for builtin tools", func(t *testing.T) {
		registry := NewRegistry(slog.Default())

		pack := &BuiltinPack{
			ID: "builtin:test",
			Tools: []*BuiltinTool{
				{
					Definition: &pb.ToolDefinition{Name: "my_builtin"},
					Handler: func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
						return nil, nil
					},
				},
			},
		}

		if err := registry.RegisterBuiltinPack(pack); err != nil {
			t.Fatalf("RegisterBuiltinPack: %v", err)
		}

		if !registry.IsBuiltin("my_builtin") {
			t.Error("expected my_builtin to be builtin")
		}
	})

	t.Run("returns false for non-existent tools", func(t *testing.T) {
		registry := NewRegistry(slog.Default())

		if registry.IsBuiltin("non_existent") {
			t.Error("expected non_existent to not be builtin")
		}
	})

	t.Run("returns false for external pack tools", func(t *testing.T) {
		registry := NewRegistry(slog.Default())

		manifest := createTestManifest("pack-1", "1.0.0",
			createTestTool("external_tool", "External tool"),
		)
		if err := registry.RegisterPack("pack-1", manifest); err != nil {
			t.Fatalf("RegisterPack: %v", err)
		}

		if registry.IsBuiltin("external_tool") {
			t.Error("expected external_tool to not be builtin")
		}
	})
}

func TestGetBuiltinTool(t *testing.T) {
	t.Run("returns tool when exists", func(t *testing.T) {
		registry := NewRegistry(slog.Default())

		pack := &BuiltinPack{
			ID: "builtin:test",
			Tools: []*BuiltinTool{
				{
					Definition: &pb.ToolDefinition{
						Name:        "my_tool",
						Description: "My tool description",
					},
					Handler: func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
						return []byte(`"handled"`), nil
					},
				},
			},
		}

		if err := registry.RegisterBuiltinPack(pack); err != nil {
			t.Fatalf("RegisterBuiltinPack: %v", err)
		}

		tool := registry.GetBuiltinTool("my_tool")
		if tool == nil {
			t.Fatal("expected to find tool")
		}
		if tool.Definition.GetDescription() != "My tool description" {
			t.Errorf("unexpected description: %s", tool.Definition.GetDescription())
		}
		if tool.Handler == nil {
			t.Error("expected handler to be set")
		}
	})

	t.Run("returns nil for non-existent tool", func(t *testing.T) {
		registry := NewRegistry(slog.Default())

		tool := registry.GetBuiltinTool("non_existent")
		if tool != nil {
			t.Error("expected nil for non-existent tool")
		}
	})

	t.Run("handler can be invoked", func(t *testing.T) {
		registry := NewRegistry(slog.Default())

		pack := &BuiltinPack{
			ID: "builtin:test",
			Tools: []*BuiltinTool{
				{
					Definition: &pb.ToolDefinition{Name: "echo"},
					Handler: func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
						return input, nil // echo back the input
					},
				},
			},
		}

		if err := registry.RegisterBuiltinPack(pack); err != nil {
			t.Fatalf("RegisterBuiltinPack: %v", err)
		}

		tool := registry.GetBuiltinTool("echo")
		result, err := tool.Handler(context.Background(), "agent-1", json.RawMessage(`{"test": "data"}`))
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if string(result) != `{"test": "data"}` {
			t.Errorf("unexpected result: %s", string(result))
		}
	})
}

func TestBuiltinCapabilityFiltering(t *testing.T) {
	t.Run("returns builtin tools matching capabilities", func(t *testing.T) {
		registry := NewRegistry(slog.Default())

		pack := &BuiltinPack{
			ID: "builtin:mixed",
			Tools: []*BuiltinTool{
				{
					Definition: &pb.ToolDefinition{
						Name:                 "public_tool",
						RequiredCapabilities: []string{"base"},
					},
					Handler: func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
						return nil, nil
					},
				},
				{
					Definition: &pb.ToolDefinition{
						Name:                 "admin_tool",
						RequiredCapabilities: []string{"admin"},
					},
					Handler: func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
						return nil, nil
					},
				},
			},
		}

		err := registry.RegisterBuiltinPack(pack)
		if err != nil {
			t.Fatalf("RegisterBuiltinPack: %v", err)
		}

		// Agent with only base capability
		tools := registry.GetToolsForCapabilities([]string{"base"})
		if len(tools) != 1 {
			t.Fatalf("expected 1 tool for base, got %d", len(tools))
		}
		if tools[0].GetName() != "public_tool" {
			t.Errorf("expected public_tool, got %s", tools[0].GetName())
		}

		// Agent with admin capability
		tools = registry.GetToolsForCapabilities([]string{"admin"})
		if len(tools) != 1 {
			t.Fatalf("expected 1 tool for admin, got %d", len(tools))
		}

		// Agent with both
		tools = registry.GetToolsForCapabilities([]string{"base", "admin"})
		if len(tools) != 2 {
			t.Fatalf("expected 2 tools, got %d", len(tools))
		}
	})

	t.Run("includes builtins with no required capabilities", func(t *testing.T) {
		registry := NewRegistry(slog.Default())

		pack := &BuiltinPack{
			ID: "builtin:test",
			Tools: []*BuiltinTool{
				{
					Definition: &pb.ToolDefinition{Name: "no_cap_tool"},
					Handler: func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
						return nil, nil
					},
				},
			},
		}

		if err := registry.RegisterBuiltinPack(pack); err != nil {
			t.Fatalf("RegisterBuiltinPack: %v", err)
		}

		tools := registry.GetToolsForCapabilities([]string{})
		if len(tools) != 1 {
			t.Fatalf("expected 1 tool, got %d", len(tools))
		}
	})

	t.Run("mixes builtin and external pack tools", func(t *testing.T) {
		registry := NewRegistry(slog.Default())

		// Register external pack
		manifest := createTestManifest("pack-1", "1.0.0",
			createTestTool("external_tool", "External tool", "base"),
		)
		if err := registry.RegisterPack("pack-1", manifest); err != nil {
			t.Fatalf("RegisterPack: %v", err)
		}

		// Register builtin pack
		pack := &BuiltinPack{
			ID: "builtin:test",
			Tools: []*BuiltinTool{
				{
					Definition: &pb.ToolDefinition{
						Name:                 "builtin_tool",
						RequiredCapabilities: []string{"base"},
					},
					Handler: func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
						return nil, nil
					},
				},
			},
		}
		if err := registry.RegisterBuiltinPack(pack); err != nil {
			t.Fatalf("RegisterBuiltinPack: %v", err)
		}

		tools := registry.GetToolsForCapabilities([]string{"base"})
		if len(tools) != 2 {
			t.Fatalf("expected 2 tools (1 external + 1 builtin), got %d", len(tools))
		}
	})
}

func TestBuiltinCollision(t *testing.T) {
	t.Run("rejects duplicate builtin tool names", func(t *testing.T) {
		registry := NewRegistry(slog.Default())

		pack1 := &BuiltinPack{
			ID: "builtin:pack1",
			Tools: []*BuiltinTool{
				{
					Definition: &pb.ToolDefinition{Name: "my_tool"},
					Handler: func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
						return nil, nil
					},
				},
			},
		}

		pack2 := &BuiltinPack{
			ID: "builtin:pack2",
			Tools: []*BuiltinTool{
				{
					Definition: &pb.ToolDefinition{Name: "my_tool"}, // collision!
					Handler: func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
						return nil, nil
					},
				},
			},
		}

		err := registry.RegisterBuiltinPack(pack1)
		if err != nil {
			t.Fatalf("RegisterBuiltinPack pack1: %v", err)
		}

		err = registry.RegisterBuiltinPack(pack2)
		if err == nil {
			t.Fatal("expected collision error")
		}
		if !errors.Is(err, ErrToolCollision) {
			t.Errorf("expected ErrToolCollision, got %v", err)
		}
	})

	t.Run("rejects collision with external pack tool", func(t *testing.T) {
		registry := NewRegistry(slog.Default())

		// Register external pack first
		manifest := createTestManifest("pack-1", "1.0.0",
			createTestTool("shared_tool", "External tool"),
		)
		err := registry.RegisterPack("pack-1", manifest)
		if err != nil {
			t.Fatalf("RegisterPack: %v", err)
		}

		// Try to register builtin with same name
		pack := &BuiltinPack{
			ID: "builtin:test",
			Tools: []*BuiltinTool{
				{
					Definition: &pb.ToolDefinition{Name: "shared_tool"}, // collision!
					Handler: func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
						return nil, nil
					},
				},
			},
		}

		err = registry.RegisterBuiltinPack(pack)
		if err == nil {
			t.Fatal("expected collision error")
		}
		if !errors.Is(err, ErrToolCollision) {
			t.Errorf("expected ErrToolCollision, got %v", err)
		}
	})

	t.Run("external pack rejects collision with builtin", func(t *testing.T) {
		registry := NewRegistry(slog.Default())

		// Register builtin first
		pack := &BuiltinPack{
			ID: "builtin:test",
			Tools: []*BuiltinTool{
				{
					Definition: &pb.ToolDefinition{Name: "shared_tool"},
					Handler: func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
						return nil, nil
					},
				},
			},
		}
		err := registry.RegisterBuiltinPack(pack)
		if err != nil {
			t.Fatalf("RegisterBuiltinPack: %v", err)
		}

		// Try to register external pack with same name
		manifest := createTestManifest("pack-1", "1.0.0",
			createTestTool("shared_tool", "External tool"), // collision!
		)
		err = registry.RegisterPack("pack-1", manifest)
		if err == nil {
			t.Fatal("expected collision error")
		}
		if !errors.Is(err, ErrToolCollision) {
			t.Errorf("expected ErrToolCollision, got %v", err)
		}
	})

	t.Run("no partial registration on collision", func(t *testing.T) {
		registry := NewRegistry(slog.Default())

		// Register a builtin first
		pack1 := &BuiltinPack{
			ID: "builtin:pack1",
			Tools: []*BuiltinTool{
				{
					Definition: &pb.ToolDefinition{Name: "existing_tool"},
					Handler: func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
						return nil, nil
					},
				},
			},
		}
		if err := registry.RegisterBuiltinPack(pack1); err != nil {
			t.Fatalf("RegisterBuiltinPack pack1: %v", err)
		}

		// Try to register a pack where the second tool collides
		pack2 := &BuiltinPack{
			ID: "builtin:pack2",
			Tools: []*BuiltinTool{
				{
					Definition: &pb.ToolDefinition{Name: "unique_tool"},
					Handler: func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
						return nil, nil
					},
				},
				{
					Definition: &pb.ToolDefinition{Name: "existing_tool"}, // collision!
					Handler: func(ctx context.Context, agentID string, input json.RawMessage) (json.RawMessage, error) {
						return nil, nil
					},
				},
			},
		}

		err := registry.RegisterBuiltinPack(pack2)
		if err == nil {
			t.Fatal("expected collision error")
		}

		// unique_tool should NOT be registered due to rollback
		if registry.IsBuiltin("unique_tool") {
			t.Error("unique_tool should not be registered on collision")
		}
	})
}
