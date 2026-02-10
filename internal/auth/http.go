// ABOUTME: HTTP middleware for JWT authentication on API endpoints
// ABOUTME: Extracts JWT from Authorization header and adds principal to context

package auth

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/2389/coven-gateway/internal/store"
)

// errorResponse is the JSON structure for error responses.
type errorResponse struct {
	Error string `json:"error"`
}

// jsonError writes a JSON error response with the given status code.
func jsonError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(errorResponse{Error: message}); err != nil {
		// If JSON encoding fails, the response is already partially written.
		// Log would be ideal but we don't have a logger here; silently fail.
		_ = err
	}
}

// extractBearerToken extracts a bearer token from the Authorization header.
// Returns the token and an error message (empty if successful).
func extractBearerToken(authHeader string) (string, string) {
	if authHeader == "" {
		return "", "missing authorization header"
	}
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return "", "invalid authorization header format"
	}
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == "" {
		return "", "empty token"
	}
	return token, ""
}

// checkPrincipalStatus validates that a principal has an allowed status.
// Returns an error message (empty if allowed).
func checkPrincipalStatus(status store.PrincipalStatus) string {
	switch status {
	case store.PrincipalStatusApproved, store.PrincipalStatusOnline, store.PrincipalStatusOffline:
		return ""
	case store.PrincipalStatusPending:
		return "principal status is pending"
	case store.PrincipalStatusRevoked:
		return "principal has been revoked"
	default:
		return "unknown principal status"
	}
}

// buildAuthContext creates an AuthContext from a principal and role list.
func buildAuthContext(principalID string, principalType store.PrincipalType, roleNames []store.RoleName) *AuthContext {
	roleStrings := make([]string, len(roleNames))
	for i, rn := range roleNames {
		roleStrings[i] = string(rn)
	}
	return &AuthContext{
		PrincipalID:   principalID,
		PrincipalType: string(principalType),
		MemberID:      nil,
		Roles:         roleStrings,
	}
}

// HTTPAuthMiddleware creates an HTTP middleware that extracts and validates JWT tokens.
// It looks up the principal and adds AuthContext to the request context using the same
// WithAuth/FromContext pattern as the gRPC interceptors for consistency.
func HTTPAuthMiddleware(principals PrincipalStore, roles RoleStore, verifier TokenVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, errMsg := extractBearerToken(r.Header.Get("Authorization"))
			if errMsg != "" {
				jsonError(w, errMsg, http.StatusUnauthorized)
				return
			}

			principalID, err := verifier.Verify(token)
			if err != nil {
				jsonError(w, "invalid token", http.StatusUnauthorized)
				return
			}

			principal, err := principals.GetPrincipal(r.Context(), principalID)
			if err != nil {
				jsonError(w, "principal not found", http.StatusUnauthorized)
				return
			}

			if errMsg = checkPrincipalStatus(principal.Status); errMsg != "" {
				status := http.StatusForbidden
				if errMsg == "unknown principal status" {
					status = http.StatusInternalServerError
				}
				jsonError(w, errMsg, status)
				return
			}

			roleNames, _ := roles.ListRoles(r.Context(), store.RoleSubjectPrincipal, principalID)
			authCtx := buildAuthContext(principalID, principal.Type, roleNames)
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
				jsonError(w, "not authenticated", http.StatusUnauthorized)
				return
			}

			if !authCtx.IsAdmin() {
				jsonError(w, "admin role required", http.StatusForbidden)
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
			token, errMsg := extractBearerToken(r.Header.Get("Authorization"))
			if errMsg != "" {
				next.ServeHTTP(w, r) // Continue as anonymous
				return
			}

			principalID, err := verifier.Verify(token)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			principal, err := principals.GetPrincipal(r.Context(), principalID)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			if checkPrincipalStatus(principal.Status) != "" {
				next.ServeHTTP(w, r)
				return
			}

			roleNames, _ := roles.ListRoles(r.Context(), store.RoleSubjectPrincipal, principalID)
			authCtx := buildAuthContext(principalID, principal.Type, roleNames)
			next.ServeHTTP(w, r.WithContext(WithAuth(r.Context(), authCtx)))
		})
	}
}
