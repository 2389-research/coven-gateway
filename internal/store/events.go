// ABOUTME: Ledger event store for conversation history and audit trail
// ABOUTME: Provides Event struct with actor attribution and CRUD operations

package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrEventNotFound is returned when a requested event does not exist
var ErrEventNotFound = errors.New("event not found")

// EventDirection indicates whether an event is inbound (to agent) or outbound (from agent)
type EventDirection string

const (
	EventDirectionInbound  EventDirection = "inbound_to_agent"
	EventDirectionOutbound EventDirection = "outbound_from_agent"
)

// EventType categorizes the kind of event
type EventType string

const (
	EventTypeMessage    EventType = "message"
	EventTypeToolCall   EventType = "tool_call"
	EventTypeToolResult EventType = "tool_result"
	EventTypeSystem     EventType = "system"
	EventTypeError      EventType = "error"
)

// GetEventsParams specifies the parameters for retrieving events from the history store.
type GetEventsParams struct {
	ConversationKey string     // Required: the conversation to fetch events from
	Since           *time.Time // Optional: only events at or after this timestamp
	Until           *time.Time // Optional: only events at or before this timestamp
	Limit           int        // 1-500, defaults to 50
	Cursor          string     // Opaque cursor from a previous response for pagination
}

// GetEventsResult contains the results of a GetEvents query.
type GetEventsResult struct {
	Events     []LedgerEvent // The events returned by the query
	NextCursor string        // Opaque cursor for fetching the next page, empty if no more
	HasMore    bool          // True if there are more events after this page
}

// LedgerEvent represents a normalized event in the conversation ledger.
// All inbound and outbound messages are stored here for audit and history.
type LedgerEvent struct {
	ID              string
	ConversationKey string         // e.g., "matrix:!room:server.com", "{agent_id}" for thread-based conversations
	ThreadID        *string        // optional: links event to a thread for efficient queries
	Direction       EventDirection // inbound_to_agent or outbound_from_agent
	Author          string         // user/client/agent identifier
	Timestamp       time.Time
	Type            EventType
	Text            *string // optional message text
	RawTransport    *string // optional: "matrix", "slack", "tui"
	RawPayloadRef   *string // optional pointer to stored raw JSON

	// Actor attribution - who originated this event
	ActorPrincipalID *string // principal_id of the authenticated entity
	ActorMemberID    *string // member_id if principal is linked to a member (nullable in v1)
}

// SaveEvent persists a ledger event to the database
func (s *SQLiteStore) SaveEvent(ctx context.Context, event *LedgerEvent) error {
	query := `
		INSERT INTO ledger_events (
			event_id, conversation_key, thread_id, direction, author, timestamp, type, text,
			raw_transport, raw_payload_ref, actor_principal_id, actor_member_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.ExecContext(ctx, query,
		event.ID,
		event.ConversationKey,
		event.ThreadID,
		string(event.Direction),
		event.Author,
		event.Timestamp.Format(time.RFC3339),
		string(event.Type),
		event.Text,
		event.RawTransport,
		event.RawPayloadRef,
		event.ActorPrincipalID,
		event.ActorMemberID,
	)
	if err != nil {
		return fmt.Errorf("inserting event: %w", err)
	}

	s.logger.Debug("saved ledger event",
		"event_id", event.ID,
		"conversation_key", event.ConversationKey,
		"thread_id", event.ThreadID,
		"type", event.Type,
	)
	return nil
}

// GetEvent retrieves a single event by ID
func (s *SQLiteStore) GetEvent(ctx context.Context, id string) (*LedgerEvent, error) {
	query := `
		SELECT event_id, conversation_key, thread_id, direction, author, timestamp, type, text,
		       raw_transport, raw_payload_ref, actor_principal_id, actor_member_id
		FROM ledger_events
		WHERE event_id = ?
	`

	event := &LedgerEvent{}
	var timestampStr string
	var direction, eventType string

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&event.ID,
		&event.ConversationKey,
		&event.ThreadID,
		&direction,
		&event.Author,
		&timestampStr,
		&eventType,
		&event.Text,
		&event.RawTransport,
		&event.RawPayloadRef,
		&event.ActorPrincipalID,
		&event.ActorMemberID,
	)

	if err == sql.ErrNoRows {
		return nil, ErrEventNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying event: %w", err)
	}

	event.Direction = EventDirection(direction)
	event.Type = EventType(eventType)
	event.Timestamp, err = time.Parse(time.RFC3339, timestampStr)
	if err != nil {
		return nil, fmt.Errorf("parsing timestamp: %w", err)
	}

	return event, nil
}

// ListEventsByConversation retrieves events for a conversation key, ordered by timestamp ASC
func (s *SQLiteStore) ListEventsByConversation(ctx context.Context, conversationKey string, limit int) ([]*LedgerEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	query := `
		SELECT event_id, conversation_key, thread_id, direction, author, timestamp, type, text,
		       raw_transport, raw_payload_ref, actor_principal_id, actor_member_id
		FROM ledger_events
		WHERE conversation_key = ?
		ORDER BY timestamp ASC
		LIMIT ?
	`

	return s.queryEvents(ctx, query, conversationKey, limit)
}

// ListEventsByActor retrieves events created by a specific principal
func (s *SQLiteStore) ListEventsByActor(ctx context.Context, principalID string, limit int) ([]*LedgerEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	query := `
		SELECT event_id, conversation_key, thread_id, direction, author, timestamp, type, text,
		       raw_transport, raw_payload_ref, actor_principal_id, actor_member_id
		FROM ledger_events
		WHERE actor_principal_id = ?
		ORDER BY timestamp ASC
		LIMIT ?
	`

	return s.queryEvents(ctx, query, principalID, limit)
}

// ListEventsByActorDesc retrieves events created by a specific principal, ordered newest first
func (s *SQLiteStore) ListEventsByActorDesc(ctx context.Context, principalID string, limit int) ([]*LedgerEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	query := `
		SELECT event_id, conversation_key, thread_id, direction, author, timestamp, type, text,
		       raw_transport, raw_payload_ref, actor_principal_id, actor_member_id
		FROM ledger_events
		WHERE actor_principal_id = ?
		ORDER BY timestamp DESC
		LIMIT ?
	`

	return s.queryEvents(ctx, query, principalID, limit)
}

// queryEvents is a helper that executes a query and returns events
func (s *SQLiteStore) queryEvents(ctx context.Context, query string, args ...any) ([]*LedgerEvent, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying events: %w", err)
	}
	defer rows.Close()

	var events []*LedgerEvent
	for rows.Next() {
		event := &LedgerEvent{}
		var timestampStr string
		var direction, eventType string

		if err := rows.Scan(
			&event.ID,
			&event.ConversationKey,
			&event.ThreadID,
			&direction,
			&event.Author,
			&timestampStr,
			&eventType,
			&event.Text,
			&event.RawTransport,
			&event.RawPayloadRef,
			&event.ActorPrincipalID,
			&event.ActorMemberID,
		); err != nil {
			return nil, fmt.Errorf("scanning event row: %w", err)
		}

		event.Direction = EventDirection(direction)
		event.Type = EventType(eventType)
		event.Timestamp, err = time.Parse(time.RFC3339, timestampStr)
		if err != nil {
			return nil, fmt.Errorf("parsing timestamp: %w", err)
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating event rows: %w", err)
	}

	return events, nil
}

// encodeCursor creates an opaque cursor string from a timestamp and event ID.
// Format is base64(timestamp_rfc3339|event_id)
func encodeCursor(ts time.Time, id string) string {
	data := fmt.Sprintf("%s|%s", ts.Format(time.RFC3339), id)
	return base64.StdEncoding.EncodeToString([]byte(data))
}

// decodeCursor parses an opaque cursor string into a timestamp and event ID.
// Returns an error if the cursor is invalid.
func decodeCursor(cursor string) (time.Time, string, error) {
	decoded, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("invalid cursor encoding: %w", err)
	}

	parts := strings.SplitN(string(decoded), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, "", fmt.Errorf("invalid cursor format: expected timestamp|event_id")
	}

	ts, err := time.Parse(time.RFC3339, parts[0])
	if err != nil {
		return time.Time{}, "", fmt.Errorf("invalid cursor timestamp: %w", err)
	}

	return ts, parts[1], nil
}

// GetEvents retrieves events for a conversation with pagination support.
// Events are returned in chronological order (oldest first).
func (s *SQLiteStore) GetEvents(ctx context.Context, p GetEventsParams) (*GetEventsResult, error) {
	// Validate required params
	if p.ConversationKey == "" {
		return nil, errors.New("conversation_key required")
	}

	// Apply default and cap limit
	if p.Limit <= 0 {
		p.Limit = 50
	}
	if p.Limit > 500 {
		p.Limit = 500
	}

	// Parse cursor if provided
	var cursorTS time.Time
	var cursorID string
	if p.Cursor != "" {
		var err error
		cursorTS, cursorID, err = decodeCursor(p.Cursor)
		if err != nil {
			return nil, fmt.Errorf("invalid cursor: %w", err)
		}
	}

	// Build the query dynamically based on which parameters are set
	var args []any
	query := `
		SELECT event_id, conversation_key, thread_id, direction, author, timestamp, type, text,
		       raw_transport, raw_payload_ref, actor_principal_id, actor_member_id
		FROM ledger_events
		WHERE conversation_key = ?
	`
	args = append(args, p.ConversationKey)

	// Add Since filter
	if p.Since != nil {
		query += ` AND timestamp >= ?`
		args = append(args, p.Since.Format(time.RFC3339))
	}

	// Add Until filter
	if p.Until != nil {
		query += ` AND timestamp <= ?`
		args = append(args, p.Until.Format(time.RFC3339))
	}

	// Add cursor-based pagination
	if p.Cursor != "" {
		query += ` AND (timestamp > ? OR (timestamp = ? AND event_id > ?))`
		args = append(args, cursorTS.Format(time.RFC3339), cursorTS.Format(time.RFC3339), cursorID)
	}

	// Order by timestamp, then event_id for deterministic pagination
	query += ` ORDER BY timestamp ASC, event_id ASC`

	// Fetch limit+1 to detect if there are more results
	query += ` LIMIT ?`
	args = append(args, p.Limit+1)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying events: %w", err)
	}
	defer rows.Close()

	var events []LedgerEvent
	for rows.Next() {
		var event LedgerEvent
		var timestampStr string
		var direction, eventType string

		if err := rows.Scan(
			&event.ID,
			&event.ConversationKey,
			&event.ThreadID,
			&direction,
			&event.Author,
			&timestampStr,
			&eventType,
			&event.Text,
			&event.RawTransport,
			&event.RawPayloadRef,
			&event.ActorPrincipalID,
			&event.ActorMemberID,
		); err != nil {
			return nil, fmt.Errorf("scanning event row: %w", err)
		}

		event.Direction = EventDirection(direction)
		event.Type = EventType(eventType)
		event.Timestamp, err = time.Parse(time.RFC3339, timestampStr)
		if err != nil {
			return nil, fmt.Errorf("parsing timestamp: %w", err)
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating event rows: %w", err)
	}

	// Determine if there are more results
	hasMore := len(events) > p.Limit
	if hasMore {
		events = events[:p.Limit] // Trim to requested limit
	}

	// Build result
	result := &GetEventsResult{
		Events:  events,
		HasMore: hasMore,
	}

	// Set next cursor if there are more results
	if hasMore && len(events) > 0 {
		lastEvent := events[len(events)-1]
		result.NextCursor = encodeCursor(lastEvent.Timestamp, lastEvent.ID)
	}

	return result, nil
}

// GetEventsByThreadID retrieves the most recent events for a thread, ordered
// chronologically (ASC). Uses a DESC subquery to pick the N most recent rows,
// then re-orders ASC so callers receive events in conversation order.
func (s *SQLiteStore) GetEventsByThreadID(ctx context.Context, threadID string, limit int) ([]*LedgerEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	query := `
		SELECT event_id, conversation_key, thread_id, direction, author, timestamp, type, text,
		       raw_transport, raw_payload_ref, actor_principal_id, actor_member_id
		FROM (
			SELECT event_id, conversation_key, thread_id, direction, author, timestamp, type, text,
			       raw_transport, raw_payload_ref, actor_principal_id, actor_member_id
			FROM ledger_events
			WHERE thread_id = ?
			ORDER BY timestamp DESC
			LIMIT ?
		)
		ORDER BY timestamp ASC
	`

	return s.queryEvents(ctx, query, threadID, limit)
}

// EventToMessage converts a LedgerEvent to the legacy Message format.
// This provides a single conversion point for all code that needs to
// display events as messages.
func EventToMessage(evt *LedgerEvent) *Message {
	msg := &Message{
		ID:        evt.ID,
		Sender:    evt.Author,
		CreatedAt: evt.Timestamp,
	}

	if evt.ThreadID != nil {
		msg.ThreadID = *evt.ThreadID
	}

	if evt.Text != nil {
		msg.Content = *evt.Text
	}

	// Map event type to message type and parse tool metadata from JSON
	switch evt.Type {
	case EventTypeToolCall:
		msg.Type = MessageTypeToolUse
		// Parse tool metadata from JSON text if available
		if evt.Text != nil {
			var toolData struct {
				Name  string `json:"name"`
				ID    string `json:"id"`
				Input string `json:"input"`
			}
			if err := json.Unmarshal([]byte(*evt.Text), &toolData); err == nil {
				msg.ToolName = toolData.Name
				msg.ToolID = toolData.ID
				msg.Content = toolData.Input
			}
		}
	case EventTypeToolResult:
		msg.Type = MessageTypeToolResult
		// Parse tool result from JSON text if available
		if evt.Text != nil {
			var resultData struct {
				ID     string `json:"id"`
				Output string `json:"output"`
			}
			if err := json.Unmarshal([]byte(*evt.Text), &resultData); err == nil {
				msg.ToolID = resultData.ID
				msg.Content = resultData.Output
			}
		}
	default:
		msg.Type = MessageTypeMessage
	}

	return msg
}

// EventsToMessages converts a slice of LedgerEvents to Messages.
func EventsToMessages(events []*LedgerEvent) []*Message {
	messages := make([]*Message, len(events))
	for i, evt := range events {
		messages[i] = EventToMessage(evt)
	}
	return messages
}
