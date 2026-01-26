// ABOUTME: ClientService gRPC handler for streaming conversation events
// ABOUTME: Implements StreamEvents RPC for real-time event streaming to clients

package client

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/2389/coven-gateway/internal/store"
	pb "github.com/2389/coven-gateway/proto/coven"
)

const (
	// Polling interval for new events when streaming
	streamPollInterval = 100 * time.Millisecond
	// Maximum time to keep a stream open without new events
	streamIdleTimeout = 5 * time.Minute
	// Default limit for initial history fetch
	defaultHistoryLimit = 50
)

// StreamEvents implements real-time event streaming for a conversation.
// It first replays historical events (if since_event_id is provided) then
// streams new events as they arrive.
func (s *ClientService) StreamEvents(req *pb.StreamEventsRequest, stream pb.ClientService_StreamEventsServer) error {
	if req.ConversationKey == "" {
		return status.Error(codes.InvalidArgument, "conversation_key required")
	}

	ctx := stream.Context()

	// Track cursor for pagination (opaque string from the store)
	var cursor string

	// If resuming from a specific event, we need to find its timestamp to build a cursor
	if req.SinceEventId != nil && *req.SinceEventId != "" {
		// Find the event to get its timestamp for cursor construction
		foundCursor, err := s.findCursorForEvent(ctx, req.ConversationKey, *req.SinceEventId)
		if err != nil {
			return err
		}
		cursor = foundCursor
	}

	// Send initial events (either from cursor position or recent history)
	newCursor, err := s.sendInitialEvents(ctx, stream, req.ConversationKey, cursor)
	if err != nil {
		return err
	}
	cursor = newCursor

	// Poll for new events
	ticker := time.NewTicker(streamPollInterval)
	defer ticker.Stop()

	idleTimer := time.NewTimer(streamIdleTimeout)
	defer idleTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			// Client disconnected
			return nil
		case <-idleTimer.C:
			// Stream has been idle too long - close gracefully
			return nil
		case <-ticker.C:
			newCursor, hasNew, err := s.pollAndSendNewEvents(ctx, stream, req.ConversationKey, cursor)
			if err != nil {
				return err
			}
			if hasNew {
				cursor = newCursor
				// Reset idle timer on activity
				if !idleTimer.Stop() {
					select {
					case <-idleTimer.C:
					default:
					}
				}
				idleTimer.Reset(streamIdleTimeout)
			}
		}
	}
}

// findCursorForEvent looks up an event by ID and constructs a cursor for pagination.
// Returns an encoded cursor that can be used with GetEvents.
func (s *ClientService) findCursorForEvent(ctx context.Context, convKey, eventID string) (string, error) {
	// Fetch events from the beginning to find the one we want
	// This is not efficient for large histories, but works for now
	params := store.GetEventsParams{
		ConversationKey: convKey,
		Limit:           500, // Reasonable batch to search
	}

	result, err := s.store.GetEvents(ctx, params)
	if err != nil {
		return "", status.Error(codes.Internal, "failed to find event")
	}

	// Find the event in the results
	for _, event := range result.Events {
		if event.ID == eventID {
			// Found it - construct a cursor from its timestamp and ID
			return encodeCursor(event.Timestamp, event.ID), nil
		}
	}

	// Event not found - start from beginning
	return "", nil
}

// encodeCursor creates an opaque cursor string from a timestamp and event ID.
func encodeCursor(ts time.Time, id string) string {
	data := fmt.Sprintf("%s|%s", ts.Format(time.RFC3339), id)
	return base64.StdEncoding.EncodeToString([]byte(data))
}

// sendInitialEvents sends initial events for the stream.
// If cursor is provided, fetches events after that point.
// Otherwise fetches recent history.
// Returns the cursor for the next page, or empty string if no events.
func (s *ClientService) sendInitialEvents(ctx context.Context, stream pb.ClientService_StreamEventsServer, convKey, cursor string) (string, error) {
	// Check context before fetching
	select {
	case <-ctx.Done():
		return "", nil
	default:
	}

	limit := defaultHistoryLimit
	if cursor != "" {
		limit = 100 // Larger batch when resuming
	}

	params := store.GetEventsParams{
		ConversationKey: convKey,
		Cursor:          cursor,
		Limit:           limit,
	}

	result, err := s.store.GetEvents(ctx, params)
	if err != nil {
		// If context was cancelled, return gracefully
		if ctx.Err() != nil {
			return "", nil
		}
		return "", status.Error(codes.Internal, "failed to fetch events")
	}

	for _, event := range result.Events {
		streamEvent := eventToClientStreamEvent(&event)
		if err := stream.Send(streamEvent); err != nil {
			return "", err
		}
	}

	return result.NextCursor, nil
}

// pollAndSendNewEvents checks for new events after cursor and sends them.
// Returns the new cursor, whether new events were found, and any error.
func (s *ClientService) pollAndSendNewEvents(ctx context.Context, stream pb.ClientService_StreamEventsServer, convKey, cursor string) (string, bool, error) {
	// Check context before polling
	select {
	case <-ctx.Done():
		return cursor, false, nil
	default:
	}

	params := store.GetEventsParams{
		ConversationKey: convKey,
		Cursor:          cursor,
		Limit:           50, // Poll batch size
	}

	result, err := s.store.GetEvents(ctx, params)
	if err != nil {
		// If context was cancelled, return gracefully
		if ctx.Err() != nil {
			return cursor, false, nil
		}
		return cursor, false, status.Error(codes.Internal, "failed to poll for events")
	}

	if len(result.Events) == 0 {
		return cursor, false, nil
	}

	for _, event := range result.Events {
		streamEvent := eventToClientStreamEvent(&event)
		if err := stream.Send(streamEvent); err != nil {
			return cursor, false, err
		}
	}

	// Use the next cursor for future polling
	nextCursor := result.NextCursor
	if nextCursor == "" && len(result.Events) > 0 {
		// No next cursor but we got events - build cursor from last event
		lastEvent := result.Events[len(result.Events)-1]
		nextCursor = encodeCursor(lastEvent.Timestamp, lastEvent.ID)
	}

	return nextCursor, true, nil
}

// decodeCursor parses an opaque cursor string (unused but kept for reference)
func decodeCursor(cursor string) (time.Time, string, error) {
	decoded, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("invalid cursor encoding: %w", err)
	}

	parts := strings.SplitN(string(decoded), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, "", fmt.Errorf("invalid cursor format")
	}

	ts, err := time.Parse(time.RFC3339, parts[0])
	if err != nil {
		return time.Time{}, "", fmt.Errorf("invalid cursor timestamp: %w", err)
	}

	return ts, parts[1], nil
}

// eventToClientStreamEvent converts a ledger event to a ClientStreamEvent proto.
func eventToClientStreamEvent(e *store.LedgerEvent) *pb.ClientStreamEvent {
	streamEvent := &pb.ClientStreamEvent{
		ConversationKey: e.ConversationKey,
		Timestamp:       e.Timestamp.Format(time.RFC3339),
	}

	// Wrap the full event in the Event payload
	streamEvent.Payload = &pb.ClientStreamEvent_Event{
		Event: toProtoEvent(e),
	}

	return streamEvent
}
