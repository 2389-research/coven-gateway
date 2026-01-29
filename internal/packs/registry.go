// ABOUTME: Thread-safe registry for tool packs and their tools in the gateway.
// ABOUTME: Manages pack registration, tool lookup, and capability-based filtering.

package packs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	pb "github.com/2389/coven-gateway/proto/coven"
)

// ErrPackAlreadyRegistered indicates a pack with the same ID is already connected.
var ErrPackAlreadyRegistered = errors.New("pack already registered")

// ErrPackNotFound indicates the specified pack was not found.
var ErrPackNotFound = errors.New("pack not found")

// ErrToolCollision indicates a tool name already exists from another pack.
var ErrToolCollision = errors.New("tool name collision")

// ErrPackClosed indicates the pack's channel has been closed.
var ErrPackClosed = errors.New("pack channel closed")

// Tool wraps a tool definition with its owning pack ID.
type Tool struct {
	Definition *pb.ToolDefinition
	PackID     string
}

// Pack represents a connected tool pack and its tools.
type Pack struct {
	ID      string
	Version string
	Tools   map[string]*Tool            // by tool name
	Channel chan *pb.ExecuteToolRequest // for sending tool calls to pack

	closeMu sync.Mutex // protects closed and Channel close
	closed  bool       // true after Channel is closed
}

// Send sends a request to the pack's channel if not closed.
// Returns ErrPackClosed if the channel has been closed.
func (p *Pack) Send(ctx context.Context, req *pb.ExecuteToolRequest) error {
	p.closeMu.Lock()
	if p.closed {
		p.closeMu.Unlock()
		return ErrPackClosed
	}
	// Send while holding the lock to prevent close during send
	select {
	case p.Channel <- req:
		p.closeMu.Unlock()
		return nil
	case <-ctx.Done():
		p.closeMu.Unlock()
		return ctx.Err()
	}
}

// Close closes the pack's channel and marks it as closed.
// Safe to call multiple times.
func (p *Pack) Close() {
	p.closeMu.Lock()
	defer p.closeMu.Unlock()
	if !p.closed {
		p.closed = true
		close(p.Channel)
	}
}

// Registry maintains the registry of connected packs and their tools.
type Registry struct {
	mu       sync.RWMutex
	packs    map[string]*Pack
	tools    map[string]*Tool         // global tool name -> tool (for collision detection)
	builtins map[string]*builtinEntry // builtin tool name -> builtin entry
	logger   *slog.Logger
}

// NewRegistry creates a new Registry instance.
func NewRegistry(logger *slog.Logger) *Registry {
	return &Registry{
		packs:    make(map[string]*Pack),
		tools:    make(map[string]*Tool),
		builtins: make(map[string]*builtinEntry),
		logger:   logger,
	}
}

// RegisterPack validates and stores a pack and its tools.
// Returns ErrPackAlreadyRegistered if a pack with the same ID exists.
// Returns ErrToolCollision if any tool name already exists from another pack.
func (r *Registry) RegisterPack(packID string, manifest *pb.PackManifest) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if pack already registered
	if _, exists := r.packs[packID]; exists {
		return ErrPackAlreadyRegistered
	}

	// Check for tool name collisions before registering
	for _, toolDef := range manifest.GetTools() {
		if existingTool, exists := r.tools[toolDef.GetName()]; exists {
			return fmt.Errorf("%w: tool '%s' already registered by pack '%s'",
				ErrToolCollision, toolDef.GetName(), existingTool.PackID)
		}
		if _, exists := r.builtins[toolDef.GetName()]; exists {
			return fmt.Errorf("%w: tool '%s' already registered as builtin",
				ErrToolCollision, toolDef.GetName())
		}
	}

	// Create the pack
	pack := &Pack{
		ID:      packID,
		Version: manifest.GetVersion(),
		Tools:   make(map[string]*Tool),
		Channel: make(chan *pb.ExecuteToolRequest, 16),
	}

	// Register all tools
	for _, toolDef := range manifest.GetTools() {
		tool := &Tool{
			Definition: toolDef,
			PackID:     packID,
		}
		pack.Tools[toolDef.GetName()] = tool
		r.tools[toolDef.GetName()] = tool
	}

	r.packs[packID] = pack

	r.logger.Info("=== PACK REGISTERED ===",
		"pack_id", packID,
		"version", manifest.GetVersion(),
		"tool_count", len(manifest.GetTools()),
		"total_packs", len(r.packs),
		"total_tools", len(r.tools),
	)

	return nil
}

// UnregisterPack removes a pack and all its tools from the registry.
func (r *Registry) UnregisterPack(packID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	pack, exists := r.packs[packID]
	if !exists {
		return
	}

	// Remove all tools belonging to this pack
	for toolName := range pack.Tools {
		delete(r.tools, toolName)
	}

	// Close the pack's channel safely
	pack.Close()

	delete(r.packs, packID)

	r.logger.Info("=== PACK UNREGISTERED ===",
		"pack_id", packID,
		"total_packs", len(r.packs),
		"total_tools", len(r.tools),
	)
}

// GetPack retrieves a pack by its ID.
func (r *Registry) GetPack(packID string) *Pack {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.packs[packID]
}

// GetToolByName finds a tool by its name and returns both the tool and its owning pack.
func (r *Registry) GetToolByName(name string) (*Tool, *Pack) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, exists := r.tools[name]
	if !exists {
		return nil, nil
	}

	pack := r.packs[tool.PackID]
	return tool, pack
}

// RegisterBuiltinPack registers a pack of built-in tools that execute in-process.
// Returns error if any tool name collides with existing tools.
func (r *Registry) RegisterBuiltinPack(pack *BuiltinPack) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check for collisions
	for _, tool := range pack.Tools {
		name := tool.Definition.GetName()
		if _, exists := r.tools[name]; exists {
			return fmt.Errorf("%w: tool '%s' already registered by external pack", ErrToolCollision, name)
		}
		if _, exists := r.builtins[name]; exists {
			return fmt.Errorf("%w: tool '%s' already registered as builtin", ErrToolCollision, name)
		}
	}

	// Register all tools
	for _, tool := range pack.Tools {
		r.builtins[tool.Definition.GetName()] = &builtinEntry{
			Tool:   tool,
			PackID: pack.ID,
		}
	}

	r.logger.Info("=== BUILTIN PACK REGISTERED ===",
		"pack_id", pack.ID,
		"tool_count", len(pack.Tools),
	)

	return nil
}

// GetBuiltinTool returns a builtin tool by name, or nil if not found.
func (r *Registry) GetBuiltinTool(name string) *BuiltinTool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if entry, ok := r.builtins[name]; ok {
		return entry.Tool
	}
	return nil
}

// IsBuiltin returns true if the tool name is a builtin tool.
func (r *Registry) IsBuiltin(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.builtins[name]
	return ok
}

// BuiltinPackInfo contains information about a registered builtin pack for display.
type BuiltinPackInfo struct {
	ID    string
	Tools []*BuiltinTool
}

// ListBuiltinPacks returns information about all registered builtin packs.
func (r *Registry) ListBuiltinPacks() []BuiltinPackInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Group tools by pack ID
	packTools := make(map[string][]*BuiltinTool)
	for _, entry := range r.builtins {
		packTools[entry.PackID] = append(packTools[entry.PackID], entry.Tool)
	}

	// Build result
	result := make([]BuiltinPackInfo, 0, len(packTools))
	for packID, tools := range packTools {
		result = append(result, BuiltinPackInfo{
			ID:    packID,
			Tools: tools,
		})
	}
	return result
}

// GetAllTools returns all registered tools.
func (r *Registry) GetAllTools() []*Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]*Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}
	return tools
}

// GetToolsForCapabilities returns tools where the agent has ALL required capabilities.
// If a tool has no required capabilities, it is always included.
// Includes both external pack tools and builtin tools.
func (r *Registry) GetToolsForCapabilities(caps []string) []*pb.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Build a set of agent capabilities for fast lookup
	capSet := make(map[string]struct{}, len(caps))
	for _, cap := range caps {
		capSet[cap] = struct{}{}
	}

	var result []*pb.ToolDefinition

	// External pack tools
	for _, tool := range r.tools {
		if r.hasAllCapabilities(tool.Definition.GetRequiredCapabilities(), capSet) {
			result = append(result, tool.Definition)
		}
	}

	// Builtin tools
	for _, entry := range r.builtins {
		if r.hasAllCapabilities(entry.Tool.Definition.GetRequiredCapabilities(), capSet) {
			result = append(result, entry.Tool.Definition)
		}
	}

	return result
}

// hasAllCapabilities checks if the capability set contains all required capabilities.
func (r *Registry) hasAllCapabilities(required []string, capSet map[string]struct{}) bool {
	for _, req := range required {
		if _, has := capSet[req]; !has {
			return false
		}
	}
	return true
}

// ListPacks returns information about all registered packs.
func (r *Registry) ListPacks() []*PackInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	packs := make([]*PackInfo, 0, len(r.packs))
	for _, pack := range r.packs {
		toolNames := make([]string, 0, len(pack.Tools))
		for name := range pack.Tools {
			toolNames = append(toolNames, name)
		}
		packs = append(packs, &PackInfo{
			ID:        pack.ID,
			Version:   pack.Version,
			ToolNames: toolNames,
		})
	}
	return packs
}

// PackInfo contains public information about a registered pack.
type PackInfo struct {
	ID        string
	Version   string
	ToolNames []string
}

// Close closes all registered packs and clears the registry.
// This should be called during graceful shutdown.
func (r *Registry) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Count before clearing
	packCount := len(r.packs)
	builtinCount := len(r.builtins)

	// Close all pack channels
	for _, pack := range r.packs {
		pack.Close()
	}

	// Clear all maps
	r.packs = make(map[string]*Pack)
	r.tools = make(map[string]*Tool)
	r.builtins = make(map[string]*builtinEntry)

	r.logger.Info("registry closed", "packs_closed", packCount, "builtins_cleared", builtinCount)
}
