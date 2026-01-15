// ABOUTME: Store interface and data types for fold-gateway persistence
// ABOUTME: Defines Thread, Message structs and the Store interface for database operations

package store

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when a requested entity does not exist
var ErrNotFound = errors.New("not found")

// Thread represents a conversation thread linking a frontend conversation to an agent
type Thread struct {
	ID           string
	FrontendName string
	ExternalID   string
	AgentID      string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Message represents a single message within a thread for audit/history purposes
type Message struct {
	ID        string
	ThreadID  string
	Sender    string
	Content   string
	CreatedAt time.Time
}

// Store defines the interface for thread and message persistence
type Store interface {
	// Threads
	CreateThread(ctx context.Context, thread *Thread) error
	GetThread(ctx context.Context, id string) (*Thread, error)
	UpdateThread(ctx context.Context, thread *Thread) error

	// Messages (for audit/history)
	SaveMessage(ctx context.Context, msg *Message) error
	GetThreadMessages(ctx context.Context, threadID string, limit int) ([]*Message, error)

	// Agent state (optional, for future use)
	SaveAgentState(ctx context.Context, agentID string, state []byte) error
	GetAgentState(ctx context.Context, agentID string) ([]byte, error)

	// Close releases any resources held by the store
	Close() error
}
