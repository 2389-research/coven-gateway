// ABOUTME: Store interface and data types for coven-gateway persistence
// ABOUTME: Defines Thread, Message structs and the Store interface for database operations

package store

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when a requested entity does not exist
var ErrNotFound = errors.New("not found")

// ErrDuplicateThread is returned when trying to create a thread that already exists
var ErrDuplicateThread = errors.New("thread already exists")

// Thread represents a conversation thread linking a frontend conversation to an agent
type Thread struct {
	ID           string
	FrontendName string
	ExternalID   string
	AgentID      string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// MessageType constants for message types
const (
	MessageTypeMessage    = "message"     // Regular text message
	MessageTypeToolUse    = "tool_use"    // Tool invocation
	MessageTypeToolResult = "tool_result" // Tool result
)

// Message represents a single message within a thread for audit/history purposes
type Message struct {
	ID        string
	ThreadID  string
	Sender    string
	Content   string
	Type      string // "message", "tool_use", "tool_result" (defaults to "message")
	ToolName  string // For tool_use: name of the tool being called
	ToolID    string // Links tool_use to its corresponding tool_result
	CreatedAt time.Time
}

// ChannelBinding represents a sticky assignment of a frontend channel to an agent
type ChannelBinding struct {
	FrontendName string
	ChannelID    string
	AgentID      string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// LogEntry represents an activity log entry
type LogEntry struct {
	ID        string
	AgentID   string
	Message   string
	Tags      []string
	CreatedAt time.Time
}

// Todo represents a task
type Todo struct {
	ID          string
	AgentID     string
	Description string
	Status      string // pending, in_progress, completed
	Priority    string // low, medium, high
	Notes       string
	DueDate     *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// BBSPost represents a bulletin board post or reply
type BBSPost struct {
	ID        string
	AgentID   string
	ThreadID  string // empty for top-level threads
	Subject   string // required for threads, empty for replies
	Content   string
	CreatedAt time.Time
}

// BBSThread is a post with its replies
type BBSThread struct {
	Post    *BBSPost
	Replies []*BBSPost
}

// AgentMail represents a message between agents
type AgentMail struct {
	ID          string
	FromAgentID string
	ToAgentID   string
	Subject     string
	Content     string
	ReadAt      *time.Time
	CreatedAt   time.Time
}

// AgentNote represents a key-value note for an agent
type AgentNote struct {
	ID        string
	AgentID   string
	Key       string
	Value     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Store defines the interface for thread and message persistence
type Store interface {
	// Threads
	CreateThread(ctx context.Context, thread *Thread) error
	GetThread(ctx context.Context, id string) (*Thread, error)
	GetThreadByFrontendID(ctx context.Context, frontendName, externalID string) (*Thread, error)
	UpdateThread(ctx context.Context, thread *Thread) error
	ListThreads(ctx context.Context, limit int) ([]*Thread, error)

	// Messages (for audit/history)
	SaveMessage(ctx context.Context, msg *Message) error
	GetThreadMessages(ctx context.Context, threadID string, limit int) ([]*Message, error)

	// Agent state (optional, for future use)
	SaveAgentState(ctx context.Context, agentID string, state []byte) error
	GetAgentState(ctx context.Context, agentID string) ([]byte, error)

	// Channel bindings (legacy - use V2 methods below for new code)
	CreateBinding(ctx context.Context, binding *ChannelBinding) error
	GetBinding(ctx context.Context, frontend, channelID string) (*ChannelBinding, error)
	ListBindings(ctx context.Context) ([]*ChannelBinding, error)
	DeleteBinding(ctx context.Context, frontend, channelID string) error

	// Bindings V2 (validated against principals table)
	CreateBindingV2(ctx context.Context, binding *Binding) error
	GetBindingByChannel(ctx context.Context, frontend, channelID string) (*Binding, error)
	ListBindingsV2(ctx context.Context, filter BindingFilter) ([]Binding, error)
	DeleteBindingByID(ctx context.Context, id string) error
	DeleteBindingByChannel(ctx context.Context, frontend, channelID string) error

	// Ledger events
	SaveEvent(ctx context.Context, event *LedgerEvent) error
	GetEvent(ctx context.Context, id string) (*LedgerEvent, error)
	ListEventsByConversation(ctx context.Context, conversationKey string, limit int) ([]*LedgerEvent, error)
	ListEventsByActor(ctx context.Context, principalID string, limit int) ([]*LedgerEvent, error)
	ListEventsByActorDesc(ctx context.Context, principalID string, limit int) ([]*LedgerEvent, error)
	GetEvents(ctx context.Context, params GetEventsParams) (*GetEventsResult, error)

	// Close releases any resources held by the store
	Close() error
}

// BuiltinStore defines methods for built-in tool pack data
type BuiltinStore interface {
	// Log entries
	CreateLogEntry(ctx context.Context, entry *LogEntry) error
	SearchLogEntries(ctx context.Context, query string, since *time.Time, limit int) ([]*LogEntry, error)

	// Todos
	CreateTodo(ctx context.Context, todo *Todo) error
	GetTodo(ctx context.Context, id string) (*Todo, error)
	ListTodos(ctx context.Context, agentID string, status, priority string) ([]*Todo, error)
	UpdateTodo(ctx context.Context, todo *Todo) error
	DeleteTodo(ctx context.Context, id string) error

	// BBS
	CreateBBSPost(ctx context.Context, post *BBSPost) error
	GetBBSPost(ctx context.Context, id string) (*BBSPost, error)
	ListBBSThreads(ctx context.Context, limit int) ([]*BBSPost, error)
	GetBBSThread(ctx context.Context, threadID string) (*BBSThread, error)

	// Mail
	SendMail(ctx context.Context, mail *AgentMail) error
	GetMail(ctx context.Context, id string) (*AgentMail, error)
	ListInbox(ctx context.Context, agentID string, unreadOnly bool, limit int) ([]*AgentMail, error)
	MarkMailRead(ctx context.Context, id string) error

	// Notes
	SetNote(ctx context.Context, note *AgentNote) error
	GetNote(ctx context.Context, agentID, key string) (*AgentNote, error)
	ListNotes(ctx context.Context, agentID string) ([]*AgentNote, error)
	DeleteNote(ctx context.Context, agentID, key string) error
}
