// ABOUTME: Authentication context for tracking identity through request handlers
// ABOUTME: Provides WithAuth/FromContext for propagating auth info via context

package auth

import (
	"context"
)

// AuthContext holds the authenticated identity information extracted from a request.
// This is populated by the auth interceptor and can be retrieved from context in handlers.
type AuthContext struct {
	PrincipalID   string   // UUID of the authenticated principal
	PrincipalType string   // "client" | "agent" | "pack"
	MemberID      *string  // always nil in v1 (reserved for future member-level auth)
	Roles         []string // roles assigned to this principal
}

// IsAdmin returns true if the principal has admin or owner role.
func (a *AuthContext) IsAdmin() bool {
	for _, r := range a.Roles {
		if r == "admin" || r == "owner" {
			return true
		}
	}
	return false
}

// authContextKey is the key type for storing AuthContext in context.Context.
type authContextKey struct{}

// WithAuth returns a new context with the AuthContext attached.
func WithAuth(ctx context.Context, auth *AuthContext) context.Context {
	return context.WithValue(ctx, authContextKey{}, auth)
}

// FromContext retrieves the AuthContext from the context, returning nil if not present.
func FromContext(ctx context.Context) *AuthContext {
	val := ctx.Value(authContextKey{})
	if val == nil {
		return nil
	}
	auth, ok := val.(*AuthContext)
	if !ok {
		return nil
	}
	return auth
}

// MustFromContext retrieves the AuthContext from the context, panicking if not present.
func MustFromContext(ctx context.Context) *AuthContext {
	auth := FromContext(ctx)
	if auth == nil {
		panic("auth: AuthContext not found in context")
	}
	return auth
}
