// ABOUTME: Tests for pair token store operations backing QR device pairing.
// ABOUTME: Covers create/get, consume-once semantics, the consume race, and cleanup.

package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createPairTestUser satisfies the pair_tokens.created_by foreign key.
func createPairTestUser(t *testing.T, s *SQLiteStore) *AdminUser {
	t.Helper()
	u := &AdminUser{
		ID:          "user-" + uuid.New().String(),
		Username:    "pairadmin-" + uuid.New().String(),
		DisplayName: "Pair Admin",
		CreatedAt:   time.Now(),
	}
	require.NoError(t, s.CreateAdminUser(context.Background(), u))
	return u
}

func TestCreatePairToken_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	user := createPairTestUser(t, s)

	expires := time.Now().Add(5 * time.Minute)
	created, err := s.CreatePairToken(ctx, "roundtrip-hash", user.ID, expires)
	require.NoError(t, err)
	require.NotEmpty(t, created.ID)

	got, err := s.GetPairTokenByHash(ctx, "roundtrip-hash")
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
	assert.Equal(t, "roundtrip-hash", got.TokenHash)
	assert.Equal(t, user.ID, got.CreatedBy)
	assert.WithinDuration(t, expires, got.ExpiresAt, 2*time.Second)
	assert.Nil(t, got.UsedAt)
	assert.Nil(t, got.PrincipalID)
}

func TestGetPairTokenByHash_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetPairTokenByHash(context.Background(), "no-such-hash")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestCreatePairToken_DuplicateHashRejected(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	user := createPairTestUser(t, s)

	_, err := s.CreatePairToken(ctx, "dup-hash", user.ID, time.Now().Add(5*time.Minute))
	require.NoError(t, err)
	_, err = s.CreatePairToken(ctx, "dup-hash", user.ID, time.Now().Add(5*time.Minute))
	assert.Error(t, err, "token_hash is UNIQUE")
}

func TestConsumePairToken_SetsUsedAtOnce(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	user := createPairTestUser(t, s)

	pt, err := s.CreatePairToken(ctx, "consume-hash", user.ID, time.Now().Add(5*time.Minute))
	require.NoError(t, err)

	require.NoError(t, s.ConsumePairToken(ctx, pt.ID))

	got, err := s.GetPairTokenByHash(ctx, "consume-hash")
	require.NoError(t, err)
	require.NotNil(t, got.UsedAt)
	assert.WithinDuration(t, time.Now(), *got.UsedAt, 5*time.Second)

	err = s.ConsumePairToken(ctx, pt.ID)
	assert.ErrorIs(t, err, ErrPairTokenUsed)
}

func TestConsumePairToken_ConcurrentRace(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	user := createPairTestUser(t, s)

	pt, err := s.CreatePairToken(ctx, "race-hash", user.ID, time.Now().Add(5*time.Minute))
	require.NoError(t, err)

	start := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			results <- s.ConsumePairToken(ctx, pt.ID)
		}()
	}
	close(start)

	var wins, losses int
	for i := 0; i < 2; i++ {
		switch err := <-results; {
		case err == nil:
			wins++
		case errors.Is(err, ErrPairTokenUsed):
			losses++
		default:
			t.Fatalf("unexpected error from concurrent consume: %v", err)
		}
	}
	assert.Equal(t, 1, wins, "exactly one consumer must win")
	assert.Equal(t, 1, losses, "the loser must see ErrPairTokenUsed")
}

func TestSetPairTokenPrincipal(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	user := createPairTestUser(t, s)

	// principal_id has a foreign key to principals.
	principal := &Principal{
		ID:          uuid.New().String(),
		Type:        PrincipalTypeAgent,
		PubkeyFP:    strings.Repeat("a", 64),
		DisplayName: "Paired Device",
		Status:      PrincipalStatusApproved,
		CreatedAt:   time.Now(),
	}
	require.NoError(t, s.CreatePrincipal(ctx, principal))

	pt, err := s.CreatePairToken(ctx, "principal-hash", user.ID, time.Now().Add(5*time.Minute))
	require.NoError(t, err)

	require.NoError(t, s.SetPairTokenPrincipal(ctx, pt.ID, principal.ID))

	got, err := s.GetPairTokenByHash(ctx, "principal-hash")
	require.NoError(t, err)
	require.NotNil(t, got.PrincipalID)
	assert.Equal(t, principal.ID, *got.PrincipalID)
}

func TestSetPairTokenPrincipal_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	principal := &Principal{
		ID:          uuid.New().String(),
		Type:        PrincipalTypeAgent,
		PubkeyFP:    strings.Repeat("b", 64),
		DisplayName: "Paired Device",
		Status:      PrincipalStatusApproved,
		CreatedAt:   time.Now(),
	}
	require.NoError(t, s.CreatePrincipal(ctx, principal))

	err := s.SetPairTokenPrincipal(ctx, "no-such-id", principal.ID)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestDeleteExpiredPairTokens(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	user := createPairTestUser(t, s)

	// Expired + unused: should be deleted.
	_, err := s.CreatePairToken(ctx, "expired-hash", user.ID, time.Now().Add(-time.Minute))
	require.NoError(t, err)
	// Live: should survive.
	_, err = s.CreatePairToken(ctx, "live-hash", user.ID, time.Now().Add(5*time.Minute))
	require.NoError(t, err)
	// Expired + used: should survive (enrollment linkage is kept).
	usedPT, err := s.CreatePairToken(ctx, "used-hash", user.ID, time.Now().Add(-time.Minute))
	require.NoError(t, err)
	require.NoError(t, s.ConsumePairToken(ctx, usedPT.ID))

	require.NoError(t, s.DeleteExpiredPairTokens(ctx))

	_, err = s.GetPairTokenByHash(ctx, "expired-hash")
	assert.ErrorIs(t, err, ErrNotFound, "expired unused token must be deleted")
	_, err = s.GetPairTokenByHash(ctx, "live-hash")
	assert.NoError(t, err, "live token must survive")
	_, err = s.GetPairTokenByHash(ctx, "used-hash")
	assert.NoError(t, err, "used token must survive for its enrollment record")
}
