// ABOUTME: Simple round-robin router for selecting agents to handle requests.
// ABOUTME: Phase 1 implementation - selects agents in a rotating fashion.

package agent

import (
	"errors"
	"sync/atomic"
)

// ErrNoAgentsAvailable indicates no agents are available to handle a request.
var ErrNoAgentsAvailable = errors.New("no agents available")

// Router selects agents using a round-robin strategy.
type Router struct {
	current uint64
}

// NewRouter creates a new Router instance.
func NewRouter() *Router {
	return &Router{
		current: 0,
	}
}

// SelectAgent picks an agent from the available pool using round-robin selection.
// Returns ErrNoAgentsAvailable if no agents are provided.
func (r *Router) SelectAgent(agents []*Connection) (*Connection, error) {
	if len(agents) == 0 {
		return nil, ErrNoAgentsAvailable
	}

	// Atomically increment and get the index
	idx := atomic.AddUint64(&r.current, 1) - 1
	selected := agents[idx%uint64(len(agents))]

	return selected, nil
}
