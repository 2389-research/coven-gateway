// ABOUTME: Bridge message handling with deduplication.
// ABOUTME: Ensures duplicate messages from frontends (Matrix, Slack, Telegram) are silently ignored.

package gateway

import (
	"context"
	"fmt"
	"log/slog"
)

// BridgeMessage represents a message received from a frontend bridge.
// Each frontend provides a unique platform-specific message ID:
//   - Matrix: event_id (e.g., "$abc123")
//   - Slack: ts (e.g., "1234567890.123456")
//   - Telegram: message_id (integer, stored as string)
type BridgeMessage struct {
	// Frontend identifies the source platform (e.g., "matrix", "slack", "telegram")
	Frontend string

	// PlatformMessageID is the unique message identifier from the frontend platform
	PlatformMessageID string

	// ChannelID is the channel/room identifier on the frontend platform
	ChannelID string

	// Sender is the user identifier on the frontend platform
	Sender string

	// Content is the message text
	Content string
}

// HandleBridgeMessage processes a message from a frontend bridge.
// It uses deduplication to ensure the same platform message is only processed once.
// Duplicate messages return nil (success, idempotent behavior).
func (g *Gateway) HandleBridgeMessage(ctx context.Context, msg *BridgeMessage) error {
	// Build dedupe key from platform ID
	key := fmt.Sprintf("bridge:%s:%s", msg.Frontend, msg.PlatformMessageID)

	if g.dedupe.Check(key) {
		slog.Debug("duplicate bridge message ignored",
			"frontend", msg.Frontend,
			"platform_id", msg.PlatformMessageID,
		)
		return nil // success, idempotent
	}

	// Process message
	if err := g.processMessage(ctx, msg); err != nil {
		return err
	}

	// Mark as seen only after successful processing
	g.dedupe.Mark(key)
	return nil
}

// processMessage handles the actual message processing logic.
// This is separated from HandleBridgeMessage to allow the dedupe pattern:
// check -> process -> mark (only mark after successful processing).
func (g *Gateway) processMessage(ctx context.Context, msg *BridgeMessage) error {
	g.logger.Debug("processing bridge message",
		"frontend", msg.Frontend,
		"platform_id", msg.PlatformMessageID,
		"channel_id", msg.ChannelID,
		"sender", msg.Sender,
	)

	// The actual message routing to agents would go here.
	// For now, this is a stub that will be filled in by the message routing implementation.
	// The core dedupe logic is what this task implements.

	return nil
}
