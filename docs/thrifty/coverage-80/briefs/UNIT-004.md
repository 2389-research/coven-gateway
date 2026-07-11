# UNIT-004 — webadmin link + invite flows

## Objective

Cover the device-link flow (request/status/approve/page/JSON) and the invite flow
(create/page/signup), including their validators and error renderers.

## Inputs / context

- `newTestAdminWithStore(t)`, `requestWithUser(r)`.
- Baseline 0% functions owned: `handleLinkPage`, `handleLinkRequest`, `handleLinkStatus`,
  `handleLinkApprove`, `handleLinkJSON`, `validatePendingLinkCode`, `handleInvitePage`,
  `handleInviteSignup`, `handleCreateInviteJSON`, `parseInviteSignupForm`,
  `validateInvite`, `showInviteError`.
- Read each handler in webadmin.go first (grep + region read). The link flow likely has
  a pending-code store (in `internal/store` or in-memory on Admin) — trace where
  `handleLinkRequest` writes and `handleLinkStatus`/`handleLinkApprove` read.

## Approach

1. **Link flow end-to-end:** `handleLinkRequest` creates a pending code (parse it from
   the response) → `handleLinkStatus` reports pending → `handleLinkApprove` (as an
   authenticated admin, CSRF as required) approves it → `handleLinkStatus` reflects
   approval and yields whatever credential/token the flow issues. Assert each step's
   observable output. Then the error branches: unknown code on status/approve, malformed
   code (`validatePendingLinkCode` edge cases — call it directly too), expired code if
   expiry is reachable by constructing an aged entry through the same path the code
   uses (only if reachable without source edits).
2. **`handleLinkPage` / `handleLinkJSON`:** render with none and with a pending code.
3. **Invite flow end-to-end:** `handleCreateInviteJSON` (authenticated) → parse invite
   code/URL → `handleInvitePage` with valid code renders signup; with invalid/used code
   renders error (`validateInvite`, `showInviteError`) → `handleInviteSignup` happy path
   creates the admin user (assert via store) and rejects: password mismatch, weak/short
   password, taken username, invalid code (`parseInviteSignupForm` branches).
4. Package tests with `-cover`; note %. Commit:
   `test(webadmin): cover device-link and invite flows`.

## Constraints

- Contract prohibitions. Drive flows through the handlers end-to-end (real behavior);
  reach into the store directly only to seed or to assert final state.

## Acceptance criteria

- [ ] (runnable) `go test -count=1 ./internal/webadmin/` passes.
- [ ] (runnable) `go tool cover -func`: every listed function > 0%; the two flow
      entrypoints (`handleLinkRequest`, `handleInviteSignup`) ≥ 75%.
- [ ] (assertional) Both flows tested end-to-end (request→status→approve;
      create→page→signup) with at least three distinct rejection branches across the
      validators.

## Dependencies

UNIT-003
