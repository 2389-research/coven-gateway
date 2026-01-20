// ABOUTME: MCP token store for mapping tokens to agent capabilities.
// ABOUTME: Tokens are generated during agent registration and validated on MCP requests.

package mcp

import (
	"sync"

	"github.com/google/uuid"
)

// TokenStore manages MCP access tokens and their associated capabilities.
// Tokens are created when agents register and invalidated when they disconnect.
type TokenStore struct {
	mu     sync.RWMutex
	tokens map[string][]string // token -> capabilities
}

// NewTokenStore creates a new token store.
func NewTokenStore() *TokenStore {
	return &TokenStore{
		tokens: make(map[string][]string),
	}
}

// CreateToken generates a new token for the given capabilities.
// Returns the token string that should be included in MCP URLs.
func (s *TokenStore) CreateToken(capabilities []string) string {
	token := uuid.New().String()

	// Copy capabilities to avoid aliasing
	caps := make([]string, len(capabilities))
	copy(caps, capabilities)

	s.mu.Lock()
	s.tokens[token] = caps
	s.mu.Unlock()

	return token
}

// GetCapabilities returns the capabilities for a token, or nil if not found.
func (s *TokenStore) GetCapabilities(token string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	caps, ok := s.tokens[token]
	if !ok {
		return nil
	}

	// Return a copy to prevent modification
	result := make([]string, len(caps))
	copy(result, caps)
	return result
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
