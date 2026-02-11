// Package admin implements gRPC server handlers for the AdminService.
//
// # Overview
//
// This package provides the server-side implementation of the AdminService
// gRPC service, used by the coven-admin CLI tool and web admin interface
// for administrative operations.
//
// # Service Methods
//
// The AdminService implements these gRPC methods:
//
// Principal management:
//
//   - ListPrincipals: List all principals with optional filters
//   - GetPrincipal: Get a single principal by ID
//   - CreatePrincipal: Create a new principal
//   - DeletePrincipal: Remove a principal
//
// Token management:
//
//   - ListTokens: List API tokens
//   - CreateToken: Generate a new API token
//   - RevokeToken: Invalidate a token
//
// Binding management:
//
//   - ListBindings: List channel-to-agent bindings
//   - CreateBinding: Create a new binding
//   - DeleteBinding: Remove a binding
//
// # Principal Types
//
// The system tracks three types of principals:
//
//   - agent: AI agents (coven-agent instances)
//   - user: Human users (via frontends)
//   - admin: Administrative users (web admin access)
//
// # Principal Status
//
// Agents have status affecting their connection ability:
//
//   - pending: Awaiting admin approval
//   - approved: Can connect and receive messages
//   - revoked: Access denied
//
// # Capabilities
//
// Capabilities control what tools a principal can use:
//
//   - base: Basic tools (logging, todos, BBS)
//   - notes: Key-value note storage
//   - mail: Inter-agent messaging
//   - admin: Administrative tools
//   - ui: User interaction tools
//
// # Authentication
//
// Admin methods require authentication via gRPC metadata:
//
//	md := metadata.Pairs("authorization", "Bearer <admin-token>")
//	ctx := metadata.NewOutgoingContext(ctx, md)
//
// # Usage
//
// The AdminService is typically created by the gateway:
//
//	service := admin.NewPrincipalService(store, tokenGenerator)
//	pb.RegisterAdminServiceServer(grpcServer, service)
package admin
