// ABOUTME: Tests for bridge message deduplication.
// ABOUTME: Validates that duplicate platform messages are silently ignored.

package gateway

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/2389/fold-gateway/internal/dedupe"
)

// testGatewayWithDedupe creates a minimal Gateway with a dedupe cache for testing.
func testGatewayWithDedupe(t *testing.T, ttl time.Duration) *Gateway {
	t.Helper()
	cfg := testConfig(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	gw, err := New(cfg, logger)
	require.NoError(t, err)

	// Replace the dedupe cache with a test-configured one
	if gw.dedupe != nil {
		gw.dedupe.Close()
	}
	gw.dedupe = dedupe.New(ttl, 1000)

	t.Cleanup(func() {
		gw.Shutdown(context.Background())
	})

	return gw
}

func TestBridgeDedupe_FirstMessage(t *testing.T) {
	gw := testGatewayWithDedupe(t, 5*time.Minute)

	msg := &BridgeMessage{
		Frontend:          "matrix",
		PlatformMessageID: "$abc123",
		ChannelID:         "!room:matrix.org",
		Sender:            "@user:matrix.org",
		Content:           "Hello, world!",
	}

	// First message should be processed
	err := gw.HandleBridgeMessage(context.Background(), msg)
	assert.NoError(t, err)

	// Verify the message was marked as seen by checking dedupe cache directly
	key := "bridge:matrix:$abc123"
	assert.True(t, gw.dedupe.Check(key), "message should be marked as seen after processing")
}

func TestBridgeDedupe_DuplicateMessage(t *testing.T) {
	gw := testGatewayWithDedupe(t, 5*time.Minute)

	msg := &BridgeMessage{
		Frontend:          "slack",
		PlatformMessageID: "1234567890.123456",
		ChannelID:         "C01234567",
		Sender:            "U01234567",
		Content:           "Hello from Slack!",
	}

	// First message should be processed
	err := gw.HandleBridgeMessage(context.Background(), msg)
	assert.NoError(t, err)

	// Second (duplicate) message should also succeed (idempotent) but not be reprocessed
	// The check-mark pattern means duplicates return success without side effects
	err = gw.HandleBridgeMessage(context.Background(), msg)
	assert.NoError(t, err)

	// Verify key is still marked (was not cleared or changed)
	key := "bridge:slack:1234567890.123456"
	assert.True(t, gw.dedupe.Check(key))
}

func TestBridgeDedupe_AfterTTL(t *testing.T) {
	// Use a very short TTL for testing
	gw := testGatewayWithDedupe(t, 20*time.Millisecond)

	msg := &BridgeMessage{
		Frontend:          "telegram",
		PlatformMessageID: "12345",
		ChannelID:         "-1001234567890",
		Sender:            "123456789",
		Content:           "Hello from Telegram!",
	}

	// First message should be processed
	err := gw.HandleBridgeMessage(context.Background(), msg)
	assert.NoError(t, err)

	// Verify it was marked
	key := "bridge:telegram:12345"
	assert.True(t, gw.dedupe.Check(key))

	// Wait for TTL to expire
	time.Sleep(30 * time.Millisecond)

	// After TTL, the key should be expired
	assert.False(t, gw.dedupe.Check(key), "key should be expired after TTL")

	// Message should be processable again (no longer seen as duplicate)
	err = gw.HandleBridgeMessage(context.Background(), msg)
	assert.NoError(t, err)
}

func TestBridgeDedupe_DifferentChannels(t *testing.T) {
	gw := testGatewayWithDedupe(t, 5*time.Minute)

	// Same platform message ID but different frontends should be treated as different
	matrixMsg := &BridgeMessage{
		Frontend:          "matrix",
		PlatformMessageID: "same-id-123",
		ChannelID:         "!room:matrix.org",
		Sender:            "@user:matrix.org",
		Content:           "Matrix message",
	}

	slackMsg := &BridgeMessage{
		Frontend:          "slack",
		PlatformMessageID: "same-id-123",
		ChannelID:         "C01234567",
		Sender:            "U01234567",
		Content:           "Slack message",
	}

	// Both should be processed as they have different frontends
	err := gw.HandleBridgeMessage(context.Background(), matrixMsg)
	assert.NoError(t, err)

	err = gw.HandleBridgeMessage(context.Background(), slackMsg)
	assert.NoError(t, err)

	// Both keys should be marked separately
	assert.True(t, gw.dedupe.Check("bridge:matrix:same-id-123"))
	assert.True(t, gw.dedupe.Check("bridge:slack:same-id-123"))
}

func TestBridgeDedupe_DifferentMessages_SameFrontend(t *testing.T) {
	gw := testGatewayWithDedupe(t, 5*time.Minute)

	msg1 := &BridgeMessage{
		Frontend:          "matrix",
		PlatformMessageID: "$event1",
		ChannelID:         "!room:matrix.org",
		Sender:            "@user:matrix.org",
		Content:           "First message",
	}

	msg2 := &BridgeMessage{
		Frontend:          "matrix",
		PlatformMessageID: "$event2",
		ChannelID:         "!room:matrix.org",
		Sender:            "@user:matrix.org",
		Content:           "Second message",
	}

	// Both distinct messages should be processed
	err := gw.HandleBridgeMessage(context.Background(), msg1)
	assert.NoError(t, err)

	err = gw.HandleBridgeMessage(context.Background(), msg2)
	assert.NoError(t, err)

	// Both should be marked
	assert.True(t, gw.dedupe.Check("bridge:matrix:$event1"))
	assert.True(t, gw.dedupe.Check("bridge:matrix:$event2"))
}
