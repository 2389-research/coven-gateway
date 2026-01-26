// ABOUTME: Tests for the pack registry including registration, collision detection, and capability filtering.
// ABOUTME: Validates thread-safe operations and tool lookup functionality.

package packs

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"

	pb "github.com/2389/coven-gateway/proto/coven"
)

// createTestManifest creates a PackManifest for testing.
func createTestManifest(packID, version string, tools ...*pb.ToolDefinition) *pb.PackManifest {
	return &pb.PackManifest{
		PackId:  packID,
		Version: version,
		Tools:   tools,
	}
}

// createTestTool creates a ToolDefinition for testing.
func createTestTool(name, description string, requiredCaps ...string) *pb.ToolDefinition {
	return &pb.ToolDefinition{
		Name:                 name,
		Description:          description,
		InputSchemaJson:      `{"type": "object"}`,
		RequiredCapabilities: requiredCaps,
	}
}

func TestRegistryRegisterPack(t *testing.T) {
	t.Run("registers pack successfully", func(t *testing.T) {
		registry := NewRegistry(slog.Default())
		manifest := createTestManifest("pack-1", "1.0.0",
			createTestTool("tool-a", "Tool A description"),
			createTestTool("tool-b", "Tool B description"),
		)

		err := registry.RegisterPack("pack-1", manifest)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify pack is registered
		pack := registry.GetPack("pack-1")
		if pack == nil {
			t.Fatal("expected pack to be registered")
		}
		if pack.ID != "pack-1" {
			t.Errorf("expected ID 'pack-1', got '%s'", pack.ID)
		}
		if pack.Version != "1.0.0" {
			t.Errorf("expected version '1.0.0', got '%s'", pack.Version)
		}
		if len(pack.Tools) != 2 {
			t.Errorf("expected 2 tools, got %d", len(pack.Tools))
		}
	})

	t.Run("returns error for duplicate pack ID", func(t *testing.T) {
		registry := NewRegistry(slog.Default())
		manifest := createTestManifest("pack-1", "1.0.0",
			createTestTool("tool-a", "Tool A"),
		)

		err := registry.RegisterPack("pack-1", manifest)
		if err != nil {
			t.Fatalf("unexpected error on first register: %v", err)
		}

		// Try to register again with same ID
		manifest2 := createTestManifest("pack-1", "2.0.0",
			createTestTool("tool-c", "Tool C"),
		)
		err = registry.RegisterPack("pack-1", manifest2)
		if err == nil {
			t.Error("expected error for duplicate pack ID")
		}
		if !errors.Is(err, ErrPackAlreadyRegistered) {
			t.Errorf("expected ErrPackAlreadyRegistered, got %v", err)
		}
	})

	t.Run("creates channel for pack", func(t *testing.T) {
		registry := NewRegistry(slog.Default())
		manifest := createTestManifest("pack-1", "1.0.0",
			createTestTool("tool-a", "Tool A"),
		)

		registry.RegisterPack("pack-1", manifest)
		pack := registry.GetPack("pack-1")

		if pack.Channel == nil {
			t.Fatal("expected channel to be created")
		}

		// Verify channel is usable
		select {
		case pack.Channel <- &pb.ExecuteToolRequest{ToolName: "test"}:
			// Good, channel accepts writes
		default:
			t.Error("channel should accept writes")
		}
	})
}

func TestRegistryUnregisterPack(t *testing.T) {
	t.Run("unregisters existing pack", func(t *testing.T) {
		registry := NewRegistry(slog.Default())
		manifest := createTestManifest("pack-1", "1.0.0",
			createTestTool("tool-a", "Tool A"),
		)

		registry.RegisterPack("pack-1", manifest)
		registry.UnregisterPack("pack-1")

		pack := registry.GetPack("pack-1")
		if pack != nil {
			t.Error("expected pack to be removed")
		}
	})

	t.Run("removes tools when pack is unregistered", func(t *testing.T) {
		registry := NewRegistry(slog.Default())
		manifest := createTestManifest("pack-1", "1.0.0",
			createTestTool("tool-a", "Tool A"),
			createTestTool("tool-b", "Tool B"),
		)

		registry.RegisterPack("pack-1", manifest)
		registry.UnregisterPack("pack-1")

		// Verify tools are removed
		tool, pack := registry.GetToolByName("tool-a")
		if tool != nil || pack != nil {
			t.Error("expected tools to be removed")
		}
	})

	t.Run("closes channel when pack is unregistered", func(t *testing.T) {
		registry := NewRegistry(slog.Default())
		manifest := createTestManifest("pack-1", "1.0.0",
			createTestTool("tool-a", "Tool A"),
		)

		registry.RegisterPack("pack-1", manifest)
		pack := registry.GetPack("pack-1")
		channel := pack.Channel

		registry.UnregisterPack("pack-1")

		// Verify channel is closed
		_, ok := <-channel
		if ok {
			t.Error("expected channel to be closed")
		}
	})

	t.Run("unregistering non-existent pack is no-op", func(t *testing.T) {
		registry := NewRegistry(slog.Default())

		// Should not panic
		registry.UnregisterPack("non-existent")
	})
}

func TestRegistryToolCollisionDetection(t *testing.T) {
	t.Run("detects tool name collision", func(t *testing.T) {
		registry := NewRegistry(slog.Default())

		manifest1 := createTestManifest("pack-1", "1.0.0",
			createTestTool("shared-tool", "Tool from pack 1"),
		)
		manifest2 := createTestManifest("pack-2", "1.0.0",
			createTestTool("shared-tool", "Tool from pack 2"),
		)

		err := registry.RegisterPack("pack-1", manifest1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		err = registry.RegisterPack("pack-2", manifest2)
		if err == nil {
			t.Error("expected error for tool collision")
		}
		if !errors.Is(err, ErrToolCollision) {
			t.Errorf("expected ErrToolCollision, got %v", err)
		}
	})

	t.Run("collision error includes tool and pack info", func(t *testing.T) {
		registry := NewRegistry(slog.Default())

		manifest1 := createTestManifest("pack-1", "1.0.0",
			createTestTool("my-special-tool", "Tool from pack 1"),
		)
		manifest2 := createTestManifest("pack-2", "1.0.0",
			createTestTool("my-special-tool", "Tool from pack 2"),
		)

		registry.RegisterPack("pack-1", manifest1)
		err := registry.RegisterPack("pack-2", manifest2)

		if err == nil {
			t.Fatal("expected error")
		}

		errStr := err.Error()
		if errStr == "" {
			t.Error("expected error message to contain details")
		}
	})

	t.Run("allows same tool name after pack unregistered", func(t *testing.T) {
		registry := NewRegistry(slog.Default())

		manifest1 := createTestManifest("pack-1", "1.0.0",
			createTestTool("reusable-tool", "Tool from pack 1"),
		)
		manifest2 := createTestManifest("pack-2", "1.0.0",
			createTestTool("reusable-tool", "Tool from pack 2"),
		)

		registry.RegisterPack("pack-1", manifest1)
		registry.UnregisterPack("pack-1")

		// Now pack-2 should be able to register with the same tool name
		err := registry.RegisterPack("pack-2", manifest2)
		if err != nil {
			t.Errorf("expected registration to succeed after unregister, got: %v", err)
		}
	})

	t.Run("no partial registration on collision", func(t *testing.T) {
		registry := NewRegistry(slog.Default())

		manifest1 := createTestManifest("pack-1", "1.0.0",
			createTestTool("tool-a", "Tool A"),
		)
		manifest2 := createTestManifest("pack-2", "1.0.0",
			createTestTool("tool-b", "Tool B - unique"),
			createTestTool("tool-a", "Tool A - collision"),
			createTestTool("tool-c", "Tool C - unique"),
		)

		registry.RegisterPack("pack-1", manifest1)
		err := registry.RegisterPack("pack-2", manifest2)

		if err == nil {
			t.Fatal("expected collision error")
		}

		// Verify pack-2 was not registered
		pack := registry.GetPack("pack-2")
		if pack != nil {
			t.Error("pack-2 should not be registered on collision")
		}

		// Verify tool-b and tool-c were not registered
		toolB, _ := registry.GetToolByName("tool-b")
		if toolB != nil {
			t.Error("tool-b should not be registered on collision")
		}
	})
}

func TestRegistryGetToolByName(t *testing.T) {
	t.Run("returns tool and pack when exists", func(t *testing.T) {
		registry := NewRegistry(slog.Default())
		manifest := createTestManifest("pack-1", "1.0.0",
			createTestTool("my-tool", "My tool description"),
		)

		registry.RegisterPack("pack-1", manifest)

		tool, pack := registry.GetToolByName("my-tool")
		if tool == nil {
			t.Fatal("expected to find tool")
		}
		if pack == nil {
			t.Fatal("expected to find pack")
		}
		if tool.Definition.GetName() != "my-tool" {
			t.Errorf("expected 'my-tool', got '%s'", tool.Definition.GetName())
		}
		if tool.PackID != "pack-1" {
			t.Errorf("expected pack ID 'pack-1', got '%s'", tool.PackID)
		}
		if pack.ID != "pack-1" {
			t.Errorf("expected pack ID 'pack-1', got '%s'", pack.ID)
		}
	})

	t.Run("returns nil when tool not found", func(t *testing.T) {
		registry := NewRegistry(slog.Default())

		tool, pack := registry.GetToolByName("non-existent")
		if tool != nil {
			t.Error("expected nil tool")
		}
		if pack != nil {
			t.Error("expected nil pack")
		}
	})
}

func TestRegistryGetAllTools(t *testing.T) {
	t.Run("returns empty list when no tools", func(t *testing.T) {
		registry := NewRegistry(slog.Default())

		tools := registry.GetAllTools()
		if len(tools) != 0 {
			t.Errorf("expected 0 tools, got %d", len(tools))
		}
	})

	t.Run("returns all tools from all packs", func(t *testing.T) {
		registry := NewRegistry(slog.Default())

		manifest1 := createTestManifest("pack-1", "1.0.0",
			createTestTool("tool-a", "Tool A"),
			createTestTool("tool-b", "Tool B"),
		)
		manifest2 := createTestManifest("pack-2", "1.0.0",
			createTestTool("tool-c", "Tool C"),
		)

		registry.RegisterPack("pack-1", manifest1)
		registry.RegisterPack("pack-2", manifest2)

		tools := registry.GetAllTools()
		if len(tools) != 3 {
			t.Errorf("expected 3 tools, got %d", len(tools))
		}
	})
}

func TestRegistryCapabilityFiltering(t *testing.T) {
	t.Run("returns tools with no required capabilities", func(t *testing.T) {
		registry := NewRegistry(slog.Default())
		manifest := createTestManifest("pack-1", "1.0.0",
			createTestTool("tool-a", "Tool A"), // no capabilities required
		)

		registry.RegisterPack("pack-1", manifest)

		tools := registry.GetToolsForCapabilities([]string{})
		if len(tools) != 1 {
			t.Errorf("expected 1 tool, got %d", len(tools))
		}
	})

	t.Run("filters tools by single capability", func(t *testing.T) {
		registry := NewRegistry(slog.Default())
		manifest := createTestManifest("pack-1", "1.0.0",
			createTestTool("tool-public", "Public tool"),
			createTestTool("tool-admin", "Admin tool", "admin"),
		)

		registry.RegisterPack("pack-1", manifest)

		// Without admin capability
		tools := registry.GetToolsForCapabilities([]string{})
		if len(tools) != 1 {
			t.Errorf("expected 1 tool without admin, got %d", len(tools))
		}
		if tools[0].GetName() != "tool-public" {
			t.Errorf("expected 'tool-public', got '%s'", tools[0].GetName())
		}

		// With admin capability
		tools = registry.GetToolsForCapabilities([]string{"admin"})
		if len(tools) != 2 {
			t.Errorf("expected 2 tools with admin, got %d", len(tools))
		}
	})

	t.Run("requires ALL capabilities", func(t *testing.T) {
		registry := NewRegistry(slog.Default())
		manifest := createTestManifest("pack-1", "1.0.0",
			createTestTool("tool-super", "Super tool", "admin", "superuser"),
		)

		registry.RegisterPack("pack-1", manifest)

		// With only admin
		tools := registry.GetToolsForCapabilities([]string{"admin"})
		if len(tools) != 0 {
			t.Errorf("expected 0 tools with only admin, got %d", len(tools))
		}

		// With only superuser
		tools = registry.GetToolsForCapabilities([]string{"superuser"})
		if len(tools) != 0 {
			t.Errorf("expected 0 tools with only superuser, got %d", len(tools))
		}

		// With both
		tools = registry.GetToolsForCapabilities([]string{"admin", "superuser"})
		if len(tools) != 1 {
			t.Errorf("expected 1 tool with both caps, got %d", len(tools))
		}
	})

	t.Run("extra capabilities do not exclude tools", func(t *testing.T) {
		registry := NewRegistry(slog.Default())
		manifest := createTestManifest("pack-1", "1.0.0",
			createTestTool("tool-basic", "Basic tool", "read"),
		)

		registry.RegisterPack("pack-1", manifest)

		// Agent has more capabilities than required
		tools := registry.GetToolsForCapabilities([]string{"read", "write", "admin"})
		if len(tools) != 1 {
			t.Errorf("expected 1 tool, got %d", len(tools))
		}
	})

	t.Run("filters across multiple packs", func(t *testing.T) {
		registry := NewRegistry(slog.Default())

		manifest1 := createTestManifest("pack-1", "1.0.0",
			createTestTool("tool-a", "Tool A", "cap1"),
		)
		manifest2 := createTestManifest("pack-2", "1.0.0",
			createTestTool("tool-b", "Tool B", "cap2"),
		)
		manifest3 := createTestManifest("pack-3", "1.0.0",
			createTestTool("tool-c", "Tool C", "cap1", "cap2"),
		)

		registry.RegisterPack("pack-1", manifest1)
		registry.RegisterPack("pack-2", manifest2)
		registry.RegisterPack("pack-3", manifest3)

		// With cap1 only
		tools := registry.GetToolsForCapabilities([]string{"cap1"})
		if len(tools) != 1 {
			t.Errorf("expected 1 tool with cap1, got %d", len(tools))
		}

		// With cap1 and cap2
		tools = registry.GetToolsForCapabilities([]string{"cap1", "cap2"})
		if len(tools) != 3 {
			t.Errorf("expected 3 tools with cap1+cap2, got %d", len(tools))
		}
	})
}

func TestRegistryListPacks(t *testing.T) {
	t.Run("returns empty list when no packs", func(t *testing.T) {
		registry := NewRegistry(slog.Default())

		packs := registry.ListPacks()
		if len(packs) != 0 {
			t.Errorf("expected 0 packs, got %d", len(packs))
		}
	})

	t.Run("returns all registered packs", func(t *testing.T) {
		registry := NewRegistry(slog.Default())

		for i := 1; i <= 3; i++ {
			manifest := createTestManifest(
				fmt.Sprintf("pack-%d", i),
				fmt.Sprintf("%d.0.0", i),
				createTestTool(fmt.Sprintf("tool-%d", i), "Tool"),
			)
			registry.RegisterPack(fmt.Sprintf("pack-%d", i), manifest)
		}

		packs := registry.ListPacks()
		if len(packs) != 3 {
			t.Errorf("expected 3 packs, got %d", len(packs))
		}
	})

	t.Run("includes tool names in pack info", func(t *testing.T) {
		registry := NewRegistry(slog.Default())
		manifest := createTestManifest("pack-1", "1.0.0",
			createTestTool("tool-a", "Tool A"),
			createTestTool("tool-b", "Tool B"),
		)

		registry.RegisterPack("pack-1", manifest)

		packs := registry.ListPacks()
		if len(packs) != 1 {
			t.Fatalf("expected 1 pack, got %d", len(packs))
		}
		if len(packs[0].ToolNames) != 2 {
			t.Errorf("expected 2 tool names, got %d", len(packs[0].ToolNames))
		}
	})
}

func TestRegistryConcurrentAccess(t *testing.T) {
	t.Run("handles concurrent register and list", func(t *testing.T) {
		registry := NewRegistry(slog.Default())
		var wg sync.WaitGroup

		// Concurrent registrations
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				manifest := createTestManifest(
					fmt.Sprintf("pack-%d", id),
					"1.0.0",
					createTestTool(fmt.Sprintf("tool-%d", id), "Tool"),
				)
				registry.RegisterPack(fmt.Sprintf("pack-%d", id), manifest)
			}(i)
		}

		// Concurrent list operations
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				registry.ListPacks()
			}()
		}

		// Concurrent tool lookups
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				registry.GetToolByName(fmt.Sprintf("tool-%d", id))
			}(i)
		}

		wg.Wait()
	})

	t.Run("handles concurrent register and unregister", func(t *testing.T) {
		registry := NewRegistry(slog.Default())
		var wg sync.WaitGroup

		// Pre-register some packs
		for i := 0; i < 5; i++ {
			manifest := createTestManifest(
				fmt.Sprintf("existing-%d", i),
				"1.0.0",
				createTestTool(fmt.Sprintf("existing-tool-%d", i), "Tool"),
			)
			registry.RegisterPack(fmt.Sprintf("existing-%d", i), manifest)
		}

		// Concurrent registrations
		for i := 5; i < 15; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				manifest := createTestManifest(
					fmt.Sprintf("pack-%d", id),
					"1.0.0",
					createTestTool(fmt.Sprintf("tool-%d", id), "Tool"),
				)
				registry.RegisterPack(fmt.Sprintf("pack-%d", id), manifest)
			}(i)
		}

		// Concurrent unregistrations
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				registry.UnregisterPack(fmt.Sprintf("existing-%d", id))
			}(i)
		}

		wg.Wait()
	})

	t.Run("handles concurrent capability filtering", func(t *testing.T) {
		registry := NewRegistry(slog.Default())

		// Register packs with various capabilities
		for i := 0; i < 5; i++ {
			caps := []string{}
			if i%2 == 0 {
				caps = append(caps, "even")
			}
			if i%3 == 0 {
				caps = append(caps, "divisible-by-3")
			}
			manifest := createTestManifest(
				fmt.Sprintf("pack-%d", i),
				"1.0.0",
				createTestTool(fmt.Sprintf("tool-%d", i), "Tool", caps...),
			)
			registry.RegisterPack(fmt.Sprintf("pack-%d", i), manifest)
		}

		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				caps := []string{}
				if id%2 == 0 {
					caps = append(caps, "even")
				}
				if id%3 == 0 {
					caps = append(caps, "divisible-by-3")
				}
				registry.GetToolsForCapabilities(caps)
			}(i)
		}

		wg.Wait()
	})
}
