# QR Pairing — Gateway Side

## Overview

The coven-app client shipped QR pairing on 2026-08-22 (coven-app `docs/plans/2026-08-22-qr-pairing-design.md`): it parses `coven://pair` deep links, scans QR codes on iOS, accepts pasted links on macOS, and POSTs to `POST /api/link/pair`. Against today's gateway that endpoint 404s and the app shows "this gateway doesn't support QR pairing yet" by design.

This spec adds the gateway half: minting single-use pairing tokens from the authed admin UI, rendering them as QR codes on `/admin/link`, and enrolling devices that present a valid token — skipping the manual approval step because an admin authorized the enrollment by minting.

The wire contract below was posted to the gateway team bbs thread on 2026-08-22 and is frozen: the client is already released against it. Everything else in this spec is gateway-internal and negotiable.

## The Pairing Contract (frozen)

**QR payload / deep link:**

```
coven://pair?v=1&host=<canonical-tailnet-hostname>&port=<grpc-port>&token=<opaque>
```

- `v` must be `1`.
- `host` is the canonical tailnet hostname, never a raw IP — TLS certs cannot cover IPs, so an IP host guarantees a handshake failure on the client.
- `port` is the gRPC port, optional (1–65535), default 50051. The client derives the HTTPS port itself: 50051 → 443, anything else → same port.
- `token` is opaque and URL-safe. The client never interprets it.

**Enrollment endpoint:** `POST /api/link/pair`, unauthenticated, JSON body:

```json
{"token": "...", "fingerprint": "<64-char SHA-256 hex>", "device_name": "..."}
```

- `200 {"principal_id": "<uuid>"}` — device enrolled exactly as if an admin had approved a `/api/link/request`. No JWT in the response: the shipped client saves only `principal_id` and authenticates gRPC with SSH key signatures thereafter.
- `401 {"error": "<reason>"}` — one status for every rejection (missing, malformed, unknown, expired, already used). The client logs the reason and shows the user a generic failure; the reason string is for operators.
- The client treats `404` as "gateway doesn't support QR pairing" — old gateways need no changes.

**Token semantics:** minted only from an authed `/admin` session; single-use (consumed on first 200); 5-minute TTL; compared in constant time; never stored in plaintext; every mint and enrollment appears in the admin audit trail; enrolled devices are revocable like any linked device.

## Design

### Token storage

New `pair_tokens` table in `schemaAdminSQL` (`internal/store/sqlite.go`), modeled on `link_codes` and `admin_invites`:

```sql
CREATE TABLE IF NOT EXISTS pair_tokens (
  id TEXT PRIMARY KEY,                              -- UUID v4
  token_hash TEXT UNIQUE NOT NULL,                  -- hex SHA-256 of the token
  created_by TEXT NOT NULL REFERENCES admin_users(id),
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  used_at TEXT,                                     -- NULL until consumed
  principal_id TEXT REFERENCES principals(principal_id)  -- set on consumption
);
CREATE INDEX IF NOT EXISTS idx_pair_tokens_expires ON pair_tokens(expires_at);
```

The token itself is 32 bytes from `crypto/rand`, base64 raw-URL encoded (43 chars). Only its SHA-256 hex digest is stored; the plaintext exists in the mint HTTP response and the QR image, nowhere else — never in the database, logs, or audit detail.

Store interface `PairTokenStore` in a new `internal/store/pair.go`, implemented by `SQLiteStore`:

- `CreatePairToken(ctx, tokenHash, createdBy string, expiresAt time.Time) (*PairToken, error)` — the store generates the row `id`
- `GetPairTokenByHash(ctx, tokenHash string) (*PairToken, error)`
- `ConsumePairToken(ctx, id string) error` — the claim: `UPDATE pair_tokens SET used_at = ? WHERE id = ? AND used_at IS NULL`; zero rows affected returns `ErrPairTokenUsed`, making double-use race-proof at the database.
- `SetPairTokenPrincipal(ctx, id, principalID string) error` — records the enrollment linkage after the principal exists.
- `DeleteExpiredPairTokens(ctx) error`

Claiming is separate from principal linkage because the claim must happen **before** enrollment: two devices racing one token must resolve to exactly one enrollment, and the principal ID does not exist until enrollment runs.

Lookup is by hash (indexed); a `crypto/subtle.ConstantTimeCompare` of the stored and computed digests satisfies the constant-time requirement — SHA-256 preimage resistance already blunts timing on the index lookup, the explicit compare is belt and braces.

`PairTokenDuration = 5 * time.Minute` lives beside `LinkCodeDuration` in `internal/webadmin/webadmin.go`.

### Mint endpoint

`POST /api/admin/link/pair-token` — registered inside `requireAuth`, CSRF-checked like `handleLinkApprove` (form-urlencoded `csrf_token`), responding JSON:

```json
{"url": "coven://pair?v=1&host=...&token=...", "qr": "data:image/png;base64,...", "expires_at": "2026-08-23T..."}
```

The handler: generate token → store hash → build the payload URL → render QR → audit → respond. The plaintext token appears only in this response body.

**Host:** hostname parsed from the already-resolved `Config.BaseURL` (`determineWebAdminBaseURL` in `internal/gateway/gateway.go` — explicit config, then `COVEN_GATEWAY_URL`, then tailscale auto-detect). If that hostname parses as an IP literal, the handler refuses to mint (`409` with a message telling the operator to set `webadmin.base_url` or `COVEN_GATEWAY_URL` to the tailnet DNS name) — an IP in the QR would fail on every client, so failing loudly at mint beats a mystery on the phone.

**Port:** new `GRPCPort int` field on `webadmin.Config`, following the `BaseURL` plumbing precedent. In tailscale mode the tsnet gRPC listener is hardcoded to `:50051` (`setupTailscaleListeners`), so the value is 50051; in direct mode it parses from `cfg.Server.GRPCAddr` (split host:port). Omitted from the payload when 50051 (the client default) to keep the QR sparse.

### QR rendering

Server-side in Go with `github.com/skip2/go-qrcode` (new dependency: mature, zero transitive deps, renders PNG bytes in one call). Rendered at mint time into a base64 data URI in the JSON response — no image route, no caching, the QR dies with the page. A JS QR library was rejected: `web/` dependencies are exact-pinned with a deliberate-updates policy mid-redesign, and the token would still need shipping to the browser either way.

### Admin UI

`LinkPage.svelte` (island `link-page`) grows a "Pair by QR" section: a mint button, the QR image, the `coven://pair` URL as selectable text (the macOS client pastes the link instead of scanning), and a countdown to `expires_at` that replaces the QR with a "expired — mint another" state. Pending-code approval UI is untouched. No new island; no props changes — the section drives itself through the mint endpoint. Frontend work follows `docs/plans/frontend-redesign/RUNBOOK.md` session rules.

### Enrollment endpoint

`POST /api/link/pair` — registered in `registerRootRoutes` beside `/api/link/request`, deliberately outside `requireAuth`. Handler shape follows `handleLinkRequest` (inline request struct, same field validation: fingerprint exactly 64 hex chars, device_name 1–100 chars) but responds with the contract's JSON envelope in all cases.

Flow:

1. Rate-limit check (below) → `401 {"error":"too many attempts"}`.
2. Validate body → `401` with a field-specific reason.
3. `DeleteExpiredPairTokens` (eager cleanup, the `DeleteExpiredLinkCodes` pattern), then hash the presented token and look it up → unknown hash: `401 {"error":"invalid token"}`.
4. Expired → `401 {"error":"token expired"}`. Already used → `401 {"error":"token already used"}`.
5. `ConsumePairToken` — the claim. `ErrPairTokenUsed` here means a concurrent request won the race: `401 {"error":"token already used"}`.
6. Enroll: shared helper extracted from `getOrCreatePrincipalForLink` (`internal/webadmin/webadmin.go`) so both flows call one function taking `(fingerprint, deviceName)` — reuse existing principal by pubkey, else `CreatePrincipal` (type agent, status approved) + member role. Failure after the claim → `500`; the token stays burned (fail closed — the admin mints another).
7. `SetPairTokenPrincipal`, audit, then `200 {"principal_id":"..."}`.

No JWT is generated. The code flow mints one in `generateApprovalToken`, but the shipped client ignores it and authenticates gRPC via SSH key signatures against the enrolled fingerprint; pair omits it on purpose (YAGNI, and one less credential in flight).

The raw token is never logged — slog lines carry the token row `id` once known, or nothing.

### Rate limiting

Same sliding-window shape as `loginRateLimiter` (`internal/webadmin/login_ratelimit.go`), keyed by remote IP, applied to `/api/link/pair` attempts. The real defense is 256-bit random tokens with a 5-minute TTL — the limiter caps brute-force noise and log spam, which matters because tailscale funnel can expose this endpoint to the public internet. Caveat noted in code: behind funnel the observed remote address may be an ingress address, coarsening the limit; acceptable for defense-in-depth.

### Audit

Three new `audit_log` actions, which require extending the schema's CHECK constraint via a migration (precedent: `migrateAuditLogCheckConstraint` in `internal/store/sqlite.go`):

| Action | Actor (`actor_principal_id`) | Target | Detail |
|---|---|---|---|
| `mint_pair_token` | admin user ID | `pair_token` / row id | `{"username": ..., "expires_at": ...}` |
| `pair_enroll` | enrolled principal ID | `principal` / principal id | `{"device_name": ..., "pair_token_id": ..., "reused_principal": bool}` |
| `approve_link` | admin user ID | `principal` / principal id | `{"device_name": ..., "link_code_id": ...}` |

`approve_link` fixes a pre-existing gap found while mapping this work: `handleLinkApprove` creates principals and 30-day JWTs today with **no audit entry at all**. The fix is one `AppendAuditLog` call at the same seam this spec already builds; leaving it unaudited while auditing the QR path would make the audit trail lie by omission. (`actor_principal_id` is a plain `TEXT NOT NULL` column with no FK, so admin user IDs are valid actors — `internal/admin` callers already established the AuditEntry shape.)

Entries surface wherever `ListAuditLog` renders today; no UI changes.

## Error handling

| Condition | Response |
|---|---|
| Rate limited | `401 {"error":"too many attempts"}` |
| Malformed JSON / missing field / bad fingerprint or device_name | `401 {"error":"invalid request: <field>"}` |
| Unknown token | `401 {"error":"invalid token"}` |
| Expired token | `401 {"error":"token expired"}` |
| Used token (including lost race) | `401 {"error":"token already used"}` |
| Store failure | `500 {"error":"internal error"}` (details to slog only) |
| Mint with IP-literal host | `409` plain text with remediation (authed admin sees it) |

One 401 status for all rejections is contractual: the client must not be able to distinguish "expired" from "never existed" by status code, and neither can an attacker.

## Security posture

- Bearer-token-in-QR is the accepted model (decided in the client spec): anyone who captures the QR within 5 minutes can enroll one device. Mitigations: authed-mint-only, single-use, short TTL, hash-only storage, audit trail, revocability via the existing principals UI.
- The mint endpoint sits behind session auth + CSRF; the enroll endpoint is unauthenticated by necessity and rate-limited.
- Token comparison is constant-time; the plaintext token never touches disk or logs.

## Testing

Store tests (`internal/store`, real SQLite in `t.TempDir()`): create/get round-trip, hash uniqueness, expiry filtering, consume sets `used_at`, double-consume returns `ErrPairTokenUsed`, concurrent consume race (two goroutines, exactly one wins), `SetPairTokenPrincipal` records linkage, delete-expired.

Webadmin tests (`internal/webadmin`, httptest + `newTestAdminWithStore(t)` conventions, beside `webadmin_link_test.go`): mint requires auth and CSRF; mint response contains a parseable `coven://pair` URL, PNG data URI, and correct expiry; mint refuses IP-literal hosts; pair happy path enrolls a new principal (status approved, role member) and returns its ID; pair reuses an existing principal by fingerprint; 401s for unknown/expired/used tokens, bad fingerprint, bad device_name, oversized body; second use of a consumed token 401s; audit entries asserted for mint, pair_enroll, and approve_link (per repo convention: new exported behavior in security-relevant packages verifies its audit writes).

Frontend: new colocated `web/src/lib/components/LinkPage.test.ts` (vitest + @testing-library/svelte, the per-component convention) covering the mint section: button renders, successful mint shows QR + URL + countdown, CSRF header sent, failure shows an error.

Acceptance (live e2e, from the client spec — runs on real hardware once both halves ship): mint on `/admin/link` → scan with an iPhone → device enrolled and chatting → rescan the same QR → generic failure (401 underneath) → mint again, wait 5 minutes → expiry failure.

## Non-goals

- Mint from CLI (`coven-admin`) — admin web only, matching where link approval lives today.
- Mint from an already-linked device (device-to-device pairing) — possible later phase; nothing here precludes it.
- JWT issuance on pair — see enrollment section.
- Changes to the existing code-based link flow beyond the shared enrollment helper and the `approve_link` audit fix.

## Files touched

- `internal/store/sqlite.go` — `pair_tokens` DDL, audit CHECK migration, `PairTokenStore` impl
- `internal/store/pair.go` — new: `PairToken`, `PairTokenStore`, errors
- `internal/store/audit.go` — three new `AuditAction` constants
- `internal/webadmin/webadmin.go` — mint handler, pair handler, route registrations, shared enrollment helper, `approve_link` audit call, `PairTokenDuration`, `Config.GRPCPort`
- `internal/webadmin/login_ratelimit.go` (or sibling) — IP-keyed limiter for pair
- `internal/gateway/gateway.go` — plumb `GRPCPort` into `webadmin.Config`
- `web/src/lib/components/LinkPage.svelte` — QR mint section
- `go.mod` — `github.com/skip2/go-qrcode`
- Tests: `internal/store/pair_test.go`, `internal/webadmin/webadmin_pair_test.go`, `web/src/lib/components/LinkPage.test.ts` (new)
