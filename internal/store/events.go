// ABOUTME: Ledger event store for conversation history and audit trail
// ABOUTME: Provides Event struct with actor attribution and CRUD operations

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

// LedgerEvent represents a normalized event in the conversation ledger.
// All inbound and outbound messages are stored here for audit and history.
type LedgerEvent struct {
	ID              string
	ConversationKey string         // e.g., "matrix:!room:server.com", "tui:client:pane"
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
			event_id, conversation_key, direction, author, timestamp, type, text,
			raw_transport, raw_payload_ref, actor_principal_id, actor_member_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.ExecContext(ctx, query,
		event.ID,
		event.ConversationKey,
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
		"type", event.Type,
	)
	return nil
}

// GetEvent retrieves a single event by ID
func (s *SQLiteStore) GetEvent(ctx context.Context, id string) (*LedgerEvent, error) {
	query := `
		SELECT event_id, conversation_key, direction, author, timestamp, type, text,
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
		SELECT event_id, conversation_key, direction, author, timestamp, type, text,
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
		SELECT event_id, conversation_key, direction, author, timestamp, type, text,
		       raw_transport, raw_payload_ref, actor_principal_id, actor_member_id
		FROM ledger_events
		WHERE actor_principal_id = ?
		ORDER BY timestamp ASC
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
