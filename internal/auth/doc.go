// Package auth provides authentication and authorization for coven-gateway.
//
// # Authentication Methods
//
// The package supports multiple authentication methods:
//
//   - SSH Signatures: Agents authenticate by signing a challenge with their SSH key.
//     The gateway verifies the signature against the agent's registered public key.
//
//   - JWT Tokens: Human users and API clients authenticate with JWT tokens.
//     Tokens are signed with HS256 using the configured jwt_secret.
//
//   - WebAuthn: Admin users can register passkeys for passwordless authentication.
//     Requires HTTPS (enforced by browsers).
//
// # Principal System
//
// All identities are represented as principals with:
//
//   - ID: Unique identifier (e.g., "agent:xxx", "user:xxx")
//   - Type: "agent", "user", or "admin"
//   - Capabilities: String set (e.g., "base", "notes", "admin")
//
// Principals are stored in the database and referenced throughout the system
// for audit trails and access control.
//
// # gRPC Interceptors
//
// The package provides gRPC interceptors for agent authentication:
//
//	AuthInterceptor(store, logger) // Returns unary and stream interceptors
//
// Agents send their registration with SSH public key fingerprint. The interceptor
// validates the registration and creates/retrieves the principal.
//
// # Token Management
//
// API tokens for principals:
//
//	token, err := GenerateToken(principalID, secret)
//	claims, err := ValidateToken(token, secret)
//
// Tokens include:
//   - Principal ID
//   - Expiration time
//   - Custom claims (optional)
//
// # Agent Registration Modes
//
// Configurable via auth.agent_auto_registration:
//
//   - "approved": New agents can connect immediately (development)
//   - "pending": New agents require admin approval (recommended for production)
//   - "disabled": Unknown agents are rejected (strict mode)
package auth
