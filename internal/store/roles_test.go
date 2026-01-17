// ABOUTME: Tests for roles store operations
// ABOUTME: Covers Add, Remove, Has, and List for role assignments

package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoleStore_Add(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	err := store.AddRole(ctx, RoleSubjectPrincipal, "principal-123", RoleAdmin)
	require.NoError(t, err)

	// Verify it was added
	has, err := store.HasRole(ctx, RoleSubjectPrincipal, "principal-123", RoleAdmin)
	require.NoError(t, err)
	assert.True(t, has)
}

func TestRoleStore_Add_Idempotent(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Add same role twice - should succeed without error
	err := store.AddRole(ctx, RoleSubjectPrincipal, "principal-123", RoleAdmin)
	require.NoError(t, err)

	err = store.AddRole(ctx, RoleSubjectPrincipal, "principal-123", RoleAdmin)
	require.NoError(t, err, "adding existing role should be idempotent")

	// Should still only have one role
	roles, err := store.ListRoles(ctx, RoleSubjectPrincipal, "principal-123")
	require.NoError(t, err)
	assert.Len(t, roles, 1)
}

func TestRoleStore_Remove(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Add then remove
	err := store.AddRole(ctx, RoleSubjectPrincipal, "principal-123", RoleAdmin)
	require.NoError(t, err)

	err = store.RemoveRole(ctx, RoleSubjectPrincipal, "principal-123", RoleAdmin)
	require.NoError(t, err)

	// Verify it was removed
	has, err := store.HasRole(ctx, RoleSubjectPrincipal, "principal-123", RoleAdmin)
	require.NoError(t, err)
	assert.False(t, has)
}

func TestRoleStore_Remove_Idempotent(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Remove non-existent role - should succeed
	err := store.RemoveRole(ctx, RoleSubjectPrincipal, "principal-123", RoleAdmin)
	require.NoError(t, err, "removing non-existent role should be idempotent")
}

func TestRoleStore_Has_True(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	err := store.AddRole(ctx, RoleSubjectPrincipal, "principal-123", RoleOwner)
	require.NoError(t, err)

	has, err := store.HasRole(ctx, RoleSubjectPrincipal, "principal-123", RoleOwner)
	require.NoError(t, err)
	assert.True(t, has)
}

func TestRoleStore_Has_False(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Check role that was never added
	has, err := store.HasRole(ctx, RoleSubjectPrincipal, "principal-123", RoleAdmin)
	require.NoError(t, err)
	assert.False(t, has, "should return false for non-existent role")
}

func TestRoleStore_Has_DifferentSubject(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Add role to one principal
	err := store.AddRole(ctx, RoleSubjectPrincipal, "principal-1", RoleAdmin)
	require.NoError(t, err)

	// Different principal should not have it
	has, err := store.HasRole(ctx, RoleSubjectPrincipal, "principal-2", RoleAdmin)
	require.NoError(t, err)
	assert.False(t, has)
}

func TestRoleStore_List(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Add multiple roles
	require.NoError(t, store.AddRole(ctx, RoleSubjectPrincipal, "principal-123", RoleOwner))
	require.NoError(t, store.AddRole(ctx, RoleSubjectPrincipal, "principal-123", RoleAdmin))
	require.NoError(t, store.AddRole(ctx, RoleSubjectPrincipal, "principal-123", RoleMember))

	roles, err := store.ListRoles(ctx, RoleSubjectPrincipal, "principal-123")
	require.NoError(t, err)
	assert.Len(t, roles, 3)

	// Verify all roles are present
	roleMap := make(map[RoleName]bool)
	for _, r := range roles {
		roleMap[r] = true
	}
	assert.True(t, roleMap[RoleOwner])
	assert.True(t, roleMap[RoleAdmin])
	assert.True(t, roleMap[RoleMember])
}

func TestRoleStore_List_Empty(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	roles, err := store.ListRoles(ctx, RoleSubjectPrincipal, "principal-123")
	require.NoError(t, err)
	assert.Len(t, roles, 0, "should return empty slice for subject with no roles")
}

func TestRoleStore_List_DifferentSubjectTypes(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Add role to principal
	require.NoError(t, store.AddRole(ctx, RoleSubjectPrincipal, "id-123", RoleAdmin))

	// Add role to member with same ID
	require.NoError(t, store.AddRole(ctx, RoleSubjectMember, "id-123", RoleOwner))

	// List should return roles for the specific subject type
	principalRoles, err := store.ListRoles(ctx, RoleSubjectPrincipal, "id-123")
	require.NoError(t, err)
	assert.Len(t, principalRoles, 1)
	assert.Equal(t, RoleAdmin, principalRoles[0])

	memberRoles, err := store.ListRoles(ctx, RoleSubjectMember, "id-123")
	require.NoError(t, err)
	assert.Len(t, memberRoles, 1)
	assert.Equal(t, RoleOwner, memberRoles[0])
}
