// ABOUTME: Ledger event recording with actor attribution from AuthContext
// ABOUTME: Provides recordEvent that populates actor fields for audit trail

package gateway

import (
	"context"

	"github.com/2389/coven-gateway/internal/auth"
	"github.com/2389/coven-gateway/internal/store"
)

// recordEvent saves a ledger event to the store, populating actor fields from AuthContext.
// If the event already has ActorPrincipalID set, it will not be overridden.
//
// Actor attribution rules:
//   - TUI/Web/Swift client: ActorPrincipalID = client principal, ActorMemberID = member if linked
//   - Bridge (Matrix/Slack): ActorPrincipalID = bridge principal, remote user in Author field
//   - Agent response: ActorPrincipalID = agent principal
//   - Pack event: ActorPrincipalID = pack principal
//   - No auth context: both actor fields remain nil (system events, pre-auth)
func (g *Gateway) recordEvent(ctx context.Context, event *store.LedgerEvent) error {
	// Only populate actor if not already set (don't override forwarded events)
	if event.ActorPrincipalID == nil {
		authCtx := auth.FromContext(ctx)
		if authCtx != nil {
			event.ActorPrincipalID = &authCtx.PrincipalID
			if authCtx.MemberID != nil {
				event.ActorMemberID = authCtx.MemberID
			}
		}
	}

	return g.store.SaveEvent(ctx, event)
}
