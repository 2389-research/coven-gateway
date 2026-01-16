// ABOUTME: Mock Store implementation for testing
// ABOUTME: Allows tests to run without SQLite

package store

import (
	"context"
	"sync"
)

// MockStore is an in-memory Store implementation for testing.
type MockStore struct {
	mu          sync.RWMutex
	threads     map[string]*Thread            // keyed by thread ID
	threadIndex map[string]string             // keyed by "frontendName:externalID" -> thread ID
	messages    map[string][]*Message         // keyed by threadID
	bindings    map[string]*ChannelBinding    // keyed by "frontend:channelID"
	agentState  map[string][]byte             // keyed by agentID
}

// NewMockStore creates a new MockStore.
func NewMockStore() *MockStore {
	return &MockStore{
		threads:     make(map[string]*Thread),
		threadIndex: make(map[string]string),
		messages:    make(map[string][]*Message),
		bindings:    make(map[string]*ChannelBinding),
		agentState:  make(map[string][]byte),
	}
}

// CreateThread stores a new thread.
func (m *MockStore) CreateThread(ctx context.Context, thread *Thread) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Make a copy to avoid external modification
	t := *thread
	m.threads[t.ID] = &t

	// Index by frontend name and external ID
	key := t.FrontendName + ":" + t.ExternalID
	m.threadIndex[key] = t.ID

	return nil
}

// GetThread retrieves a thread by ID.
func (m *MockStore) GetThread(ctx context.Context, id string) (*Thread, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	t, ok := m.threads[id]
	if !ok {
		return nil, ErrNotFound
	}

	// Return a copy
	result := *t
	return &result, nil
}

// GetThreadByFrontendID retrieves a thread by frontend name and external ID.
func (m *MockStore) GetThreadByFrontendID(ctx context.Context, frontendName, externalID string) (*Thread, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := frontendName + ":" + externalID
	threadID, ok := m.threadIndex[key]
	if !ok {
		return nil, ErrNotFound
	}

	t, ok := m.threads[threadID]
	if !ok {
		return nil, ErrNotFound
	}

	// Return a copy
	result := *t
	return &result, nil
}

// UpdateThread updates an existing thread.
func (m *MockStore) UpdateThread(ctx context.Context, thread *Thread) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.threads[thread.ID]; !ok {
		return ErrNotFound
	}

	// Make a copy to avoid external modification
	t := *thread
	m.threads[t.ID] = &t

	return nil
}

// SaveMessage stores a message.
func (m *MockStore) SaveMessage(ctx context.Context, msg *Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Make a copy to avoid external modification
	msgCopy := *msg
	m.messages[msg.ThreadID] = append(m.messages[msg.ThreadID], &msgCopy)

	return nil
}

// GetThreadMessages retrieves messages for a thread, limited by count.
// If limit <= 0, returns all messages.
func (m *MockStore) GetThreadMessages(ctx context.Context, threadID string, limit int) ([]*Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	msgs, ok := m.messages[threadID]
	if !ok {
		return []*Message{}, nil
	}

	// Apply limit
	if limit > 0 && len(msgs) > limit {
		msgs = msgs[len(msgs)-limit:]
	}

	// Return copies
	result := make([]*Message, len(msgs))
	for i, msg := range msgs {
		msgCopy := *msg
		result[i] = &msgCopy
	}

	return result, nil
}

// SaveAgentState saves agent state as bytes.
func (m *MockStore) SaveAgentState(ctx context.Context, agentID string, state []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Make a copy to avoid external modification
	stateCopy := make([]byte, len(state))
	copy(stateCopy, state)
	m.agentState[agentID] = stateCopy

	return nil
}

// GetAgentState retrieves agent state by ID.
func (m *MockStore) GetAgentState(ctx context.Context, agentID string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, ok := m.agentState[agentID]
	if !ok {
		return nil, ErrNotFound
	}

	// Return a copy
	result := make([]byte, len(state))
	copy(result, state)
	return result, nil
}

// CreateBinding creates a new channel binding.
func (m *MockStore) CreateBinding(ctx context.Context, binding *ChannelBinding) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := binding.FrontendName + ":" + binding.ChannelID

	// Make a copy to avoid external modification
	b := *binding
	m.bindings[key] = &b

	return nil
}

// GetBinding retrieves a binding by frontend and channel ID.
func (m *MockStore) GetBinding(ctx context.Context, frontend, channelID string) (*ChannelBinding, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := frontend + ":" + channelID
	b, ok := m.bindings[key]
	if !ok {
		return nil, ErrNotFound
	}

	// Return a copy
	result := *b
	return &result, nil
}

// ListBindings returns all channel bindings.
func (m *MockStore) ListBindings(ctx context.Context) ([]*ChannelBinding, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*ChannelBinding, 0, len(m.bindings))
	for _, b := range m.bindings {
		// Return copies
		bindingCopy := *b
		result = append(result, &bindingCopy)
	}

	return result, nil
}

// DeleteBinding removes a channel binding.
func (m *MockStore) DeleteBinding(ctx context.Context, frontend, channelID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := frontend + ":" + channelID
	if _, ok := m.bindings[key]; !ok {
		return ErrNotFound
	}

	delete(m.bindings, key)
	return nil
}

// Close is a no-op for MockStore.
func (m *MockStore) Close() error {
	return nil
}

// Verify MockStore implements Store interface at compile time.
var _ Store = (*MockStore)(nil)
