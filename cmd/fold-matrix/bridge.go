// ABOUTME: Matrix bridge core for fold-matrix
// ABOUTME: Handles Matrix client connection and message routing to gateway

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// Bridge connects Matrix to fold-gateway.
type Bridge struct {
	config  *Config
	matrix  *mautrix.Client
	gateway *GatewayClient
	logger  *slog.Logger

	// Track rooms we're actively processing to avoid duplicate handling
	processing sync.Map

	// ctx is the parent context for message processing goroutines
	ctx    context.Context
	cancel context.CancelFunc
}

// NewBridge creates a new Matrix bridge.
func NewBridge(cfg *Config, logger *slog.Logger) (*Bridge, error) {
	// Create Matrix client
	client, err := mautrix.NewClient(cfg.Matrix.Homeserver, id.UserID(cfg.Matrix.UserID), cfg.Matrix.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("creating matrix client: %w", err)
	}

	// Create gateway client
	gateway := NewGatewayClient(cfg.Gateway.URL)

	return &Bridge{
		config:  cfg,
		matrix:  client,
		gateway: gateway,
		logger:  logger,
	}, nil
}

// Run starts the bridge and blocks until context is cancelled.
func (b *Bridge) Run(ctx context.Context) error {
	b.logger.Info("starting matrix bridge",
		"homeserver", b.config.Matrix.Homeserver,
		"user_id", b.config.Matrix.UserID,
		"gateway", b.config.Gateway.URL,
	)

	// Store context for message processing goroutines
	b.ctx, b.cancel = context.WithCancel(ctx)
	defer b.cancel()

	// Register event handler for messages
	syncer, ok := b.matrix.Syncer.(*mautrix.DefaultSyncer)
	if !ok {
		return fmt.Errorf("unexpected syncer type: %T", b.matrix.Syncer)
	}
	syncer.OnEventType(event.EventMessage, b.handleMessageEvent)

	// Start syncing
	b.logger.Info("connecting to matrix homeserver")

	syncErr := make(chan error, 1)
	go func() {
		syncErr <- b.matrix.SyncWithContext(b.ctx)
	}()

	b.logger.Info("matrix bridge running")

	// Wait for context cancellation or sync error
	select {
	case <-ctx.Done():
		b.logger.Info("shutting down matrix bridge")
		b.cancel()
		return nil
	case err := <-syncErr:
		return fmt.Errorf("matrix sync failed: %w", err)
	}
}

// handleMessageEvent processes incoming Matrix messages.
func (b *Bridge) handleMessageEvent(ctx context.Context, evt *event.Event) {
	// Ignore our own messages
	if evt.Sender == id.UserID(b.config.Matrix.UserID) {
		return
	}

	// Get message content
	content, ok := evt.Content.Parsed.(*event.MessageEventContent)
	if !ok {
		return
	}

	// Only handle text messages
	if content.MsgType != event.MsgText {
		return
	}

	roomID := evt.RoomID.String()
	msgBody := content.Body

	// Check allowed rooms
	if !b.isRoomAllowed(roomID) {
		b.logger.Debug("ignoring message from non-allowed room", "room", roomID)
		return
	}

	// Check command prefix
	if b.config.Bridge.CommandPrefix != "" {
		if !strings.HasPrefix(msgBody, b.config.Bridge.CommandPrefix) {
			return
		}
		msgBody = strings.TrimPrefix(msgBody, b.config.Bridge.CommandPrefix)
		msgBody = strings.TrimSpace(msgBody)
	}

	if msgBody == "" {
		return
	}

	b.logger.Info("received message",
		"room", roomID,
		"sender", evt.Sender.String(),
		"content", truncate(msgBody, 50),
	)

	// Process message in goroutine to not block sync
	// Use bridge context for graceful shutdown support
	go b.processMessage(b.ctx, evt.RoomID, evt.Sender, msgBody)
}

// processMessage sends the message to gateway and streams response back.
func (b *Bridge) processMessage(ctx context.Context, roomID id.RoomID, sender id.UserID, content string) {
	roomStr := roomID.String()

	// Check if we're already processing a message in this room
	if _, loaded := b.processing.LoadOrStore(roomStr, true); loaded {
		b.logger.Debug("already processing message in room, dropping", "room", roomStr)
		return
	}
	defer b.processing.Delete(roomStr)

	// Send typing indicator
	if b.config.Bridge.TypingIndicator {
		b.setTyping(roomID, true)
		defer b.setTyping(roomID, false)
	}

	// Build gateway request
	req := SendRequest{
		Sender:    sender.String(),
		Content:   content,
		Frontend:  "matrix",
		ChannelID: roomStr,
	}

	// Accumulate response text
	var responseText strings.Builder

	// Send to gateway and handle streaming response
	fullResponse, err := b.gateway.SendMessage(ctx, req, func(evt SSEEvent) {
		switch evt.Type {
		case EventText:
			var data TextEventData
			if err := parseJSON(evt.Data, &data); err == nil {
				responseText.WriteString(data.Text)
			}
		case EventThinking:
			// Optionally show thinking indicator
			b.logger.Debug("agent thinking", "room", roomStr)
		case EventError:
			var data ErrorEventData
			if err := parseJSON(evt.Data, &data); err == nil {
				b.logger.Error("agent error", "room", roomStr, "error", data.Error)
			}
		}
	})

	if err != nil {
		b.logger.Error("gateway request failed", "room", roomStr, "error", err)
		b.sendMessage(roomID, fmt.Sprintf("Error: %v", err))
		return
	}

	// Use full response if available, otherwise accumulated text
	response := fullResponse
	if response == "" {
		response = responseText.String()
	}

	if response == "" {
		b.logger.Warn("empty response from agent", "room", roomStr)
		return
	}

	b.logger.Info("sending response",
		"room", roomStr,
		"length", len(response),
	)

	b.sendMessage(roomID, response)
}

// isRoomAllowed checks if the room is in the allowed list.
func (b *Bridge) isRoomAllowed(roomID string) bool {
	if len(b.config.Bridge.AllowedRooms) == 0 {
		return true // Allow all if no filter
	}

	for _, allowed := range b.config.Bridge.AllowedRooms {
		if allowed == roomID {
			return true
		}
	}
	return false
}

// typingTimeout is the duration the typing indicator shows (30 seconds).
const typingTimeout = 30 * time.Second

// networkTimeout is the timeout for Matrix API calls.
const networkTimeout = 10 * time.Second

// setTyping sends typing indicator to room.
func (b *Bridge) setTyping(roomID id.RoomID, typing bool) {
	var timeout time.Duration
	if typing {
		timeout = typingTimeout
	}
	// Use a timeout context to avoid hanging during shutdown or network issues
	ctx, cancel := context.WithTimeout(context.Background(), networkTimeout)
	defer cancel()
	_, err := b.matrix.UserTyping(ctx, roomID, typing, timeout)
	if err != nil {
		b.logger.Debug("failed to set typing indicator", "room", roomID.String(), "error", err)
	}
}

// sendMessage sends a text message to a room.
func (b *Bridge) sendMessage(roomID id.RoomID, text string) {
	// Use a longer timeout for sending messages (they can be large)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := b.matrix.SendText(ctx, roomID, text)
	if err != nil {
		b.logger.Error("failed to send message", "room", roomID.String(), "error", err)
	}
}

// truncate shortens a string to the given max rune count, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// parseJSON unmarshals JSON from a string into the given value.
func parseJSON(data string, v interface{}) error {
	return json.Unmarshal([]byte(data), v)
}
