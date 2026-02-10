// ABOUTME: MCP token store for mapping tokens to agent capabilities.
// ABOUTME: Tokens are generated during agent registration and validated on MCP requests.

package mcp

import (
	"sync"

	"github.com/google/uuid"
)

// TokenInfo stores the agent ID and capabilities associated with a token.
type TokenInfo struct {
	AgentID      string
	Capabilities []string
}

// TokenStore manages MCP access tokens and their associated capabilities.
// Tokens are created when agents register and invalidated when they disconnect.
type TokenStore struct {
	mu     sync.RWMutex
	tokens map[string]*TokenInfo // token -> TokenInfo
}

// NewTokenStore creates a new token store.
func NewTokenStore() *TokenStore {
	return &TokenStore{
		tokens: make(map[string]*TokenInfo),
	}
}

// CreateToken generates a new token for the given agent and capabilities.
// Returns the token string that should be included in MCP URLs.
func (s *TokenStore) CreateToken(agentID string, capabilities []string) string {
	token := uuid.New().String()

	// Copy capabilities to avoid aliasing
	caps := make([]string, len(capabilities))
	copy(caps, capabilities)

	s.mu.Lock()
	s.tokens[token] = &TokenInfo{
		AgentID:      agentID,
		Capabilities: caps,
	}
	s.mu.Unlock()

	return token
}

// GetTokenInfo returns the token info (agent ID and capabilities), or nil if not found.
func (s *TokenStore) GetTokenInfo(token string) *TokenInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	info, ok := s.tokens[token]
	if !ok {
		return nil
	}

	// Return a copy to prevent modification
	caps := make([]string, len(info.Capabilities))
	copy(caps, info.Capabilities)
	return &TokenInfo{
		AgentID:      info.AgentID,
		Capabilities: caps,
	}
}

// GetCapabilities returns the capabilities for a token, or nil if not found.
//
// Deprecated: Use GetTokenInfo for full information including agent ID.
func (s *TokenStore) GetCapabilities(token string) []string {
	info := s.GetTokenInfo(token)
	if info == nil {
		return nil
	}
	return info.Capabilities
}

// InvalidateToken removes a token from the store.
// Called when an agent disconnects.
func (s *TokenStore) InvalidateToken(token string) {
	s.mu.Lock()
	delete(s.tokens, token)
	s.mu.Unlock()
}

// TokenCount returns the number of active tokens (for monitoring).
func (s *TokenStore) TokenCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.tokens)
}
