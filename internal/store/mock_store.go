// ABOUTME: Mock Store implementation for testing
// ABOUTME: Allows tests to run without SQLite

package store

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// MockStore is an in-memory Store implementation for testing.
type MockStore struct {
	mu          sync.RWMutex
	threads     map[string]*Thread         // keyed by thread ID
	threadIndex map[string]string          // keyed by "frontendName:externalID" -> thread ID
	messages    map[string][]*Message      // keyed by threadID
	bindings    map[string]*ChannelBinding // keyed by "frontend:channelID" (legacy)
	bindingsV2  map[string]*Binding        // keyed by "frontend:channelID" (V2)
	agentState  map[string][]byte          // keyed by agentID
	events      map[string]*LedgerEvent    // keyed by event ID
	usage       map[string]*TokenUsage     // keyed by usage ID
	usageByReq  map[string]string          // keyed by request_id -> usage ID
}

// NewMockStore creates a new MockStore.
func NewMockStore() *MockStore {
	return &MockStore{
		threads:     make(map[string]*Thread),
		threadIndex: make(map[string]string),
		messages:    make(map[string][]*Message),
		bindings:    make(map[string]*ChannelBinding),
		bindingsV2:  make(map[string]*Binding),
		agentState:  make(map[string][]byte),
		events:      make(map[string]*LedgerEvent),
		usage:       make(map[string]*TokenUsage),
		usageByReq:  make(map[string]string),
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

// ListThreads retrieves threads ordered by most recent activity.
func (m *MockStore) ListThreads(ctx context.Context, limit int) ([]*Thread, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	// Collect all threads
	threads := make([]*Thread, 0, len(m.threads))
	for _, t := range m.threads {
		threadCopy := *t
		threads = append(threads, &threadCopy)
	}

	// Sort by UpdatedAt descending
	for i := range len(threads) - 1 {
		for j := i + 1; j < len(threads); j++ {
			if threads[j].UpdatedAt.After(threads[i].UpdatedAt) {
				threads[i], threads[j] = threads[j], threads[i]
			}
		}
	}

	// Apply limit
	if len(threads) > limit {
		threads = threads[:limit]
	}

	return threads, nil
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

// CreateBindingV2 creates a V2 binding (interface method).
func (m *MockStore) CreateBindingV2(ctx context.Context, binding *Binding) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := binding.Frontend + ":" + binding.ChannelID
	if _, exists := m.bindingsV2[key]; exists {
		return ErrDuplicateChannel
	}

	b := *binding
	m.bindingsV2[key] = &b
	return nil
}

// GetBindingByChannel retrieves a V2 binding by frontend and channel ID.
func (m *MockStore) GetBindingByChannel(ctx context.Context, frontend, channelID string) (*Binding, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := frontend + ":" + channelID
	b, ok := m.bindingsV2[key]
	if !ok {
		return nil, ErrBindingNotFound
	}

	// Return a copy
	result := *b
	return &result, nil
}

// DeleteBindingByID deletes a V2 binding by ID.
func (m *MockStore) DeleteBindingByID(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Find binding by ID
	for key, b := range m.bindingsV2 {
		if b.ID == id {
			delete(m.bindingsV2, key)
			return nil
		}
	}
	return ErrBindingNotFound
}

// ListBindingsV2 returns V2 bindings matching the filter criteria.
func (m *MockStore) ListBindingsV2(ctx context.Context, filter BindingFilter) ([]Binding, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []Binding
	for _, b := range m.bindingsV2 {
		// Apply filters
		if filter.Frontend != nil && b.Frontend != *filter.Frontend {
			continue
		}
		if filter.AgentID != nil && b.AgentID != *filter.AgentID {
			continue
		}
		result = append(result, *b)
	}
	return result, nil
}

// DeleteBindingByChannel deletes a V2 binding by frontend and channel_id.
func (m *MockStore) DeleteBindingByChannel(ctx context.Context, frontend, channelID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := frontend + ":" + channelID
	if _, ok := m.bindingsV2[key]; !ok {
		return ErrBindingNotFound
	}
	delete(m.bindingsV2, key)
	return nil
}

// AddBindingV2 stores a V2 binding (for test setup, alias for CreateBindingV2 without duplicate check).
func (m *MockStore) AddBindingV2(ctx context.Context, binding *Binding) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := binding.Frontend + ":" + binding.ChannelID
	b := *binding
	m.bindingsV2[key] = &b
	return nil
}

// SaveEvent stores a ledger event.
func (m *MockStore) SaveEvent(ctx context.Context, event *LedgerEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Make a copy to avoid external modification
	e := *event
	m.events[e.ID] = &e

	return nil
}

// GetEvent retrieves a ledger event by ID.
func (m *MockStore) GetEvent(ctx context.Context, id string) (*LedgerEvent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	e, ok := m.events[id]
	if !ok {
		return nil, ErrEventNotFound
	}

	// Return a copy
	result := *e
	return &result, nil
}

// ListEventsByConversation retrieves events for a conversation key, ordered by timestamp ASC.
func (m *MockStore) ListEventsByConversation(ctx context.Context, conversationKey string, limit int) ([]*LedgerEvent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*LedgerEvent
	for _, e := range m.events {
		if e.ConversationKey == conversationKey {
			eventCopy := *e
			result = append(result, &eventCopy)
		}
	}

	// Sort by timestamp ASC to match SQLiteStore behavior
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp.Before(result[j].Timestamp)
	})

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}

	return result, nil
}

// ListEventsByActor retrieves events by actor principal ID.
func (m *MockStore) ListEventsByActor(ctx context.Context, principalID string, limit int) ([]*LedgerEvent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*LedgerEvent
	for _, e := range m.events {
		if e.ActorPrincipalID != nil && *e.ActorPrincipalID == principalID {
			eventCopy := *e
			result = append(result, &eventCopy)
		}
	}

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}

	return result, nil
}

// ListEventsByActorDesc retrieves events created by a specific principal, ordered newest first.
func (m *MockStore) ListEventsByActorDesc(ctx context.Context, principalID string, limit int) ([]*LedgerEvent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*LedgerEvent
	for _, e := range m.events {
		if e.ActorPrincipalID != nil && *e.ActorPrincipalID == principalID {
			eventCopy := *e
			result = append(result, &eventCopy)
		}
	}

	// Sort by timestamp descending (newest first)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp.After(result[j].Timestamp)
	})

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}

	return result, nil
}

// GetEventsByThreadID retrieves the most recent N events for a thread,
// ordered chronologically (ASC). Mirrors SQLite behavior: pick newest N
// by timestamp DESC, then re-order ASC.
func (m *MockStore) GetEventsByThreadID(ctx context.Context, threadID string, limit int) ([]*LedgerEvent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	var result []*LedgerEvent
	for _, e := range m.events {
		// Match by thread_id column
		if e.ThreadID != nil && *e.ThreadID == threadID {
			eventCopy := *e
			result = append(result, &eventCopy)
		}
	}

	// Sort by timestamp DESC, event_id DESC to pick the most recent N
	// (mirrors SQLite tie-breaker)
	sort.Slice(result, func(i, j int) bool {
		if !result[i].Timestamp.Equal(result[j].Timestamp) {
			return result[i].Timestamp.After(result[j].Timestamp)
		}
		return result[i].ID > result[j].ID
	})

	if len(result) > limit {
		result = result[:limit]
	}

	// Re-order ASC for chronological output
	sort.Slice(result, func(i, j int) bool {
		if !result[i].Timestamp.Equal(result[j].Timestamp) {
			return result[i].Timestamp.Before(result[j].Timestamp)
		}
		return result[i].ID < result[j].ID
	})

	return result, nil
}

// normalizeLimit applies default (50) and cap (500) to pagination limit.
func normalizeLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 500 {
		return 500
	}
	return limit
}

// eventMatchesFilters checks if an event matches the GetEventsParams filters.
func eventMatchesFilters(e *LedgerEvent, p GetEventsParams) bool {
	if e.ConversationKey != p.ConversationKey {
		return false
	}
	if p.Since != nil && e.Timestamp.Before(*p.Since) {
		return false
	}
	if p.Until != nil && e.Timestamp.After(*p.Until) {
		return false
	}
	return true
}

// sortEventsByTimestampAndID sorts events by timestamp, then by ID for stable ordering.
func sortEventsByTimestampAndID(events []LedgerEvent) {
	sort.Slice(events, func(i, j int) bool {
		if events[i].Timestamp.Equal(events[j].Timestamp) {
			return events[i].ID < events[j].ID
		}
		return events[i].Timestamp.Before(events[j].Timestamp)
	})
}

// applyCursorToEvents filters events to those after the cursor position.
func applyCursorToEvents(events []LedgerEvent, cursor string) ([]LedgerEvent, error) {
	if cursor == "" {
		return events, nil
	}
	cursorTS, cursorID, err := decodeCursor(cursor)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor: %w", err)
	}
	startIdx := findCursorStartIndex(events, cursorTS, cursorID)
	if startIdx < len(events) {
		return events[startIdx:], nil
	}
	return nil, nil
}

// findCursorStartIndex finds the index of the first event after the cursor.
func findCursorStartIndex(events []LedgerEvent, cursorTS time.Time, cursorID string) int {
	startIdx := 0
	for i, e := range events {
		if e.Timestamp.After(cursorTS) || (e.Timestamp.Equal(cursorTS) && e.ID > cursorID) {
			return i
		}
		startIdx = i + 1
	}
	return startIdx
}

// buildPaginatedResult creates a GetEventsResult with pagination metadata.
func buildPaginatedResult(events []LedgerEvent, limit int) *GetEventsResult {
	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}
	result := &GetEventsResult{
		Events:  events,
		HasMore: hasMore,
	}
	if hasMore && len(events) > 0 {
		lastEvent := events[len(events)-1]
		result.NextCursor = encodeCursor(lastEvent.Timestamp, lastEvent.ID)
	}
	return result
}

// GetEvents retrieves events for a conversation with pagination support.
func (m *MockStore) GetEvents(ctx context.Context, p GetEventsParams) (*GetEventsResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if p.ConversationKey == "" {
		return nil, errors.New("conversation_key required")
	}

	p.Limit = normalizeLimit(p.Limit)

	// Collect matching events
	var matching []LedgerEvent
	for _, e := range m.events {
		if eventMatchesFilters(e, p) {
			matching = append(matching, *e)
		}
	}

	sortEventsByTimestampAndID(matching)

	// Apply cursor pagination
	var err error
	matching, err = applyCursorToEvents(matching, p.Cursor)
	if err != nil {
		return nil, err
	}

	return buildPaginatedResult(matching, p.Limit), nil
}

// Close is a no-op for MockStore.
func (m *MockStore) Close() error {
	return nil
}

// SaveUsage stores a token usage record.
func (m *MockStore) SaveUsage(ctx context.Context, usage *TokenUsage) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Make a copy to avoid external modification
	u := *usage
	m.usage[u.ID] = &u
	m.usageByReq[u.RequestID] = u.ID

	return nil
}

// LinkUsageToMessage updates a usage record with the final message ID.
func (m *MockStore) LinkUsageToMessage(ctx context.Context, requestID, messageID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	usageID, ok := m.usageByReq[requestID]
	if !ok {
		return nil // No-op if usage doesn't exist
	}

	if usage, exists := m.usage[usageID]; exists {
		usage.MessageID = messageID
	}

	return nil
}

// GetThreadUsage retrieves all usage records for a thread.
func (m *MockStore) GetThreadUsage(ctx context.Context, threadID string) ([]*TokenUsage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*TokenUsage
	for _, u := range m.usage {
		if u.ThreadID == threadID {
			usageCopy := *u
			result = append(result, &usageCopy)
		}
	}

	// Sort by created_at ascending
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})

	return result, nil
}

// GetUsageStats returns aggregated usage statistics with optional filters.
func (m *MockStore) GetUsageStats(ctx context.Context, filter UsageFilter) (*UsageStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := &UsageStats{}

	for _, u := range m.usage {
		// Apply filters
		if filter.AgentID != nil && u.AgentID != *filter.AgentID {
			continue
		}
		if filter.Since != nil && u.CreatedAt.Before(*filter.Since) {
			continue
		}
		if filter.Until != nil && !u.CreatedAt.Before(*filter.Until) {
			continue
		}

		stats.TotalInput += int64(u.InputTokens)
		stats.TotalOutput += int64(u.OutputTokens)
		stats.TotalCacheRead += int64(u.CacheReadTokens)
		stats.TotalCacheWrite += int64(u.CacheWriteTokens)
		stats.TotalThinking += int64(u.ThinkingTokens)
		stats.RequestCount++
	}

	stats.TotalTokens = stats.TotalInput + stats.TotalOutput + stats.TotalThinking

	return stats, nil
}

// Verify MockStore implements Store interface at compile time.
var _ Store = (*MockStore)(nil)

// Verify MockStore implements UsageStore interface at compile time.
var _ UsageStore = (*MockStore)(nil)
