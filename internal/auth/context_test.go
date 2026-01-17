// ABOUTME: Unit tests for authentication context functions
// ABOUTME: Tests AuthContext, IsAdmin, and context propagation helpers

package auth

import (
	"context"
	"testing"
)

func TestAuthContext_IsAdmin_True(t *testing.T) {
	tests := []struct {
		name  string
		roles []string
	}{
		{
			name:  "admin role",
			roles: []string{"admin"},
		},
		{
			name:  "owner role",
			roles: []string{"owner"},
		},
		{
			name:  "admin with other roles",
			roles: []string{"member", "admin", "viewer"},
		},
		{
			name:  "owner with other roles",
			roles: []string{"member", "owner"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := &AuthContext{
				PrincipalID:   "test-principal",
				PrincipalType: "client",
				Roles:         tt.roles,
			}

			if !auth.IsAdmin() {
				t.Errorf("IsAdmin() = false, want true for roles %v", tt.roles)
			}
		})
	}
}

func TestAuthContext_IsAdmin_False(t *testing.T) {
	tests := []struct {
		name  string
		roles []string
	}{
		{
			name:  "no roles",
			roles: []string{},
		},
		{
			name:  "nil roles",
			roles: nil,
		},
		{
			name:  "member role only",
			roles: []string{"member"},
		},
		{
			name:  "viewer role only",
			roles: []string{"viewer"},
		},
		{
			name:  "multiple non-admin roles",
			roles: []string{"member", "viewer", "reader"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := &AuthContext{
				PrincipalID:   "test-principal",
				PrincipalType: "client",
				Roles:         tt.roles,
			}

			if auth.IsAdmin() {
				t.Errorf("IsAdmin() = true, want false for roles %v", tt.roles)
			}
		})
	}
}

func TestFromContext_Present(t *testing.T) {
	expected := &AuthContext{
		PrincipalID:   "test-id",
		PrincipalType: "agent",
		Roles:         []string{"admin"},
	}

	ctx := WithAuth(context.Background(), expected)
	got := FromContext(ctx)

	if got == nil {
		t.Fatal("FromContext() = nil, want non-nil")
	}

	if got.PrincipalID != expected.PrincipalID {
		t.Errorf("PrincipalID = %q, want %q", got.PrincipalID, expected.PrincipalID)
	}

	if got.PrincipalType != expected.PrincipalType {
		t.Errorf("PrincipalType = %q, want %q", got.PrincipalType, expected.PrincipalType)
	}

	if len(got.Roles) != len(expected.Roles) {
		t.Errorf("Roles = %v, want %v", got.Roles, expected.Roles)
	}
}

func TestFromContext_Missing(t *testing.T) {
	ctx := context.Background()
	got := FromContext(ctx)

	if got != nil {
		t.Errorf("FromContext() = %v, want nil", got)
	}
}

func TestMustFromContext_Present(t *testing.T) {
	expected := &AuthContext{
		PrincipalID:   "test-id",
		PrincipalType: "agent",
		Roles:         []string{"admin"},
	}

	ctx := WithAuth(context.Background(), expected)

	// Should not panic
	got := MustFromContext(ctx)

	if got.PrincipalID != expected.PrincipalID {
		t.Errorf("PrincipalID = %q, want %q", got.PrincipalID, expected.PrincipalID)
	}
}

func TestMustFromContext_Missing(t *testing.T) {
	ctx := context.Background()

	defer func() {
		if r := recover(); r == nil {
			t.Error("MustFromContext() did not panic when auth context missing")
		}
	}()

	MustFromContext(ctx)
}
