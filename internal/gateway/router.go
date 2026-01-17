// ABOUTME: Router connects bindings to message routing for fold-gateway
// ABOUTME: Resolves frontend+channel to agent, verifying agent is online

package gateway

import (
	"context"
	"errors"
	"fmt"

	"github.com/2389/fold-gateway/internal/store"
)

// Router errors
var (
	// ErrNoRoute means no binding exists for this frontend+channel
	ErrNoRoute = errors.New("no route for channel")

	// ErrAgentOffline means the bound agent is not connected
	ErrAgentOffline = errors.New("agent is offline")
)

// BindingStore provides access to channel-to-agent bindings
type BindingStore interface {
	GetBindingByChannel(ctx context.Context, frontend, channelID string) (*store.Binding, error)
}

// AgentChecker checks whether an agent is currently connected
type AgentChecker interface {
	IsOnline(agentID string) bool
}

// Router routes messages from frontends to agents based on channel bindings
type Router struct {
	bindings BindingStore
	agents   AgentChecker
}

// NewRouter creates a new Router with the given binding store and agent checker
func NewRouter(bindings BindingStore, agents AgentChecker) *Router {
	return &Router{
		bindings: bindings,
		agents:   agents,
	}
}

// Route resolves a frontend+channel to an agent ID.
// Returns ErrNoRoute if no binding exists for the channel.
// Returns ErrAgentOffline if the bound agent is not currently connected.
func (r *Router) Route(ctx context.Context, frontend, channelID string) (string, error) {
	binding, err := r.bindings.GetBindingByChannel(ctx, frontend, channelID)
	if errors.Is(err, store.ErrBindingNotFound) {
		return "", ErrNoRoute
	}
	if err != nil {
		return "", fmt.Errorf("lookup binding: %w", err)
	}

	// Verify agent is online
	if !r.agents.IsOnline(binding.AgentID) {
		return "", ErrAgentOffline
	}

	return binding.AgentID, nil
}
