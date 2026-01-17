// ABOUTME: HTTP middleware for JWT authentication on API endpoints
// ABOUTME: Extracts JWT from Authorization header and adds principal to context

package auth

import (
	"net/http"
	"strings"

	"github.com/2389/fold-gateway/internal/store"
)

// HTTPAuthMiddleware creates an HTTP middleware that extracts and validates JWT tokens.
// It looks up the principal and adds AuthContext to the request context using the same
// WithAuth/FromContext pattern as the gRPC interceptors for consistency.
func HTTPAuthMiddleware(principals PrincipalStore, roles RoleStore, verifier TokenVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract token from Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
				return
			}

			// Check Bearer prefix
			if !strings.HasPrefix(authHeader, "Bearer ") {
				http.Error(w, `{"error":"invalid authorization header format"}`, http.StatusUnauthorized)
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token == "" {
				http.Error(w, `{"error":"empty token"}`, http.StatusUnauthorized)
				return
			}

			// Verify token and extract principal ID
			principalID, err := verifier.Verify(token)
			if err != nil {
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}

			// Look up principal
			principal, err := principals.GetPrincipal(r.Context(), principalID)
			if err != nil {
				http.Error(w, `{"error":"principal not found"}`, http.StatusUnauthorized)
				return
			}

			// Check principal status - allow approved/online/offline, deny pending/revoked
			switch principal.Status {
			case store.PrincipalStatusApproved, store.PrincipalStatusOnline, store.PrincipalStatusOffline:
				// allowed
			case store.PrincipalStatusPending:
				http.Error(w, `{"error":"principal status is pending"}`, http.StatusForbidden)
				return
			case store.PrincipalStatusRevoked:
				http.Error(w, `{"error":"principal has been revoked"}`, http.StatusForbidden)
				return
			default:
				http.Error(w, `{"error":"unknown principal status"}`, http.StatusInternalServerError)
				return
			}

			// Look up roles
			roleNames, err := roles.ListRoles(r.Context(), store.RoleSubjectPrincipal, principalID)
			if err != nil {
				// Log but don't fail - empty roles is valid
				roleNames = nil
			}

			// Convert role names to strings
			roleStrings := make([]string, len(roleNames))
			for i, rn := range roleNames {
				roleStrings[i] = string(rn)
			}

			// Build AuthContext
			authCtx := &AuthContext{
				PrincipalID:   principalID,
				PrincipalType: string(principal.Type),
				MemberID:      nil,
				Roles:         roleStrings,
			}

			// Continue with authenticated request
			next.ServeHTTP(w, r.WithContext(WithAuth(r.Context(), authCtx)))
		})
	}
}

// RequireAdminHTTP creates an HTTP middleware that requires admin or owner role.
// Must be used after HTTPAuthMiddleware.
func RequireAdminHTTP() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authCtx := FromContext(r.Context())
			if authCtx == nil {
				http.Error(w, `{"error":"not authenticated"}`, http.StatusUnauthorized)
				return
			}

			if !authCtx.IsAdmin() {
				http.Error(w, `{"error":"admin role required"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// OptionalAuthMiddleware creates an HTTP middleware that attempts JWT auth but allows unauthenticated requests.
// Useful for endpoints that work differently for authenticated vs anonymous users.
func OptionalAuthMiddleware(principals PrincipalStore, roles RoleStore, verifier TokenVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract token from Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				// No auth - continue as anonymous
				next.ServeHTTP(w, r)
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Try to verify token
			principalID, err := verifier.Verify(token)
			if err != nil {
				// Invalid token - continue as anonymous
				next.ServeHTTP(w, r)
				return
			}

			// Look up principal
			principal, err := principals.GetPrincipal(r.Context(), principalID)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			// Check principal status
			switch principal.Status {
			case store.PrincipalStatusApproved, store.PrincipalStatusOnline, store.PrincipalStatusOffline:
				// allowed - continue below
			default:
				// Invalid status - continue as anonymous
				next.ServeHTTP(w, r)
				return
			}

			// Look up roles
			roleNames, _ := roles.ListRoles(r.Context(), store.RoleSubjectPrincipal, principalID)
			roleStrings := make([]string, len(roleNames))
			for i, rn := range roleNames {
				roleStrings[i] = string(rn)
			}

			// Build AuthContext
			authCtx := &AuthContext{
				PrincipalID:   principalID,
				PrincipalType: string(principal.Type),
				MemberID:      nil,
				Roles:         roleStrings,
			}

			next.ServeHTTP(w, r.WithContext(WithAuth(r.Context(), authCtx)))
		})
	}
}
