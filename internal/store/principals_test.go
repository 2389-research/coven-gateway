// ABOUTME: Tests for principals store operations
// ABOUTME: Covers CRUD, filtering, and validation for the principals table

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrincipalStore_Create(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	p := &Principal{
		ID:          "principal-123",
		Type:        PrincipalTypeClient,
		PubkeyFP:    "abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234",
		DisplayName: "Test Client",
		Status:      PrincipalStatusApproved,
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
		Metadata:    map[string]any{"version": "1.0"},
	}

	err := store.CreatePrincipal(ctx, p)
	require.NoError(t, err)

	// Verify we can retrieve it
	retrieved, err := store.GetPrincipal(ctx, "principal-123")
	require.NoError(t, err)
	assert.Equal(t, "principal-123", retrieved.ID)
	assert.Equal(t, PrincipalTypeClient, retrieved.Type)
	assert.Equal(t, "Test Client", retrieved.DisplayName)
	assert.Equal(t, PrincipalStatusApproved, retrieved.Status)
	assert.Equal(t, "1.0", retrieved.Metadata["version"])
}

func TestPrincipalStore_Create_DuplicatePubkey(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	fp := "abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234"

	p1 := &Principal{
		ID:          "principal-1",
		Type:        PrincipalTypeClient,
		PubkeyFP:    fp,
		DisplayName: "First",
		Status:      PrincipalStatusApproved,
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
	}
	require.NoError(t, store.CreatePrincipal(ctx, p1))

	p2 := &Principal{
		ID:          "principal-2",
		Type:        PrincipalTypeClient,
		PubkeyFP:    fp, // same fingerprint
		DisplayName: "Second",
		Status:      PrincipalStatusApproved,
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
	}

	err := store.CreatePrincipal(ctx, p2)
	assert.ErrorIs(t, err, ErrDuplicatePubkey)
}

func TestPrincipalStore_Get(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	p := &Principal{
		ID:          "principal-123",
		Type:        PrincipalTypeAgent,
		PubkeyFP:    "abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234",
		DisplayName: "Test Agent",
		Status:      PrincipalStatusOnline,
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
	}
	require.NoError(t, store.CreatePrincipal(ctx, p))

	retrieved, err := store.GetPrincipal(ctx, "principal-123")
	require.NoError(t, err)
	assert.Equal(t, PrincipalTypeAgent, retrieved.Type)
	assert.Equal(t, PrincipalStatusOnline, retrieved.Status)
}

func TestPrincipalStore_Get_NotFound(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	_, err := store.GetPrincipal(ctx, "nonexistent")
	assert.ErrorIs(t, err, ErrPrincipalNotFound)
}

func TestPrincipalStore_GetByPubkey(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	fp := "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	p := &Principal{
		ID:          "principal-xyz",
		Type:        PrincipalTypePack,
		PubkeyFP:    fp,
		DisplayName: "Test Pack",
		Status:      PrincipalStatusApproved,
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
	}
	require.NoError(t, store.CreatePrincipal(ctx, p))

	retrieved, err := store.GetPrincipalByPubkey(ctx, fp)
	require.NoError(t, err)
	assert.Equal(t, "principal-xyz", retrieved.ID)
	assert.Equal(t, PrincipalTypePack, retrieved.Type)
}

func TestPrincipalStore_GetByPubkey_NotFound(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	_, err := store.GetPrincipalByPubkey(ctx, "nonexistent_fingerprint")
	assert.ErrorIs(t, err, ErrPrincipalNotFound)
}

func TestPrincipalStore_UpdateStatus(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	p := &Principal{
		ID:          "principal-123",
		Type:        PrincipalTypeClient,
		PubkeyFP:    "abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234",
		DisplayName: "Test Client",
		Status:      PrincipalStatusPending,
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
	}
	require.NoError(t, store.CreatePrincipal(ctx, p))

	err := store.UpdatePrincipalStatus(ctx, "principal-123", PrincipalStatusApproved)
	require.NoError(t, err)

	retrieved, err := store.GetPrincipal(ctx, "principal-123")
	require.NoError(t, err)
	assert.Equal(t, PrincipalStatusApproved, retrieved.Status)
}

func TestPrincipalStore_UpdateStatus_Invalid(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	p := &Principal{
		ID:          "principal-123",
		Type:        PrincipalTypeClient,
		PubkeyFP:    "abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234",
		DisplayName: "Test Client",
		Status:      PrincipalStatusPending,
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
	}
	require.NoError(t, store.CreatePrincipal(ctx, p))

	err := store.UpdatePrincipalStatus(ctx, "principal-123", "invalid_status")
	assert.ErrorIs(t, err, ErrInvalidStatus)
}

func TestPrincipalStore_UpdateStatus_NotFound(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	err := store.UpdatePrincipalStatus(ctx, "nonexistent", PrincipalStatusApproved)
	assert.ErrorIs(t, err, ErrPrincipalNotFound)
}

func TestPrincipalStore_UpdateLastSeen(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	p := &Principal{
		ID:          "principal-123",
		Type:        PrincipalTypeAgent,
		PubkeyFP:    "abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234",
		DisplayName: "Test Agent",
		Status:      PrincipalStatusOnline,
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
	}
	require.NoError(t, store.CreatePrincipal(ctx, p))

	lastSeen := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	err := store.UpdatePrincipalLastSeen(ctx, "principal-123", lastSeen)
	require.NoError(t, err)

	retrieved, err := store.GetPrincipal(ctx, "principal-123")
	require.NoError(t, err)
	require.NotNil(t, retrieved.LastSeen)
	assert.Equal(t, lastSeen, *retrieved.LastSeen)
}

func TestPrincipalStore_List_NoFilter(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Create multiple principals
	for i, typ := range []PrincipalType{PrincipalTypeClient, PrincipalTypeAgent, PrincipalTypePack} {
		p := &Principal{
			ID:          generateTestID("principal", i),
			Type:        typ,
			PubkeyFP:    generateTestFingerprint(i),
			DisplayName: string(typ),
			Status:      PrincipalStatusApproved,
			CreatedAt:   time.Now().UTC().Truncate(time.Second),
		}
		require.NoError(t, store.CreatePrincipal(ctx, p))
	}

	principals, err := store.ListPrincipals(ctx, PrincipalFilter{})
	require.NoError(t, err)
	assert.Len(t, principals, 3)
}

func TestPrincipalStore_List_ByType(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Create one of each type
	for i, typ := range []PrincipalType{PrincipalTypeClient, PrincipalTypeAgent, PrincipalTypePack} {
		p := &Principal{
			ID:          generateTestID("principal", i),
			Type:        typ,
			PubkeyFP:    generateTestFingerprint(i),
			DisplayName: string(typ),
			Status:      PrincipalStatusApproved,
			CreatedAt:   time.Now().UTC().Truncate(time.Second),
		}
		require.NoError(t, store.CreatePrincipal(ctx, p))
	}

	agentType := PrincipalTypeAgent
	principals, err := store.ListPrincipals(ctx, PrincipalFilter{Type: &agentType})
	require.NoError(t, err)
	assert.Len(t, principals, 1)
	assert.Equal(t, PrincipalTypeAgent, principals[0].Type)
}

func TestPrincipalStore_List_ByStatus(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	statuses := []PrincipalStatus{PrincipalStatusPending, PrincipalStatusApproved, PrincipalStatusRevoked}
	for i, status := range statuses {
		p := &Principal{
			ID:          generateTestID("principal", i),
			Type:        PrincipalTypeClient,
			PubkeyFP:    generateTestFingerprint(i),
			DisplayName: string(status),
			Status:      status,
			CreatedAt:   time.Now().UTC().Truncate(time.Second),
		}
		require.NoError(t, store.CreatePrincipal(ctx, p))
	}

	pendingStatus := PrincipalStatusPending
	principals, err := store.ListPrincipals(ctx, PrincipalFilter{Status: &pendingStatus})
	require.NoError(t, err)
	assert.Len(t, principals, 1)
	assert.Equal(t, PrincipalStatusPending, principals[0].Status)
}

func TestPrincipalStore_List_Pagination(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Create 5 principals
	for i := range 5 {
		p := &Principal{
			ID:          generateTestID("principal", i),
			Type:        PrincipalTypeClient,
			PubkeyFP:    generateTestFingerprint(i),
			DisplayName: generateTestID("name", i),
			Status:      PrincipalStatusApproved,
			CreatedAt:   time.Now().UTC().Truncate(time.Second),
		}
		require.NoError(t, store.CreatePrincipal(ctx, p))
	}

	// Get first page
	page1, err := store.ListPrincipals(ctx, PrincipalFilter{Limit: 2, Offset: 0})
	require.NoError(t, err)
	assert.Len(t, page1, 2)

	// Get second page
	page2, err := store.ListPrincipals(ctx, PrincipalFilter{Limit: 2, Offset: 2})
	require.NoError(t, err)
	assert.Len(t, page2, 2)

	// Pages should have different results
	assert.NotEqual(t, page1[0].ID, page2[0].ID)
}

func TestPrincipalStore_Count(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Create principals with different statuses
	for i, status := range []PrincipalStatus{PrincipalStatusPending, PrincipalStatusApproved, PrincipalStatusApproved} {
		p := &Principal{
			ID:          generateTestID("principal", i),
			Type:        PrincipalTypeClient,
			PubkeyFP:    generateTestFingerprint(i),
			DisplayName: generateTestID("name", i),
			Status:      status,
			CreatedAt:   time.Now().UTC().Truncate(time.Second),
		}
		require.NoError(t, store.CreatePrincipal(ctx, p))
	}

	// Count all
	total, err := store.CountPrincipals(ctx, PrincipalFilter{})
	require.NoError(t, err)
	assert.Equal(t, 3, total)

	// Count approved
	approvedStatus := PrincipalStatusApproved
	approved, err := store.CountPrincipals(ctx, PrincipalFilter{Status: &approvedStatus})
	require.NoError(t, err)
	assert.Equal(t, 2, approved)
}

func TestPrincipalStore_Create_MetadataTooLarge(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Create metadata that exceeds 64KB when JSON encoded
	largeData := make([]byte, MaxMetadataSize+1)
	for i := range largeData {
		largeData[i] = 'a'
	}

	p := &Principal{
		ID:          "principal-large-metadata",
		Type:        PrincipalTypeClient,
		PubkeyFP:    "large1234large1234large1234large1234large1234large1234large1234",
		DisplayName: "Large Metadata Client",
		Status:      PrincipalStatusApproved,
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
		Metadata:    map[string]any{"data": string(largeData)},
	}

	err := store.CreatePrincipal(ctx, p)
	assert.ErrorIs(t, err, ErrMetadataTooLarge)
}

func TestPrincipalStore_Create_MetadataAtLimit(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Create metadata just under the limit
	// Account for JSON overhead: {"data":"..."} = 10 chars overhead
	smallerData := make([]byte, MaxMetadataSize-100)
	for i := range smallerData {
		smallerData[i] = 'a'
	}

	p := &Principal{
		ID:          "principal-limit-metadata",
		Type:        PrincipalTypeClient,
		PubkeyFP:    "limit123limit123limit123limit123limit123limit123limit123limit12",
		DisplayName: "Limit Metadata Client",
		Status:      PrincipalStatusApproved,
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
		Metadata:    map[string]any{"data": string(smallerData)},
	}

	err := store.CreatePrincipal(ctx, p)
	require.NoError(t, err)

	// Verify we can retrieve it
	retrieved, err := store.GetPrincipal(ctx, "principal-limit-metadata")
	require.NoError(t, err)
	assert.NotNil(t, retrieved.Metadata)
}

func TestDeletePrincipal_Success(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Create a principal
	p := &Principal{
		ID:          "principal-to-delete",
		Type:        PrincipalTypeClient,
		DisplayName: "To Be Deleted",
		Status:      PrincipalStatusApproved,
		CreatedAt:   time.Now().UTC(),
	}
	require.NoError(t, store.CreatePrincipal(ctx, p))

	// Delete it
	err := store.DeletePrincipal(ctx, "principal-to-delete")
	require.NoError(t, err)

	// Verify it's gone
	_, err = store.GetPrincipal(ctx, "principal-to-delete")
	assert.ErrorIs(t, err, ErrPrincipalNotFound)
}

func TestDeletePrincipal_NotFound(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	err := store.DeletePrincipal(ctx, "nonexistent")
	assert.ErrorIs(t, err, ErrPrincipalNotFound)
}

// Helper functions for tests

func generateTestID(prefix string, i int) string {
	return prefix + "-" + string(rune('a'+i))
}

func generateTestFingerprint(i int) string {
	// Generate a 64-char hex fingerprint using valid hex digits
	hexDigits := "0123456789abcdef"
	base := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcde"
	return base + string(hexDigits[i%16])
}
