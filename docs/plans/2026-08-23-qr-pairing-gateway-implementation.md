# QR Pairing — Gateway Side Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let an admin mint a single-use QR pairing token on `/admin/link` and let a device holding that token enroll itself via `POST /api/link/pair` — completing the gateway half of the QR pairing contract the coven-app client already shipped against.

**Architecture:** A new `pair_tokens` table stores SHA-256 hashes of 5-minute single-use tokens. An authed+CSRF mint endpoint generates the token, renders a `coven://pair` QR PNG server-side (go-qrcode), and returns both as JSON to a new section of the `LinkPage` Svelte island. An unauthenticated, rate-limited enroll endpoint validates the token, claims it atomically (UPDATE … WHERE used_at IS NULL), then enrolls the device through a helper shared with the existing code-approval flow. Three new audit actions land, including a fix for the pre-existing unaudited link approval.

**Tech Stack:** Go 1.x (`net/http` mux patterns, database/sql + SQLite), `github.com/skip2/go-qrcode` (new dep), Svelte 5 runes islands, vitest + @testing-library/svelte.

**Spec:** `docs/plans/2026-08-23-qr-pairing-gateway-design.md` — read it first; it is the binding authority, including the frozen wire contract.

## Global Constraints

- **Workspace:** The main coven-gateway checkout is on `feat/datatable` with a dirty tree — someone else's active work. NEVER build on it, switch its branch, or touch its files. Work in a fresh worktree branched from `main`: `git worktree add .worktrees/qr-pairing-gateway -b feat/qr-pairing-gateway main` (Task 1 Step 1). All commands below run from that worktree root.
- **Frozen contract (client already shipped):** payload `coven://pair?v=1&host=<hostname>&port=<grpc-port>&token=<opaque>`; `host` is never an IP; `port` omitted when 50051; enroll endpoint `POST /api/link/pair` returns `200 {"principal_id":"<uuid>"}` on success and `401 {"error":"<reason>"}` for EVERY rejection (single status; reasons: `too many attempts`, `invalid request: <field>`, `invalid token`, `token expired`, `token already used`); store failures return `500 {"error":"internal error"}`. No JWT in the pair response.
- **Token hygiene:** token = 32 bytes `crypto/rand`, base64 RawURLEncoding (43 chars); only its hex SHA-256 is stored; the plaintext token NEVER appears in the database, logs, or audit detail — only in the mint HTTP response. TTL is `PairTokenDuration = 5 * time.Minute`.
- **Canonical checks (Go):** `go test ./...`, `go test -race ./...`, `golangci-lint run`, `make build`. Zero new warnings.
- **Canonical checks (web):** from `web/`: `npm run check`, `npm test`. Before ANY `web/` work, read `docs/plans/frontend-redesign/RUNBOOK.md` and follow its session rules. Web deps are exact-pinned; do not add npm dependencies.
- **Module path:** `github.com/2389/coven-gateway`.
- **House style:** every file starts with two `// ABOUTME:` comment lines (`# ABOUTME:` outside Go/TS). Errors wrap as `fmt.Errorf("context: %w", err)`. SQLite timestamps are RFC3339 UTC TEXT. `store.ErrNotFound` for missing rows. IDs are `uuid.New().String()`. Conventional commits.
- **Foreign keys are ON** (`PRAGMA foreign_keys=ON` in `NewSQLiteStore`): `pair_tokens.created_by` references `admin_users(id)` and `pair_tokens.principal_id` references `principals(principal_id)` — tests must create those rows before referencing them.
- **TDD:** every step pair is test-first. Run the named test, see the expected failure, implement, see it pass, commit.

---

### Task 1: Workspace bootstrap + pair token store layer

**Files:**
- Create: worktree `.worktrees/qr-pairing-gateway` on new branch `feat/qr-pairing-gateway` from `main`
- Create: `internal/store/pair.go`
- Create: `internal/store/pair_test.go`
- Modify: `internal/store/sqlite.go` (add `pair_tokens` DDL to `schemaAdminSQL`)

**Interfaces:**
- Consumes: `SQLiteStore` internals (`s.db`, `s.logger`), `ErrNotFound`, `newTestStore(t)` (exists at `internal/store/sqlite_test.go:572`), `CreateAdminUser`, `CreatePrincipal` (FK targets).
- Produces (later tasks call these exactly as written):
  - `ErrPairTokenUsed` (package `store` sentinel error)
  - `type PairToken struct { ID, TokenHash, CreatedBy string; CreatedAt, ExpiresAt time.Time; UsedAt *time.Time; PrincipalID *string }`
  - `CreatePairToken(ctx context.Context, tokenHash, createdBy string, expiresAt time.Time) (*PairToken, error)`
  - `GetPairTokenByHash(ctx context.Context, tokenHash string) (*PairToken, error)`
  - `ConsumePairToken(ctx context.Context, id string) error`
  - `SetPairTokenPrincipal(ctx context.Context, id, principalID string) error`
  - `DeleteExpiredPairTokens(ctx context.Context) error`

- [ ] **Step 1: Create the worktree from main (NOT from the current checkout)**

From the coven-gateway repo root:

```bash
git worktree add .worktrees/qr-pairing-gateway -b feat/qr-pairing-gateway main
cd .worktrees/qr-pairing-gateway
```

Verify: `git status` shows a clean tree on `feat/qr-pairing-gateway`; `git log -1 --oneline` matches `origin/main`'s tip, not `feat/datatable`.

- [ ] **Step 2: Bring the spec and this plan into the branch and commit them**

The two docs exist untracked in the MAIN checkout's `docs/plans/` (they were written there deliberately without committing, because that checkout is mid-work). Copy them in:

```bash
cp ../../docs/plans/2026-08-23-qr-pairing-gateway-design.md docs/plans/
cp ../../docs/plans/2026-08-23-qr-pairing-gateway-implementation.md docs/plans/
git add docs/plans/2026-08-23-qr-pairing-gateway-design.md docs/plans/2026-08-23-qr-pairing-gateway-implementation.md
git commit -m "docs: QR pairing gateway spec and implementation plan"
```

- [ ] **Step 3: Write the failing store tests**

Create `internal/store/pair_test.go`:

```go
// ABOUTME: Tests for pair token store operations backing QR device pairing.
// ABOUTME: Covers create/get, consume-once semantics, the consume race, and cleanup.

package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createPairTestUser satisfies the pair_tokens.created_by foreign key.
func createPairTestUser(t *testing.T, s *SQLiteStore) *AdminUser {
	t.Helper()
	u := &AdminUser{
		ID:          "user-" + uuid.New().String(),
		Username:    "pairadmin-" + uuid.New().String(),
		DisplayName: "Pair Admin",
		CreatedAt:   time.Now(),
	}
	require.NoError(t, s.CreateAdminUser(context.Background(), u))
	return u
}

func TestCreatePairToken_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	user := createPairTestUser(t, s)

	expires := time.Now().Add(5 * time.Minute)
	created, err := s.CreatePairToken(ctx, "roundtrip-hash", user.ID, expires)
	require.NoError(t, err)
	require.NotEmpty(t, created.ID)

	got, err := s.GetPairTokenByHash(ctx, "roundtrip-hash")
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
	assert.Equal(t, "roundtrip-hash", got.TokenHash)
	assert.Equal(t, user.ID, got.CreatedBy)
	assert.WithinDuration(t, expires, got.ExpiresAt, 2*time.Second)
	assert.Nil(t, got.UsedAt)
	assert.Nil(t, got.PrincipalID)
}

func TestGetPairTokenByHash_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetPairTokenByHash(context.Background(), "no-such-hash")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestCreatePairToken_DuplicateHashRejected(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	user := createPairTestUser(t, s)

	_, err := s.CreatePairToken(ctx, "dup-hash", user.ID, time.Now().Add(5*time.Minute))
	require.NoError(t, err)
	_, err = s.CreatePairToken(ctx, "dup-hash", user.ID, time.Now().Add(5*time.Minute))
	assert.Error(t, err, "token_hash is UNIQUE")
}

func TestConsumePairToken_SetsUsedAtOnce(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	user := createPairTestUser(t, s)

	pt, err := s.CreatePairToken(ctx, "consume-hash", user.ID, time.Now().Add(5*time.Minute))
	require.NoError(t, err)

	require.NoError(t, s.ConsumePairToken(ctx, pt.ID))

	got, err := s.GetPairTokenByHash(ctx, "consume-hash")
	require.NoError(t, err)
	require.NotNil(t, got.UsedAt)
	assert.WithinDuration(t, time.Now(), *got.UsedAt, 5*time.Second)

	err = s.ConsumePairToken(ctx, pt.ID)
	assert.ErrorIs(t, err, ErrPairTokenUsed)
}

func TestConsumePairToken_ConcurrentRace(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	user := createPairTestUser(t, s)

	pt, err := s.CreatePairToken(ctx, "race-hash", user.ID, time.Now().Add(5*time.Minute))
	require.NoError(t, err)

	start := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			results <- s.ConsumePairToken(ctx, pt.ID)
		}()
	}
	close(start)

	var wins, losses int
	for i := 0; i < 2; i++ {
		switch err := <-results; {
		case err == nil:
			wins++
		case errors.Is(err, ErrPairTokenUsed):
			losses++
		default:
			t.Fatalf("unexpected error from concurrent consume: %v", err)
		}
	}
	assert.Equal(t, 1, wins, "exactly one consumer must win")
	assert.Equal(t, 1, losses, "the loser must see ErrPairTokenUsed")
}

func TestSetPairTokenPrincipal(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	user := createPairTestUser(t, s)

	// principal_id has a foreign key to principals.
	principal := &Principal{
		ID:          uuid.New().String(),
		Type:        PrincipalTypeAgent,
		PubkeyFP:    strings.Repeat("a", 64),
		DisplayName: "Paired Device",
		Status:      PrincipalStatusApproved,
		CreatedAt:   time.Now(),
	}
	require.NoError(t, s.CreatePrincipal(ctx, principal))

	pt, err := s.CreatePairToken(ctx, "principal-hash", user.ID, time.Now().Add(5*time.Minute))
	require.NoError(t, err)

	require.NoError(t, s.SetPairTokenPrincipal(ctx, pt.ID, principal.ID))

	got, err := s.GetPairTokenByHash(ctx, "principal-hash")
	require.NoError(t, err)
	require.NotNil(t, got.PrincipalID)
	assert.Equal(t, principal.ID, *got.PrincipalID)
}

func TestSetPairTokenPrincipal_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	user := createPairTestUser(t, s)

	principal := &Principal{
		ID:          uuid.New().String(),
		Type:        PrincipalTypeAgent,
		PubkeyFP:    strings.Repeat("b", 64),
		DisplayName: "Paired Device",
		Status:      PrincipalStatusApproved,
		CreatedAt:   time.Now(),
	}
	require.NoError(t, s.CreatePrincipal(ctx, principal))
	_ = user

	err := s.SetPairTokenPrincipal(ctx, "no-such-id", principal.ID)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestDeleteExpiredPairTokens(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	user := createPairTestUser(t, s)

	// Expired + unused: should be deleted.
	_, err := s.CreatePairToken(ctx, "expired-hash", user.ID, time.Now().Add(-time.Minute))
	require.NoError(t, err)
	// Live: should survive.
	_, err = s.CreatePairToken(ctx, "live-hash", user.ID, time.Now().Add(5*time.Minute))
	require.NoError(t, err)
	// Expired + used: should survive (enrollment linkage is kept).
	usedPT, err := s.CreatePairToken(ctx, "used-hash", user.ID, time.Now().Add(-time.Minute))
	require.NoError(t, err)
	require.NoError(t, s.ConsumePairToken(ctx, usedPT.ID))

	require.NoError(t, s.DeleteExpiredPairTokens(ctx))

	_, err = s.GetPairTokenByHash(ctx, "expired-hash")
	assert.ErrorIs(t, err, ErrNotFound, "expired unused token must be deleted")
	_, err = s.GetPairTokenByHash(ctx, "live-hash")
	assert.NoError(t, err, "live token must survive")
	_, err = s.GetPairTokenByHash(ctx, "used-hash")
	assert.NoError(t, err, "used token must survive for its enrollment record")
}
```

- [ ] **Step 4: Run the tests to verify they fail**

Run: `go test ./internal/store/ -run 'PairToken' -v`
Expected: compile FAILURE — `undefined: s.CreatePairToken`, `undefined: ErrPairTokenUsed`, etc.

- [ ] **Step 5: Add the `pair_tokens` DDL to the schema**

In `internal/store/sqlite.go`, inside the `schemaAdminSQL` constant, immediately after the `link_codes` CREATE TABLE line and its `idx_link_codes_*` index lines, add (matching the file's one-statement-per-line style):

```sql
CREATE TABLE IF NOT EXISTS pair_tokens (id TEXT PRIMARY KEY, token_hash TEXT UNIQUE NOT NULL, created_by TEXT NOT NULL REFERENCES admin_users(id), created_at TEXT NOT NULL, expires_at TEXT NOT NULL, used_at TEXT, principal_id TEXT REFERENCES principals(principal_id));
CREATE INDEX IF NOT EXISTS idx_pair_tokens_expires ON pair_tokens(expires_at);
```

- [ ] **Step 6: Implement the store layer**

Create `internal/store/pair.go`. The scan/error/timestamp idioms mirror the link-code implementation in `sqlite.go` (see `DeleteExpiredLinkCodes` for the logging line style):

```go
// ABOUTME: Pair token entity and store operations for QR-code device pairing.
// ABOUTME: Tokens are single-use, short-lived, and stored only as SHA-256 hex hashes.

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ErrPairTokenUsed indicates the pair token was already consumed.
var ErrPairTokenUsed = errors.New("pair token already used")

// PairToken is a single-use QR pairing token. Only the SHA-256 hex digest of
// the token is stored; the plaintext never touches the database.
type PairToken struct {
	ID          string     // UUID v4
	TokenHash   string     // hex SHA-256 of the token
	CreatedBy   string     // admin user ID that minted it
	CreatedAt   time.Time
	ExpiresAt   time.Time
	UsedAt      *time.Time // nil until consumed
	PrincipalID *string    // set after enrollment
}

// PairTokenStore defines pair token persistence operations.
type PairTokenStore interface {
	// CreatePairToken stores a new token hash; the store generates the row ID.
	CreatePairToken(ctx context.Context, tokenHash, createdBy string, expiresAt time.Time) (*PairToken, error)
	// GetPairTokenByHash returns the token row for a hash, or ErrNotFound.
	GetPairTokenByHash(ctx context.Context, tokenHash string) (*PairToken, error)
	// ConsumePairToken claims the token; ErrPairTokenUsed if already claimed.
	ConsumePairToken(ctx context.Context, id string) error
	// SetPairTokenPrincipal records the enrolled principal on a token row.
	SetPairTokenPrincipal(ctx context.Context, id, principalID string) error
	// DeleteExpiredPairTokens removes expired tokens that were never used.
	DeleteExpiredPairTokens(ctx context.Context) error
}

// CreatePairToken stores a new pair token hash and returns the created row.
func (s *SQLiteStore) CreatePairToken(ctx context.Context, tokenHash, createdBy string, expiresAt time.Time) (*PairToken, error) {
	pt := &PairToken{
		ID:        uuid.New().String(),
		TokenHash: tokenHash,
		CreatedBy: createdBy,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: expiresAt.UTC(),
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO pair_tokens (id, token_hash, created_by, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?)
	`, pt.ID, pt.TokenHash, pt.CreatedBy,
		pt.CreatedAt.Format(time.RFC3339), pt.ExpiresAt.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("creating pair token: %w", err)
	}
	return pt, nil
}

// GetPairTokenByHash returns the token row matching a hex SHA-256 digest.
func (s *SQLiteStore) GetPairTokenByHash(ctx context.Context, tokenHash string) (*PairToken, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, token_hash, created_by, created_at, expires_at, used_at, principal_id
		FROM pair_tokens WHERE token_hash = ?
	`, tokenHash)
	return scanPairToken(row)
}

func scanPairToken(row *sql.Row) (*PairToken, error) {
	var pt PairToken
	var createdAt, expiresAt string
	var usedAt, principalID sql.NullString
	err := row.Scan(&pt.ID, &pt.TokenHash, &pt.CreatedBy, &createdAt, &expiresAt, &usedAt, &principalID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning pair token: %w", err)
	}
	pt.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parsing pair token created_at: %w", err)
	}
	pt.ExpiresAt, err = time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("parsing pair token expires_at: %w", err)
	}
	if usedAt.Valid {
		t, err := time.Parse(time.RFC3339, usedAt.String)
		if err != nil {
			return nil, fmt.Errorf("parsing pair token used_at: %w", err)
		}
		pt.UsedAt = &t
	}
	if principalID.Valid {
		pt.PrincipalID = &principalID.String
	}
	return &pt, nil
}

// ConsumePairToken claims the token: it sets used_at if and only if the token
// is still unused. Exactly one concurrent caller succeeds; the rest get
// ErrPairTokenUsed. (An unknown id also reports ErrPairTokenUsed — callers
// look the row up by hash first.)
func (s *SQLiteStore) ConsumePairToken(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx, `
		UPDATE pair_tokens SET used_at = ? WHERE id = ? AND used_at IS NULL
	`, now, id)
	if err != nil {
		return fmt.Errorf("consuming pair token: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("consuming pair token: %w", err)
	}
	if rowsAffected == 0 {
		return ErrPairTokenUsed
	}
	return nil
}

// SetPairTokenPrincipal records the enrolled principal after enrollment runs.
func (s *SQLiteStore) SetPairTokenPrincipal(ctx context.Context, id, principalID string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE pair_tokens SET principal_id = ? WHERE id = ?
	`, principalID, id)
	if err != nil {
		return fmt.Errorf("setting pair token principal: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("setting pair token principal: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteExpiredPairTokens removes expired tokens that were never used.
// Used tokens are kept: their principal_id linkage documents the enrollment.
func (s *SQLiteStore) DeleteExpiredPairTokens(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM pair_tokens WHERE expires_at <= ? AND used_at IS NULL
	`, now)
	if err != nil {
		return fmt.Errorf("deleting expired pair tokens: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		s.logger.Debug("deleted expired pair tokens", "count", rowsAffected)
	}
	return nil
}
```

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go test ./internal/store/ -run 'PairToken' -v`
Expected: all 8 tests PASS. Note: if `TestConsumePairToken_ConcurrentRace` hits `SQLITE_BUSY`/`database is locked`, that is a real finding — investigate the store's busy-timeout settings rather than serializing the test; the spec requires the two-goroutine race.

- [ ] **Step 8: Run the full store package and race detector**

Run: `go test ./internal/store/ && go test -race ./internal/store/ -run 'PairToken'`
Expected: PASS, no pre-existing tests broken.

- [ ] **Step 9: Commit**

```bash
git add internal/store/pair.go internal/store/pair_test.go internal/store/sqlite.go
git commit -m "feat: pair token store for QR pairing"
```

---

### Task 2: Audit actions + CHECK constraint migration

**Files:**
- Modify: `internal/store/audit.go` (three new `AuditAction` constants + `ValidAuditActions`)
- Modify: `internal/store/sqlite.go` (audit_log CHECK in `schemaAuthSQL` ~line 109, `needsAuditLogMigration`, `recreateAuditLogTable`, log message in `migrateAuditLogCheckConstraint`)
- Create: tests appended to `internal/store/audit_test.go`

**Interfaces:**
- Consumes: `AuditEntry`, `AppendAuditLog(ctx, *AuditEntry) error`, `newTestStore(t)`, `NewSQLiteStore(path) (*SQLiteStore, error)`, `Close() error`.
- Produces: `store.AuditMintPairToken` (= `"mint_pair_token"`), `store.AuditPairEnroll` (= `"pair_enroll"`), `store.AuditApproveLink` (= `"approve_link"`) — Tasks 3–5 write entries with exactly these constants.

- [ ] **Step 1: Write the failing tests**

Append to `internal/store/audit_test.go`:

```go
func TestAuditLogAcceptsPairActions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for _, action := range []AuditAction{AuditMintPairToken, AuditPairEnroll, AuditApproveLink} {
		err := s.AppendAuditLog(ctx, &AuditEntry{
			ActorPrincipalID: "actor-1",
			Action:           action,
			TargetType:       "principal",
			TargetID:         "target-1",
		})
		require.NoError(t, err, "action %s must pass the CHECK constraint", action)
	}

	entries, err := s.ListAuditLog(ctx, AuditFilter{})
	require.NoError(t, err)
	require.Len(t, entries, 3)
}

func TestAuditLogMigrationAddsPairActions(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "migrate-test.db")

	s, err := NewSQLiteStore(dbPath)
	require.NoError(t, err)

	// Simulate a pre-pair database: recreate audit_log with the old CHECK list.
	_, err = s.db.Exec(`DROP TABLE audit_log`)
	require.NoError(t, err)
	_, err = s.db.Exec(`CREATE TABLE audit_log (
		audit_id TEXT PRIMARY KEY,
		actor_principal_id TEXT NOT NULL,
		actor_member_id TEXT,
		action TEXT NOT NULL,
		target_type TEXT NOT NULL,
		target_id TEXT NOT NULL,
		ts TEXT NOT NULL,
		detail_json TEXT,
		CHECK (action IN ('approve_principal', 'revoke_principal', 'grant_capability', 'revoke_capability', 'create_binding', 'update_binding', 'delete_binding', 'create_token', 'create_principal', 'delete_principal'))
	)`)
	require.NoError(t, err)
	require.NoError(t, s.Close())

	// Reopening runs migrations against the old-shaped table.
	s2, err := NewSQLiteStore(dbPath)
	require.NoError(t, err)
	defer func() { _ = s2.Close() }()

	err = s2.AppendAuditLog(context.Background(), &AuditEntry{
		ActorPrincipalID: "actor-1",
		Action:           AuditMintPairToken,
		TargetType:       "pair_token",
		TargetID:         "tok-1",
	})
	require.NoError(t, err, "migrated table must accept mint_pair_token")
}
```

If `audit_test.go` does not already import `path/filepath`, add it.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/store/ -run 'TestAuditLog' -v`
Expected: compile FAILURE — `undefined: AuditMintPairToken` etc.

- [ ] **Step 3: Add the constants and extend the schema + migration**

In `internal/store/audit.go`, extend the existing `const` block (match its no-per-line-comment style):

```go
	AuditMintPairToken    AuditAction = "mint_pair_token"
	AuditPairEnroll       AuditAction = "pair_enroll"
	AuditApproveLink      AuditAction = "approve_link"
```

Append the same three to `ValidAuditActions`:

```go
	AuditMintPairToken,
	AuditPairEnroll,
	AuditApproveLink,
```

In `internal/store/sqlite.go`, THREE coordinated edits — the CHECK list must be identical in both DDL sites:

1. **`schemaAuthSQL` audit_log DDL (~line 109):** extend the `CHECK (action IN (...))` list so it ends `..., 'create_principal', 'delete_principal', 'mint_pair_token', 'pair_enroll', 'approve_link')`.

2. **`recreateAuditLogTable` (~line 398):** the `CREATE TABLE audit_log_new` statement carries the same CHECK list — extend it identically: `..., 'create_principal', 'delete_principal', 'mint_pair_token', 'pair_enroll', 'approve_link')`.

3. **`needsAuditLogMigration` (~line 383):** the up-to-date probe must test for the NEW actions, otherwise existing databases never migrate. Replace the current contains-check:

```go
	// Check if constraint already includes the new actions
	if strings.Contains(tableSQL, "mint_pair_token") && strings.Contains(tableSQL, "pair_enroll") && strings.Contains(tableSQL, "approve_link") {
		return false
	}
	return true
```

Also update the log line in `migrateAuditLogCheckConstraint` from `"migrating audit_log check constraint to include create_principal and delete_principal"` to `"migrating audit_log check constraint to include pair actions"`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/store/ -run 'TestAuditLog' -v`
Expected: PASS, including the migration test.

- [ ] **Step 5: Run the full store package**

Run: `go test ./internal/store/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/store/audit.go internal/store/sqlite.go internal/store/audit_test.go
git commit -m "feat: audit actions for QR pairing and link approval"
```

---

### Task 3: Mint endpoint, QR rendering, and GRPCPort plumbing

**Files:**
- Create: `internal/webadmin/pair.go` (mint handler + token/host/URL helpers + `writePairError`)
- Create: `internal/webadmin/webadmin_pair_test.go` (mint tests + shared test helpers)
- Modify: `internal/webadmin/webadmin.go` (`Config.GRPCPort`, `PairTokenDuration`, `FullStore` additions, route registration)

> Deliberate deviation from the spec's "Files touched" sketch: the mint and
> pair handlers live in the new `pair.go`, not in `webadmin.go` (already
> >1200 lines). The spec's own store section creates a focused `pair.go`
> for the same reason; reviewers should not flag this.
- Modify: `internal/gateway/gateway.go` (compute and pass `GRPCPort`)
- Modify: `go.mod` / `go.sum` (`go get github.com/skip2/go-qrcode`)

**Interfaces:**
- Consumes: Task 1 store methods, Task 2 `AuditMintPairToken`, existing `validateCSRF(r)`, `getUserFromContext(r)`, `requireAuth`, `CSRFCookieName`, `withUser(ctx, user)` (test helper in `webadmin_coverage_test.go`), `createAdminUserWithPassword(t, a, username, plaintext)` (helpers_test.go), `auth.NewJWTVerifier`.
- Produces (Task 4 and 6 rely on these):
  - `const PairTokenDuration = 5 * time.Minute`
  - `func generatePairToken() (string, error)`
  - `func hashPairToken(token string) string`
  - `func writePairError(w http.ResponseWriter, status int, reason string)`
  - `Config.GRPCPort int`
  - Route `POST /api/admin/link/pair-token` (requireAuth) responding `{"url","qr","expires_at"}`
  - Test helper `newPairTestAdmin(t *testing.T, cfg Config) (*Admin, *store.SQLiteStore)`
  - Test helper `mintPairToken(t *testing.T, a *Admin, user *store.AdminUser) *httptest.ResponseRecorder`

- [ ] **Step 1: Add the QR dependency**

```bash
go get github.com/skip2/go-qrcode
```

Expected: go.mod gains `github.com/skip2/go-qrcode`. The API used below is `qrcode.Encode(content string, level qrcode.RecoveryLevel, size int) ([]byte, error)` with `qrcode.Medium` — if compilation later disagrees, read the package docs (`go doc github.com/skip2/go-qrcode Encode`) and adapt the one call site; everything else stands.

- [ ] **Step 2: Write the failing mint tests**

Create `internal/webadmin/webadmin_pair_test.go`:

```go
// ABOUTME: Tests for QR pairing: token minting (admin) and device enrollment.
// ABOUTME: Uses a real SQLite store; asserts contract responses and audit writes.

package webadmin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/2389/coven-gateway/internal/auth"
	"github.com/2389/coven-gateway/internal/store"
)

// newPairTestAdmin builds an Admin backed by a real store with principal
// support, returning the store for direct assertions.
func newPairTestAdmin(t *testing.T, cfg Config) (*Admin, *store.SQLiteStore) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	jwtVerifier := auth.NewJWTVerifier([]byte("test-secret-that-is-32-bytes-lon"))
	a := NewWithConfig(NewConfig{
		Store:          s,
		Config:         cfg,
		PrincipalStore: s,
		TokenGenerator: jwtVerifier,
	})
	return a, s
}

// mintPairToken performs an authenticated, CSRF-valid mint request.
func mintPairToken(t *testing.T, a *Admin, user *store.AdminUser) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/link/pair-token", nil)
	csrfVal := "test-csrf-token-value"
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrfVal})
	req.Header.Set("X-CSRF-Token", csrfVal)
	req = req.WithContext(withUser(req.Context(), user))
	rec := httptest.NewRecorder()
	a.handleMintPairToken(rec, req)
	return rec
}

type mintResponse struct {
	URL       string `json:"url"`
	QR        string `json:"qr"`
	ExpiresAt string `json:"expires_at"`
}

func decodeMintResponse(t *testing.T, rec *httptest.ResponseRecorder) mintResponse {
	t.Helper()
	var resp mintResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode mint response: %v (body: %s)", err, rec.Body.String())
	}
	return resp
}

func TestHandleMintPairToken_RequiresCSRF(t *testing.T) {
	a, _ := newPairTestAdmin(t, Config{BaseURL: "https://gw.example.ts.net"})
	user := createAdminUserWithPassword(t, a, "mintcsrf", "password123")

	req := httptest.NewRequest(http.MethodPost, "/api/admin/link/pair-token", nil)
	req = req.WithContext(withUser(req.Context(), user))
	rec := httptest.NewRecorder()
	a.handleMintPairToken(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 without CSRF, got %d", rec.Code)
	}
}

func TestHandleMintPairToken_RequiresUser(t *testing.T) {
	// The route sits behind requireAuth; this covers the handler's own guard.
	a, _ := newPairTestAdmin(t, Config{BaseURL: "https://gw.example.ts.net"})

	req := httptest.NewRequest(http.MethodPost, "/api/admin/link/pair-token", nil)
	csrfVal := "test-csrf-token-value"
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrfVal})
	req.Header.Set("X-CSRF-Token", csrfVal)
	rec := httptest.NewRecorder()
	a.handleMintPairToken(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without a user in context, got %d", rec.Code)
	}
}

func TestHandleMintPairToken_HappyPath(t *testing.T) {
	a, s := newPairTestAdmin(t, Config{BaseURL: "https://gw.example.ts.net", GRPCPort: 50051})
	user := createAdminUserWithPassword(t, a, "mintadmin", "password123")

	rec := mintPairToken(t, a, user)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeMintResponse(t, rec)

	u, err := url.Parse(resp.URL)
	if err != nil || u.Scheme != "coven" || u.Host != "pair" {
		t.Fatalf("expected coven://pair URL, got %q (err: %v)", resp.URL, err)
	}
	q := u.Query()
	if q.Get("v") != "1" {
		t.Errorf("expected v=1, got %q", q.Get("v"))
	}
	if q.Get("host") != "gw.example.ts.net" {
		t.Errorf("expected host=gw.example.ts.net, got %q", q.Get("host"))
	}
	if q.Get("port") != "" {
		t.Errorf("expected default port 50051 omitted from payload, got %q", q.Get("port"))
	}
	token := q.Get("token")
	if token == "" {
		t.Fatal("expected token in payload URL")
	}

	if !strings.HasPrefix(resp.QR, "data:image/png;base64,") {
		t.Errorf("expected PNG data URI, got %.40q", resp.QR)
	}

	expires, err := time.Parse(time.RFC3339, resp.ExpiresAt)
	if err != nil {
		t.Fatalf("expires_at not RFC3339: %v", err)
	}
	until := time.Until(expires)
	if until < 4*time.Minute || until > 6*time.Minute {
		t.Errorf("expected ~5 minute expiry, got %s", until)
	}

	pt, err := s.GetPairTokenByHash(context.Background(), hashPairToken(token))
	if err != nil {
		t.Fatalf("minted token hash not found in store: %v", err)
	}
	if pt.CreatedBy != user.ID {
		t.Errorf("expected created_by %q, got %q", user.ID, pt.CreatedBy)
	}

	entries, err := s.ListAuditLog(context.Background(), store.AuditFilter{})
	if err != nil {
		t.Fatalf("listing audit log: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Action == store.AuditMintPairToken && e.TargetID == pt.ID && e.ActorPrincipalID == user.ID {
			found = true
		}
	}
	if !found {
		t.Error("expected a mint_pair_token audit entry for the minted token")
	}
}

func TestHandleMintPairToken_IncludesNonDefaultPort(t *testing.T) {
	a, _ := newPairTestAdmin(t, Config{BaseURL: "https://gw.example.ts.net", GRPCPort: 9443})
	user := createAdminUserWithPassword(t, a, "portadmin", "password123")

	rec := mintPairToken(t, a, user)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeMintResponse(t, rec)

	u, err := url.Parse(resp.URL)
	if err != nil {
		t.Fatalf("parsing payload URL: %v", err)
	}
	if got := u.Query().Get("port"); got != "9443" {
		t.Errorf("expected port=9443 in payload, got %q", got)
	}
}

func TestHandleMintPairToken_RefusesIPLiteralHost(t *testing.T) {
	a, _ := newPairTestAdmin(t, Config{BaseURL: "https://100.64.1.5"})
	user := createAdminUserWithPassword(t, a, "ipadmin", "password123")

	rec := mintPairToken(t, a, user)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for IP-literal base URL, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "tailnet DNS name") {
		t.Errorf("expected remediation message, got %q", rec.Body.String())
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/webadmin/ -run 'TestHandleMintPairToken' -v`
Expected: compile FAILURE — `undefined: a.handleMintPairToken`, `undefined: hashPairToken`, `unknown field GRPCPort`.

- [ ] **Step 4: Extend Config, FullStore, and routes in webadmin.go**

In `internal/webadmin/webadmin.go`:

1. **`Config` struct (~line 63):** add `GRPCPort` between `BaseURL` and `TrustForwardedProto`:

```go
// Config holds admin UI configuration.
type Config struct {
	// BaseURL is the external URL for generating invite links
	BaseURL string
	// GRPCPort is the gRPC listener port embedded in QR pairing payloads.
	GRPCPort int
	// TrustForwardedProto mirrors config.ServerConfig.TrustForwardedProto for cookie Secure decisions.
	TrustForwardedProto bool
}
```

2. **`FullStore` interface:** after the `// Link codes` method block (ends with `DeleteExpiredLinkCodes` ~line 112), add:

```go
	// Pair tokens (QR pairing)
	CreatePairToken(ctx context.Context, tokenHash, createdBy string, expiresAt time.Time) (*store.PairToken, error)
	GetPairTokenByHash(ctx context.Context, tokenHash string) (*store.PairToken, error)
	ConsumePairToken(ctx context.Context, id string) error
	SetPairTokenPrincipal(ctx context.Context, id, principalID string) error
	DeleteExpiredPairTokens(ctx context.Context) error

	// Audit
	AppendAuditLog(ctx context.Context, entry *store.AuditEntry) error
```

(`*SQLiteStore` already implements all six — no store changes needed.)

3. **Route registration:** in the authenticated "Device linking UI" block (~line 291, beside `POST /admin/link/{id}/approve`), add:

```go
	mux.HandleFunc("POST /api/admin/link/pair-token", a.requireAuth(a.handleMintPairToken))
```

4. **Token TTL constant:** beside the existing `LinkCodeDuration = 10 * time.Minute` declaration, add (the spec pins this location):

```go
	// PairTokenDuration is how long a minted QR pair token remains valid.
	PairTokenDuration = 5 * time.Minute
```

- [ ] **Step 5: Implement the mint handler**

Create `internal/webadmin/pair.go`:

```go
// ABOUTME: QR pairing handlers: minting single-use pair tokens (admin+CSRF)
// ABOUTME: and enrolling devices that present one (POST /api/link/pair).

package webadmin

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/2389/coven-gateway/internal/store"
)

// generatePairToken returns a fresh 256-bit URL-safe bearer token.
func generatePairToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating pair token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// hashPairToken returns the hex SHA-256 digest stored in place of the token.
func hashPairToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// writePairError writes the pairing contract's JSON error envelope.
func writePairError(w http.ResponseWriter, status int, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": reason})
}

// pairHost extracts the QR payload hostname from the resolved BaseURL.
// IP literals are refused: TLS certificates cannot cover raw IPs, so an IP
// host in the QR would fail on every device.
func (a *Admin) pairHost() (string, error) {
	u, err := url.Parse(a.config.BaseURL)
	if err != nil || u.Hostname() == "" {
		return "", fmt.Errorf("cannot determine gateway hostname from base URL %q; set webadmin.base_url or COVEN_GATEWAY_URL to the tailnet DNS name", a.config.BaseURL)
	}
	host := u.Hostname()
	if net.ParseIP(host) != nil {
		return "", fmt.Errorf("gateway base URL %q is an IP literal; set webadmin.base_url or COVEN_GATEWAY_URL to the tailnet DNS name", a.config.BaseURL)
	}
	return host, nil
}

// pairPayloadURL builds the coven://pair deep link. Port 50051 is the client
// default and is omitted to keep the QR sparse.
func (a *Admin) pairPayloadURL(host, token string) string {
	payload := "coven://pair?v=1&host=" + url.QueryEscape(host)
	if a.config.GRPCPort != 0 && a.config.GRPCPort != 50051 {
		payload += "&port=" + strconv.Itoa(a.config.GRPCPort)
	}
	return payload + "&token=" + url.QueryEscape(token)
}

// handleMintPairToken mints a single-use pair token and returns the payload
// URL, a QR PNG data URI, and the expiry. The plaintext token exists only in
// this response — never in the database, logs, or audit detail.
func (a *Admin) handleMintPairToken(w http.ResponseWriter, r *http.Request) {
	if !a.validateCSRF(r) {
		http.Error(w, "Invalid request", http.StatusForbidden)
		return
	}
	user := getUserFromContext(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	host, err := a.pairHost()
	if err != nil {
		a.logger.Error("refusing to mint pair token", "error", err)
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	_ = a.store.DeleteExpiredPairTokens(r.Context())

	token, err := generatePairToken()
	if err != nil {
		a.logger.Error("generating pair token", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	pt, err := a.store.CreatePairToken(r.Context(), hashPairToken(token), user.ID, time.Now().Add(PairTokenDuration))
	if err != nil {
		a.logger.Error("storing pair token", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	payload := a.pairPayloadURL(host, token)
	png, err := qrcode.Encode(payload, qrcode.Medium, 256)
	if err != nil {
		a.logger.Error("rendering pair QR", "error", err, "pair_token_id", pt.ID)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	_ = a.store.AppendAuditLog(r.Context(), &store.AuditEntry{
		ActorPrincipalID: user.ID,
		Action:           store.AuditMintPairToken,
		TargetType:       "pair_token",
		TargetID:         pt.ID,
		Detail: map[string]any{
			"username":   user.Username,
			"expires_at": pt.ExpiresAt.Format(time.RFC3339),
		},
	})

	a.logger.Info("minted pair token", "pair_token_id", pt.ID, "by", user.Username)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"url":        payload,
		"qr":         "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
		"expires_at": pt.ExpiresAt.Format(time.RFC3339),
	})
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/webadmin/ -run 'TestHandleMintPairToken' -v`
Expected: all 5 PASS.

- [ ] **Step 7: Plumb GRPCPort through the gateway**

In `internal/gateway/gateway.go`, directly above the `webAdminCfg := webadmin.NewConfig{...}` literal (~line 369), compute the port. The tailscale/direct branch condition must match how this function already distinguishes modes — reuse the exact condition `determineWebAdminBaseURL`/the surrounding code uses for tailscale (e.g. `cfg.Tailscale.Enabled` or its equivalent field — read the file and use the same expression):

```go
	// gRPC port for QR pairing payloads: the tsnet listener is hardcoded to
	// :50051 in setupTailscaleListeners; direct mode parses Server.GRPCAddr.
	grpcPort := 50051
	if !cfg.Tailscale.Enabled {
		if _, portStr, err := net.SplitHostPort(cfg.Server.GRPCAddr); err == nil {
			if p, err := strconv.Atoi(portStr); err == nil {
				grpcPort = p
			}
		}
	}
```

Add `GRPCPort: grpcPort,` to the `Config: webadmin.Config{...}` literal, after `BaseURL`. Add `"net"`/`"strconv"` imports if missing.

- [ ] **Step 8: Verify the whole module compiles and tests pass**

Run: `go build ./... && go test ./internal/webadmin/ ./internal/gateway/`
Expected: PASS. If `cfg.Tailscale.Enabled` is not the real field name, the compiler points at it — fix using the condition the surrounding gateway code uses for tailscale mode.

- [ ] **Step 9: Commit**

```bash
git add go.mod go.sum internal/webadmin/pair.go internal/webadmin/webadmin.go internal/webadmin/webadmin_pair_test.go internal/gateway/gateway.go
git commit -m "feat: mint QR pair tokens from the admin UI"
```

---

### Task 4: Enrollment endpoint `POST /api/link/pair`

**Files:**
- Create: `internal/webadmin/pair_ratelimit.go`
- Modify: `internal/webadmin/pair.go` (add `remoteIP`, `handleLinkPair`)
- Modify: `internal/webadmin/webadmin.go` (shared enrollment helper refactor, route registration)
- Modify: `internal/webadmin/webadmin_pair_test.go` (enrollment tests)

**Interfaces:**
- Consumes: Task 1 store methods + `store.ErrPairTokenUsed` + `store.ErrNotFound`; Task 2 `store.AuditPairEnroll`; Task 3 `hashPairToken`, `writePairError`, `newPairTestAdmin`, `createAdminUserWithPassword`; existing `validFingerprint()` (link_test.go), `a.principalStore`, `a.logger`.
- Produces:
  - `func (a *Admin) getOrCreatePrincipalForDevice(ctx context.Context, fingerprint, deviceName string) (string, bool, error)` — replaces `getOrCreatePrincipalForLink`; the bool reports "created new principal". Task 5's approve flow calls it via `generateApprovalToken`.
  - `var pairLimiter *pairRateLimiter` (package-level, tests swap with save/restore)
  - `const maxPairFailuresPerWindow = 5`
  - Route `POST /api/link/pair` (unauthenticated)

- [ ] **Step 1: Write the failing enrollment tests**

Append to `internal/webadmin/webadmin_pair_test.go`:

```go
// pairRequest performs a POST /api/link/pair with the given JSON body.
func pairRequest(t *testing.T, a *Admin, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/link/pair", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	a.handleLinkPair(rec, req)
	return rec
}

// createStoredPairToken mints a token row directly, returning the plaintext.
func createStoredPairToken(t *testing.T, a *Admin, s *store.SQLiteStore, expiresAt time.Time) string {
	t.Helper()
	user := createAdminUserWithPassword(t, a, "tokadmin-"+strings.ReplaceAll(t.Name(), "/", "-"), "password123")
	token, err := generatePairToken()
	if err != nil {
		t.Fatalf("generating token: %v", err)
	}
	if _, err := s.CreatePairToken(context.Background(), hashPairToken(token), user.ID, expiresAt); err != nil {
		t.Fatalf("storing token: %v", err)
	}
	return token
}

func pairBody(token, fingerprint, deviceName string) string {
	b, _ := json.Marshal(map[string]string{
		"token":       token,
		"fingerprint": fingerprint,
		"device_name": deviceName,
	})
	return string(b)
}

func TestHandleLinkPair_HappyPath(t *testing.T) {
	a, s := newPairTestAdmin(t, Config{BaseURL: "https://gw.example.ts.net"})
	token := createStoredPairToken(t, a, s, time.Now().Add(5*time.Minute))
	fp := validFingerprint()

	rec := pairRequest(t, a, pairBody(token, fp, "Test iPhone"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		PrincipalID string `json:"principal_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if resp.PrincipalID == "" {
		t.Fatal("expected principal_id in response")
	}

	// The device is enrolled: approved principal with the presented pubkey.
	p, err := s.GetPrincipalByPubkey(context.Background(), fp)
	if err != nil {
		t.Fatalf("looking up enrolled principal: %v", err)
	}
	if p.ID != resp.PrincipalID {
		t.Errorf("principal mismatch: response %q, store %q", resp.PrincipalID, p.ID)
	}
	if p.Status != store.PrincipalStatusApproved {
		t.Errorf("expected approved principal, got %q", p.Status)
	}
	if p.DisplayName != "Test iPhone" {
		t.Errorf("expected display name from device_name, got %q", p.DisplayName)
	}

	hasRole, err := s.HasRole(context.Background(), store.RoleSubjectPrincipal, resp.PrincipalID, store.RoleMember)
	if err != nil {
		t.Fatalf("checking member role: %v", err)
	}
	if !hasRole {
		t.Error("expected the enrolled principal to have the member role")
	}

	// The token is burned and linked.
	pt, err := s.GetPairTokenByHash(context.Background(), hashPairToken(token))
	if err != nil {
		t.Fatalf("looking up token: %v", err)
	}
	if pt.UsedAt == nil {
		t.Error("expected token consumed")
	}
	if pt.PrincipalID == nil || *pt.PrincipalID != resp.PrincipalID {
		t.Error("expected token linked to the enrolled principal")
	}

	// The enrollment is audited.
	entries, err := s.ListAuditLog(context.Background(), store.AuditFilter{})
	if err != nil {
		t.Fatalf("listing audit log: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Action == store.AuditPairEnroll && e.TargetID == resp.PrincipalID {
			found = true
			if reused, ok := e.Detail["reused_principal"].(bool); !ok || reused {
				t.Errorf("expected reused_principal=false detail, got %v", e.Detail["reused_principal"])
			}
		}
	}
	if !found {
		t.Error("expected a pair_enroll audit entry")
	}
}

func TestHandleLinkPair_ReusesPrincipalForKnownFingerprint(t *testing.T) {
	a, s := newPairTestAdmin(t, Config{BaseURL: "https://gw.example.ts.net"})
	fp := validFingerprint()

	first := createStoredPairToken(t, a, s, time.Now().Add(5*time.Minute))
	rec := pairRequest(t, a, pairBody(first, fp, "Test iPhone"))
	if rec.Code != http.StatusOK {
		t.Fatalf("first pair: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var firstResp struct {
		PrincipalID string `json:"principal_id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &firstResp)

	second := createStoredPairToken(t, a, s, time.Now().Add(5*time.Minute))
	rec = pairRequest(t, a, pairBody(second, fp, "Test iPhone"))
	if rec.Code != http.StatusOK {
		t.Fatalf("second pair: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var secondResp struct {
		PrincipalID string `json:"principal_id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &secondResp)

	if firstResp.PrincipalID != secondResp.PrincipalID {
		t.Errorf("expected the same principal for the same fingerprint, got %q then %q",
			firstResp.PrincipalID, secondResp.PrincipalID)
	}
}

func TestHandleLinkPair_Rejections(t *testing.T) {
	fp := validFingerprint()

	cases := []struct {
		name       string
		body       func(t *testing.T, a *Admin, s *store.SQLiteStore) string
		wantReason string
	}{
		{
			name: "malformed JSON",
			body: func(t *testing.T, a *Admin, s *store.SQLiteStore) string {
				return `{not json`
			},
			wantReason: "invalid request: body",
		},
		{
			name: "missing token",
			body: func(t *testing.T, a *Admin, s *store.SQLiteStore) string {
				return pairBody("", fp, "Test iPhone")
			},
			wantReason: "invalid request: token",
		},
		{
			name: "bad fingerprint",
			body: func(t *testing.T, a *Admin, s *store.SQLiteStore) string {
				return pairBody("some-token", "too-short", "Test iPhone")
			},
			wantReason: "invalid request: fingerprint",
		},
		{
			name: "empty device name",
			body: func(t *testing.T, a *Admin, s *store.SQLiteStore) string {
				return pairBody("some-token", fp, "")
			},
			wantReason: "invalid request: device_name",
		},
		{
			name: "oversized device name",
			body: func(t *testing.T, a *Admin, s *store.SQLiteStore) string {
				return pairBody("some-token", fp, strings.Repeat("x", 101))
			},
			wantReason: "invalid request: device_name",
		},
		{
			name: "unknown token",
			body: func(t *testing.T, a *Admin, s *store.SQLiteStore) string {
				return pairBody("never-minted-token", fp, "Test iPhone")
			},
			wantReason: "invalid token",
		},
		{
			name: "expired token",
			body: func(t *testing.T, a *Admin, s *store.SQLiteStore) string {
				// Expired but present: skip eager cleanup by using a token
				// that handleLinkPair's DeleteExpiredPairTokens will remove —
				// removal yields "invalid token"; surviving yields "token
				// expired". Both are 401; assert only the status here.
				return pairBody(createStoredPairToken(t, a, s, time.Now().Add(-time.Minute)), fp, "Test iPhone")
			},
			wantReason: "", // reason depends on eager-cleanup timing; 401 is the contract
		},
		{
			name: "already used token",
			body: func(t *testing.T, a *Admin, s *store.SQLiteStore) string {
				token := createStoredPairToken(t, a, s, time.Now().Add(5*time.Minute))
				first := pairRequest(t, a, pairBody(token, fp, "Test iPhone"))
				if first.Code != http.StatusOK {
					t.Fatalf("setup pair failed: %d %s", first.Code, first.Body.String())
				}
				return pairBody(token, fp, "Test iPhone")
			},
			wantReason: "token already used",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, s := newPairTestAdmin(t, Config{BaseURL: "https://gw.example.ts.net"})
			rec := pairRequest(t, a, tc.body(t, a, s))
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
			}
			var resp struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("401 body must be the JSON error envelope, got %q", rec.Body.String())
			}
			if tc.wantReason != "" && resp.Error != tc.wantReason {
				t.Errorf("expected reason %q, got %q", tc.wantReason, resp.Error)
			}
		})
	}
}

func TestHandleLinkPair_RateLimited(t *testing.T) {
	a, _ := newPairTestAdmin(t, Config{BaseURL: "https://gw.example.ts.net"})

	saved := pairLimiter
	pairLimiter = newPairRateLimiter()
	defer func() { pairLimiter = saved }()

	for i := 0; i < maxPairFailuresPerWindow; i++ {
		pairLimiter.recordFailure("192.0.2.9")
	}

	req := httptest.NewRequest(http.MethodPost, "/api/link/pair", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.0.2.9:5555"
	rec := httptest.NewRecorder()
	a.handleLinkPair(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when rate limited, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "too many attempts") {
		t.Errorf("expected 'too many attempts', got %q", rec.Body.String())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/webadmin/ -run 'TestHandleLinkPair' -v`
Expected: compile FAILURE — `undefined: a.handleLinkPair`, `undefined: pairLimiter`, `undefined: newPairRateLimiter`.

- [ ] **Step 3: Implement the rate limiter**

Create `internal/webadmin/pair_ratelimit.go` — a deliberate mirror of `login_ratelimit.go` keyed by remote IP instead of username (keep the method bodies aligned with that file):

```go
// ABOUTME: In-process rate limiter for failed /api/link/pair attempts.
// ABOUTME: Sliding one-minute window per remote IP; mirrors login_ratelimit.go.

package webadmin

import (
	"sync"
	"time"
)

const (
	maxPairFailuresPerWindow = 5
	pairRateWindow           = time.Minute
	pairCleanupInterval      = 5 * time.Minute
)

// pairRateLimiter tracks recent failed pair attempts per remote IP.
// Defense-in-depth only: the real protection is 256-bit single-use tokens
// with a 5-minute TTL. Behind tailscale funnel the observed remote address
// may be a shared ingress address, coarsening the limit — acceptable.
// State is in-process and resets on restart, like loginRateLimiter.
type pairRateLimiter struct {
	mu          sync.Mutex
	failures    map[string][]time.Time
	lastCleanup time.Time
}

func newPairRateLimiter() *pairRateLimiter {
	return &pairRateLimiter{
		failures:    make(map[string][]time.Time),
		lastCleanup: time.Now(),
	}
}

// pairLimiter gates handleLinkPair. Package-level like loginLimiter;
// tests swap it with save/restore.
var pairLimiter = newPairRateLimiter()

// tooMany reports whether ip has exhausted its failure budget within the
// sliding window.
func (l *pairRateLimiter) tooMany(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-pairRateWindow)

	if now.Sub(l.lastCleanup) > pairCleanupInterval {
		l.cleanup(cutoff)
		l.lastCleanup = now
	}

	return len(l.validFailures(ip, cutoff)) >= maxPairFailuresPerWindow
}

// recordFailure notes a failed pair attempt for ip.
func (l *pairRateLimiter) recordFailure(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := time.Now().Add(-pairRateWindow)
	l.failures[ip] = append(l.validFailures(ip, cutoff), time.Now())
}

// validFailures returns ip's failures newer than cutoff and stores the
// pruned slice back (dropping the key entirely when empty).
// Callers must hold l.mu.
func (l *pairRateLimiter) validFailures(ip string, cutoff time.Time) []time.Time {
	timestamps := l.failures[ip]
	valid := make([]time.Time, 0, len(timestamps))
	for _, ts := range timestamps {
		if ts.After(cutoff) {
			valid = append(valid, ts)
		}
	}
	if len(valid) == 0 {
		delete(l.failures, ip)
	} else {
		l.failures[ip] = valid
	}
	return valid
}

// cleanup removes IPs whose failures have all expired.
// Callers must hold l.mu.
func (l *pairRateLimiter) cleanup(cutoff time.Time) {
	for ip, timestamps := range l.failures {
		live := false
		for _, ts := range timestamps {
			if ts.After(cutoff) {
				live = true
				break
			}
		}
		if !live {
			delete(l.failures, ip)
		}
	}
}
```

- [ ] **Step 4: Refactor the enrollment helper to be flow-agnostic**

In `internal/webadmin/webadmin.go`, REPLACE `getOrCreatePrincipalForLink` (its current body is reproduced below for reference) with `getOrCreatePrincipalForDevice`. The body is identical except: parameters `(fingerprint, deviceName string)` replace the `linkCode` field reads, and it returns a `created bool` as its second value:

```go
// getOrCreatePrincipalForDevice finds an existing principal by pubkey
// fingerprint or creates a new approved agent principal with a member role.
// Both the code-approval flow and QR pairing enroll through here. The second
// return reports whether a new principal was created.
func (a *Admin) getOrCreatePrincipalForDevice(ctx context.Context, fingerprint, deviceName string) (string, bool, error) {
	existing, err := a.principalStore.GetPrincipalByPubkey(ctx, fingerprint)
	if err == nil && existing != nil {
		a.logger.Info("using existing principal for link", "principal_id", existing.ID, "fingerprint", fingerprint)
		return existing.ID, false, nil
	}

	principalID := uuid.New().String()
	principal := &store.Principal{
		ID:          principalID,
		Type:        store.PrincipalTypeAgent,
		PubkeyFP:    fingerprint,
		DisplayName: deviceName,
		Status:      store.PrincipalStatusApproved,
		CreatedAt:   time.Now(),
	}

	if err := a.principalStore.CreatePrincipal(ctx, principal); err != nil {
		return "", false, err
	}

	if err := a.principalStore.AddRole(ctx, store.RoleSubjectPrincipal, principalID, store.RoleMember); err != nil {
		a.logger.Error("failed to add role", "error", err)
	}
	return principalID, true, nil
}
```

Update the one caller, `generateApprovalToken`, to:

```go
	principalID, _, err := a.getOrCreatePrincipalForDevice(ctx, linkCode.Fingerprint, linkCode.DeviceName)
```

Rename safety: run `grep -rn "getOrCreatePrincipalForLink" .` — the only remaining hits may be comments (e.g. in `webadmin_coverage_test.go`); update any comment to the new name. Zero code references must remain.

- [ ] **Step 5: Implement the enrollment handler and register the route**

Append to `internal/webadmin/pair.go` (add `"crypto/subtle"` and `"errors"` to its imports):

```go
// remoteIP extracts the client IP for rate limiting.
func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// handleLinkPair enrolls a device presenting a QR pair token.
// Contract (frozen — the client already shipped against it): 200
// {"principal_id"} on success; 401 {"error": reason} for EVERY rejection;
// 500 {"error":"internal error"} on store failure. The raw token is never
// logged; slog lines carry the token row ID once known.
func (a *Admin) handleLinkPair(w http.ResponseWriter, r *http.Request) {
	ip := remoteIP(r)
	if pairLimiter.tooMany(ip) {
		writePairError(w, http.StatusUnauthorized, "too many attempts")
		return
	}

	var req struct {
		Token       string `json:"token"`
		Fingerprint string `json:"fingerprint"`
		DeviceName  string `json:"device_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		pairLimiter.recordFailure(ip)
		writePairError(w, http.StatusUnauthorized, "invalid request: body")
		return
	}
	if req.Token == "" {
		pairLimiter.recordFailure(ip)
		writePairError(w, http.StatusUnauthorized, "invalid request: token")
		return
	}
	if len(req.Fingerprint) != 64 {
		pairLimiter.recordFailure(ip)
		writePairError(w, http.StatusUnauthorized, "invalid request: fingerprint")
		return
	}
	if req.DeviceName == "" || len(req.DeviceName) > 100 {
		pairLimiter.recordFailure(ip)
		writePairError(w, http.StatusUnauthorized, "invalid request: device_name")
		return
	}

	_ = a.store.DeleteExpiredPairTokens(r.Context())

	hash := hashPairToken(req.Token)
	pt, err := a.store.GetPairTokenByHash(r.Context(), hash)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			pairLimiter.recordFailure(ip)
			writePairError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		a.logger.Error("looking up pair token", "error", err)
		writePairError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// Belt and braces on top of the indexed hash lookup: a found row already
	// implies equality, but the explicit compare keeps the path constant-time.
	if subtle.ConstantTimeCompare([]byte(pt.TokenHash), []byte(hash)) != 1 {
		pairLimiter.recordFailure(ip)
		writePairError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	if pt.UsedAt != nil {
		pairLimiter.recordFailure(ip)
		writePairError(w, http.StatusUnauthorized, "token already used")
		return
	}
	if time.Now().After(pt.ExpiresAt) {
		pairLimiter.recordFailure(ip)
		writePairError(w, http.StatusUnauthorized, "token expired")
		return
	}

	if a.principalStore == nil {
		a.logger.Error("server not configured for pair enrollment")
		writePairError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// The claim: exactly one concurrent request per token gets past here.
	if err := a.store.ConsumePairToken(r.Context(), pt.ID); err != nil {
		if errors.Is(err, store.ErrPairTokenUsed) {
			pairLimiter.recordFailure(ip)
			writePairError(w, http.StatusUnauthorized, "token already used")
			return
		}
		a.logger.Error("consuming pair token", "error", err, "pair_token_id", pt.ID)
		writePairError(w, http.StatusInternalServerError, "internal error")
		return
	}

	principalID, created, err := a.getOrCreatePrincipalForDevice(r.Context(), req.Fingerprint, req.DeviceName)
	if err != nil {
		// Fail closed: the token stays burned; the admin mints another.
		a.logger.Error("pair enrollment failed after claim", "error", err, "pair_token_id", pt.ID)
		writePairError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := a.store.SetPairTokenPrincipal(r.Context(), pt.ID, principalID); err != nil {
		a.logger.Error("recording pair token principal", "error", err, "pair_token_id", pt.ID)
	}

	_ = a.store.AppendAuditLog(r.Context(), &store.AuditEntry{
		ActorPrincipalID: principalID,
		Action:           store.AuditPairEnroll,
		TargetType:       "principal",
		TargetID:         principalID,
		Detail: map[string]any{
			"device_name":      req.DeviceName,
			"pair_token_id":    pt.ID,
			"reused_principal": !created,
		},
	})

	a.logger.Info("device paired", "device_name", req.DeviceName, "principal_id", principalID, "pair_token_id", pt.ID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"principal_id": principalID})
}
```

In `internal/webadmin/webadmin.go`, in `registerRootRoutes`'s "Device linking API (unauthenticated for devices)" block (~line 267), add:

```go
	mux.HandleFunc("POST /api/link/pair", a.handleLinkPair)
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/webadmin/ -run 'TestHandleLinkPair' -v`
Expected: all PASS (happy path, reuse, 8 rejection subtests, rate limit).

- [ ] **Step 7: Run the full webadmin package (the refactor touches the approve flow)**

Run: `go test ./internal/webadmin/ && go test -race ./internal/webadmin/ -run 'TestHandleLinkPair|TestHandleMintPairToken'`
Expected: PASS — including all pre-existing link tests against the refactored helper.

- [ ] **Step 8: Commit**

```bash
git add internal/webadmin/pair.go internal/webadmin/pair_ratelimit.go internal/webadmin/webadmin.go internal/webadmin/webadmin_pair_test.go
git commit -m "feat: device enrollment via QR pair token"
```

---

### Task 5: Audit the existing link approval

**Files:**
- Modify: `internal/webadmin/webadmin.go` (`handleLinkApprove`)
- Modify: `internal/webadmin/webadmin_pair_test.go` (one test)

**Interfaces:**
- Consumes: Task 2 `store.AuditApproveLink`; Task 3 `newPairTestAdmin`; existing helpers `createAdminUserWithPassword`, `createPendingLink(t, a, fingerprint, deviceName) string` (webadmin_coverage_test.go), `validFingerprint()`, `withUser`, `CSRFCookieName`.
- Produces: `handleLinkApprove` writes an `approve_link` audit entry — closing the pre-existing gap where approvals created principals and 30-day JWTs with no audit trail.

- [ ] **Step 1: Write the failing test**

Append to `internal/webadmin/webadmin_pair_test.go`:

```go
func TestHandleLinkApprove_WritesAuditEntry(t *testing.T) {
	a, s := newPairTestAdmin(t, Config{BaseURL: "https://gw.example.ts.net"})
	user := createAdminUserWithPassword(t, a, "approveaudit", "password123")
	linkID := createPendingLink(t, a, validFingerprint(), "Audited Device")

	req := httptest.NewRequest(http.MethodPost, "/admin/link/"+linkID+"/approve", nil)
	csrfVal := "test-csrf-token-value"
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrfVal})
	req.Header.Set("X-CSRF-Token", csrfVal)
	req.SetPathValue("id", linkID)
	req = req.WithContext(withUser(req.Context(), user))
	rec := httptest.NewRecorder()
	a.handleLinkApprove(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	entries, err := s.ListAuditLog(context.Background(), store.AuditFilter{})
	if err != nil {
		t.Fatalf("listing audit log: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Action == store.AuditApproveLink && e.ActorPrincipalID == user.ID {
			found = true
			if e.Detail["device_name"] != "Audited Device" {
				t.Errorf("expected device_name detail, got %v", e.Detail["device_name"])
			}
			if e.Detail["link_code_id"] != linkID {
				t.Errorf("expected link_code_id detail %q, got %v", linkID, e.Detail["link_code_id"])
			}
		}
	}
	if !found {
		t.Error("expected an approve_link audit entry")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/webadmin/ -run 'TestHandleLinkApprove_WritesAuditEntry' -v`
Expected: FAIL — "expected an approve_link audit entry" (the handler exists; the audit call doesn't).

- [ ] **Step 3: Add the audit call**

In `internal/webadmin/webadmin.go`, in `handleLinkApprove`, after the successful `a.store.ApproveLinkCode(...)` call and before the existing `a.logger.Info("link code approved", ...)` line, insert:

```go
	_ = a.store.AppendAuditLog(r.Context(), &store.AuditEntry{
		ActorPrincipalID: user.ID,
		Action:           store.AuditApproveLink,
		TargetType:       "principal",
		TargetID:         principalID,
		Detail: map[string]any{
			"device_name":  linkCode.DeviceName,
			"link_code_id": id,
		},
	})
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/webadmin/ -run 'TestHandleLinkApprove' -v`
Expected: PASS, including pre-existing approve tests.

- [ ] **Step 5: Commit**

```bash
git add internal/webadmin/webadmin.go internal/webadmin/webadmin_pair_test.go
git commit -m "feat: audit link approvals (closes pre-existing audit gap)"
```

---

### Task 6: Admin UI — QR section on LinkPage

**Files:**
- Modify: `web/src/lib/components/LinkPage.svelte`
- Create: `web/src/lib/components/LinkPage.test.ts`

**Interfaces:**
- Consumes: Task 3's `POST /api/admin/link/pair-token` returning `{"url","qr","expires_at"}` (200), plain-text 409 for IP-literal hosts; `validateCSRF` accepts the `X-CSRF-Token` header; existing `csrfToken` prop; `Card` component and the file's class vocabulary.
- Produces: a "Pair by QR code" Card between the pending-requests Card and the "How-to Section" Card. No props changes, no new island, no new npm deps.

- [ ] **Step 0: Read the frontend runbook**

Read `docs/plans/frontend-redesign/RUNBOOK.md` and follow its session rules for all steps in this task.

- [ ] **Step 1: Write the failing component tests**

Create `web/src/lib/components/LinkPage.test.ts`:

```ts
// ABOUTME: Tests for LinkPage — QR pair-token minting section.
// ABOUTME: Mocks fetch; asserts mint button, QR/URL display, CSRF header, errors.
import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import LinkPage from './LinkPage.svelte';

afterEach(() => {
  vi.unstubAllGlobals();
});

const mintOK = {
  ok: true,
  json: async () => ({
    url: 'coven://pair?v=1&host=gw.example.ts.net&token=abc123',
    qr: 'data:image/png;base64,AAAA',
    expires_at: new Date(Date.now() + 5 * 60 * 1000).toISOString(),
  }),
};

describe('LinkPage QR pairing', () => {
  it('renders the mint button', () => {
    render(LinkPage, { props: { codes: [], csrfToken: 't' } });
    expect(screen.getByText('Generate QR code')).toBeTruthy();
  });

  it('shows the QR image and pair link after minting', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mintOK));
    render(LinkPage, { props: { codes: [], csrfToken: 't' } });

    await fireEvent.click(screen.getByText('Generate QR code'));

    await waitFor(() => {
      expect(screen.getByAltText('Pairing QR code')).toBeTruthy();
      expect(screen.getByText('coven://pair?v=1&host=gw.example.ts.net&token=abc123')).toBeTruthy();
      expect(screen.getByText(/Expires in/)).toBeTruthy();
    });
  });

  it('sends the CSRF token header when minting', async () => {
    const fetchMock = vi.fn().mockResolvedValue(mintOK);
    vi.stubGlobal('fetch', fetchMock);
    render(LinkPage, { props: { codes: [], csrfToken: 'csrf-abc' } });

    await fireEvent.click(screen.getByText('Generate QR code'));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/admin/link/pair-token',
        expect.objectContaining({
          method: 'POST',
          headers: { 'X-CSRF-Token': 'csrf-abc' },
        })
      );
    });
  });

  it('shows an error when minting fails', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: false, status: 500, text: async () => 'boom' })
    );
    render(LinkPage, { props: { codes: [], csrfToken: 't' } });

    await fireEvent.click(screen.getByText('Generate QR code'));

    await waitFor(() => {
      expect(screen.getByText('Failed to create pairing code')).toBeTruthy();
    });
  });

  it('surfaces the 409 remediation message from the server', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 409,
        text: async () => 'gateway base URL is an IP literal; set webadmin.base_url',
      })
    );
    render(LinkPage, { props: { codes: [], csrfToken: 't' } });

    await fireEvent.click(screen.getByText('Generate QR code'));

    await waitFor(() => {
      expect(
        screen.getByText('gateway base URL is an IP literal; set webadmin.base_url')
      ).toBeTruthy();
    });
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd web && npm test -- LinkPage`
Expected: FAIL — "Generate QR code" not found.

- [ ] **Step 3: Implement the QR section**

In `web/src/lib/components/LinkPage.svelte`:

**Script additions** (after the existing `approve`/`refresh` functions, before `</script>`):

```ts
  interface MintResponse {
    url: string;
    qr: string;
    expires_at: string;
  }

  let minting = $state(false);
  let mintError = $state('');
  let pairURL = $state('');
  let pairQR = $state('');
  let pairExpiresAt = $state<Date | null>(null);
  let remaining = $state(0);
  let ticker: ReturnType<typeof setInterval> | undefined;

  $effect(() => {
    return () => {
      if (ticker) clearInterval(ticker);
    };
  });

  function stopTicker() {
    if (ticker) {
      clearInterval(ticker);
      ticker = undefined;
    }
  }

  function tick() {
    if (!pairExpiresAt) return;
    remaining = Math.max(0, Math.floor((pairExpiresAt.getTime() - Date.now()) / 1000));
    if (remaining <= 0) stopTicker();
  }

  function formatRemaining(s: number): string {
    const m = Math.floor(s / 60);
    const sec = s % 60;
    return `${m}:${String(sec).padStart(2, '0')}`;
  }

  async function mintPairToken() {
    minting = true;
    mintError = '';
    try {
      const res = await fetch('/api/admin/link/pair-token', {
        method: 'POST',
        headers: { 'X-CSRF-Token': csrfToken },
      });
      if (!res.ok) {
        mintError = res.status === 409 ? await res.text() : 'Failed to create pairing code';
        return;
      }
      const data: MintResponse = await res.json();
      pairURL = data.url;
      pairQR = data.qr;
      pairExpiresAt = new Date(data.expires_at);
      stopTicker();
      tick();
      ticker = setInterval(tick, 1000);
    } catch {
      mintError = 'Failed to create pairing code';
    } finally {
      minting = false;
    }
  }
```

**Markup:** insert a new Card between the pending-requests `</Card>` and the `<!-- How-to Section -->` comment, using the file's existing Card/snippet idiom and class vocabulary:

```svelte
  <!-- QR Pairing Section -->
  <Card>
    {#snippet children()}
      <div class="p-6">
        <h3 class="font-[var(--typography-fontWeight-semibold)] text-fg mb-3">Pair by QR code</h3>
        <p class="text-[length:var(--typography-fontSize-sm)] text-fgMuted mb-4">
          Generate a single-use code, then scan it with the Coven app (or paste the link on macOS).
          It expires after 5 minutes.
        </p>
        {#if mintError}
          <p class="text-[length:var(--typography-fontSize-sm)] text-danger mb-4" role="alert">{mintError}</p>
        {/if}
        {#if pairQR && remaining > 0}
          <div class="flex flex-col items-start gap-3">
            <img src={pairQR} alt="Pairing QR code" width="256" height="256" class="rounded-[var(--border-radius-md)] bg-white p-2" />
            <CodeText class="text-[length:var(--typography-fontSize-xs)] break-all">
              {#snippet children()}{pairURL}{/snippet}
            </CodeText>
            <span class="text-[length:var(--typography-fontSize-sm)] text-fgMuted">
              Expires in {formatRemaining(remaining)}
            </span>
          </div>
        {:else}
          {#if pairExpiresAt && remaining <= 0}
            <p class="text-[length:var(--typography-fontSize-sm)] text-fgMuted mb-4">
              This code has expired — generate another.
            </p>
          {/if}
          <button
            type="button"
            onclick={mintPairToken}
            disabled={minting}
            class="px-3 py-1.5 bg-[var(--color-primary)] text-[var(--color-primaryFg)] text-[length:var(--typography-fontSize-sm)] font-[var(--typography-fontWeight-medium)] rounded-[var(--border-radius-md)] hover:opacity-90 transition-opacity disabled:opacity-50"
          >
            {minting ? 'Generating...' : 'Generate QR code'}
          </button>
        {/if}
      </div>
    {/snippet}
  </Card>
```

If the file's design tokens name the error color differently than `text-danger`, use whatever the file or its siblings already use for error text.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd web && npm test -- LinkPage`
Expected: all 5 PASS.

- [ ] **Step 5: Type-check and run the whole web suite**

Run: `cd web && npm run check && npm test`
Expected: PASS, zero new svelte-check errors or warnings.

- [ ] **Step 6: Commit**

```bash
git add web/src/lib/components/LinkPage.svelte web/src/lib/components/LinkPage.test.ts
git commit -m "feat: mint and display QR pairing codes on the link page"
```

---

### Task 7: Full-suite verification

**Files:** none created — this task is the release gate for the branch.

**Interfaces:**
- Consumes: everything above.
- Produces: a branch that passes every canonical check, ready for review/PR.

- [ ] **Step 1: Go suite**

Run: `go test ./...`
Expected: PASS across all packages.

- [ ] **Step 2: Race detector**

Run: `go test -race ./...`
Expected: PASS.

- [ ] **Step 3: Lint**

Run: `golangci-lint run`
Expected: clean — zero findings introduced by this branch. Fix anything it flags in the new code.

- [ ] **Step 4: Build (includes the embedded web assets path)**

Run: `make build`
Expected: builds cleanly. If the Makefile's build depends on regenerated `web/` dist output, follow the RUNBOOK's build steps first.

- [ ] **Step 5: Web suite**

Run: `cd web && npm run check && npm test`
Expected: PASS.

- [ ] **Step 6: Contract self-check against the spec's error table**

Re-read the spec's "Error handling" table and confirm by inspection of `handleLinkPair`/`handleMintPairToken`:
- every enroll rejection is `401` with the JSON envelope and one of the five exact reason strings,
- store failures are `500 {"error":"internal error"}`,
- mint IP-literal refusal is `409` plain text,
- the plaintext token appears nowhere in logs, audit detail, or the database (grep the diff: `git diff main -- internal | grep -n 'token'` and verify every logged value is a row ID or hash, never the plaintext).

- [ ] **Step 7: Commit anything the gate shook out, or confirm clean**

```bash
git status
```

Expected: clean tree; branch `feat/qr-pairing-gateway` ready for review. Merging is Doctor Biz's call.
