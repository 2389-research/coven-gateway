// ABOUTME: Tests for ClientService SendMessage RPC with idempotency key deduplication.
// ABOUTME: Validates idempotency key validation, duplicate detection, and successful message handling.

package client

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/2389/fold-gateway/internal/dedupe"
	pb "github.com/2389/fold-gateway/proto/fold"
)

func TestSendMessage_MissingIdempotencyKey(t *testing.T) {
	svc := newTestClientService(t)

	req := &pb.ClientSendMessageRequest{
		ConversationKey: "test-conversation",
		Content:         "Hello, world!",
		IdempotencyKey:  "", // missing
	}

	_, err := svc.SendMessage(context.Background(), req)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Equal(t, "idempotency_key required", st.Message())
}

func TestSendMessage_IdempotencyKeyTooLong(t *testing.T) {
	svc := newTestClientService(t)

	req := &pb.ClientSendMessageRequest{
		ConversationKey: "test-conversation",
		Content:         "Hello, world!",
		IdempotencyKey:  strings.Repeat("a", 101), // 101 chars, max is 100
	}

	_, err := svc.SendMessage(context.Background(), req)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Equal(t, "idempotency_key too long", st.Message())
}

func TestSendMessage_FirstRequest(t *testing.T) {
	svc := newTestClientService(t)

	req := &pb.ClientSendMessageRequest{
		ConversationKey: "test-conversation",
		Content:         "Hello, world!",
		IdempotencyKey:  "unique-key-123",
	}

	resp, err := svc.SendMessage(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, "accepted", resp.Status)
	assert.NotEmpty(t, resp.MessageId)
}

func TestSendMessage_DuplicateRequest(t *testing.T) {
	svc := newTestClientService(t)

	req := &pb.ClientSendMessageRequest{
		ConversationKey: "test-conversation",
		Content:         "Hello, world!",
		IdempotencyKey:  "duplicate-key-456",
	}

	// First request should succeed
	resp1, err := svc.SendMessage(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "accepted", resp1.Status)

	// Second request with same idempotency key should return "duplicate"
	resp2, err := svc.SendMessage(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "duplicate", resp2.Status)
	assert.Empty(t, resp2.MessageId, "duplicate should not return a message ID")
}

func TestSendMessage_DifferentKeys(t *testing.T) {
	svc := newTestClientService(t)

	req1 := &pb.ClientSendMessageRequest{
		ConversationKey: "test-conversation",
		Content:         "First message",
		IdempotencyKey:  "key-1",
	}

	req2 := &pb.ClientSendMessageRequest{
		ConversationKey: "test-conversation",
		Content:         "Second message",
		IdempotencyKey:  "key-2",
	}

	// Both requests with different keys should succeed
	resp1, err := svc.SendMessage(context.Background(), req1)
	require.NoError(t, err)
	assert.Equal(t, "accepted", resp1.Status)

	resp2, err := svc.SendMessage(context.Background(), req2)
	require.NoError(t, err)
	assert.Equal(t, "accepted", resp2.Status)

	// IDs should be different
	assert.NotEqual(t, resp1.MessageId, resp2.MessageId)
}

func TestSendMessage_MaxLengthIdempotencyKey(t *testing.T) {
	svc := newTestClientService(t)

	req := &pb.ClientSendMessageRequest{
		ConversationKey: "test-conversation",
		Content:         "Hello, world!",
		IdempotencyKey:  strings.Repeat("a", 100), // exactly 100 chars (the max)
	}

	resp, err := svc.SendMessage(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, "accepted", resp.Status)
}

func TestSendMessage_MissingConversationKey(t *testing.T) {
	svc := newTestClientService(t)

	req := &pb.ClientSendMessageRequest{
		ConversationKey: "", // missing
		Content:         "Hello, world!",
		IdempotencyKey:  "unique-key-789",
	}

	_, err := svc.SendMessage(context.Background(), req)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Equal(t, "conversation_key required", st.Message())
}

func TestSendMessage_MissingContent(t *testing.T) {
	svc := newTestClientService(t)

	req := &pb.ClientSendMessageRequest{
		ConversationKey: "test-conversation",
		Content:         "", // missing
		IdempotencyKey:  "unique-key-abc",
	}

	_, err := svc.SendMessage(context.Background(), req)

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Equal(t, "content required", st.Message())
}

// newTestClientService creates a ClientService with a real dedupe cache for testing.
func newTestClientService(t *testing.T) *ClientService {
	t.Helper()

	// Create a dedupe cache with a 5-minute TTL (same as production)
	dedupeCache := dedupe.New(5*time.Minute, 1000)
	t.Cleanup(func() {
		dedupeCache.Close()
	})

	return NewClientServiceWithDedupe(nil, nil, dedupeCache)
}
