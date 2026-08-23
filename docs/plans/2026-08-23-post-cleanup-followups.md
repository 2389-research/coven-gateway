# Post-Cleanup Follow-ups Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the four follow-ups from PR #117's final review and Harper's 2026-08-23 selection: fix the nil-JWTVerifier panic, give fake-agent Bearer-token auth, bump vitest past its open advisory, and consolidate the four hand-rolled `formatTime` copies.

**Architecture:** Three independent Go fixes/features (auth hardening + gateway wiring, fake-agent flag) and two web changes (dependency bump, shared formatter). Each task is self-contained with its own tests and commit(s); they share only the branch.

**Tech Stack:** Go 1.22+ (stdlib `testing`, no testify), Svelte 5 + TypeScript + Vitest, npm exact pins.

**Spec:** None — requirements are fixed by the PR #117 final review (`docs/plans/2026-08-22-frontend-toolchain-cleanup.md` context), the investigation of 2026-08-23, and Harper's explicit selection of these four items. This plan is the authority; exact code below is binding.

## Global Constraints

- Branch: `chore/post-cleanup-followups` (already created from main at 430351f). Conventional commits, imperative present tense. NEVER bypass hooks — `--no-verify`, `--no-hooks`, `--no-pre-commit-hook` are forbidden. Pre-commit hooks run go fmt/vet/test + tidy + hygiene (~1 min); let them.
- Go commands: plain `go` may fail from a GOROOT mismatch on this machine — if it does, use `mise exec -- go <args>`. Verification: `go test ./...` green, `golangci-lint run` clean.
- `internal/auth` is security-critical (60% min coverage, CI-enforced). New exported behavior there MUST have tests covering success AND error paths.
- Web: `web/package.json` uses exact pins. NO dependency changes anywhere except Task 3's single vitest line. Web gates: `npm run check` (0 errors 0 warnings), `npm test`, `make web` (from repo root).
- Zero new warnings in any gate output. Never stage `proto/coven-proto` or `coven-gateway.db.bak` (pre-existing dirt). Never `git add -A` — always explicit paths.
- New hand-written source files start with a 2-line `ABOUTME:` comment header.
- Do not touch `docs/plans/frontend-redesign/**`.

---

### Task 1: Fix the nil-JWTVerifier panic (typed-nil TokenGenerator)

**Root cause:** With `auth.allow_anonymous: true` and empty `jwt_secret`, `createGRPCServer` (internal/gateway/gateway.go:191–196) returns `grpcServerResult{jwtVerifier: nil}`. Line 381 assigns that nil `*auth.JWTVerifier` POINTER into the `webadmin.TokenGenerator` INTERFACE field — a typed nil, so the interface itself is non-nil. webadmin's guard `if a.tokenGenerator == nil` (webadmin.go:696) therefore passes, `a.tokenGenerator.Generate(...)` (webadmin.go:727) runs on a nil receiver, and `token.SignedString(v.secret)` panics at internal/auth/token.go:90. Observed in Task 13's playwright gateway logs (POST /setup with "Create API principal" checked).

**Fix (both layers):** (1) nil-receiver guards on `Verify` and `Generate` in internal/auth — both dereference `v.secret`, same failure class, and this is the security package so it must not be panickable by wiring mistakes; (2) the gateway wiring keeps a true nil in the interface so webadmin's existing guard works as its author intended.

**Files:**
- Modify: `internal/auth/token.go`
- Modify: `internal/auth/token_test.go`
- Modify: `internal/gateway/gateway.go:370–383`
- Modify: `internal/webadmin/webadmin_admin_test.go` (createOwnerPrincipal section, ~line 624)

**Interfaces:**
- Produces: `auth.ErrNilVerifier` sentinel (exported). No signature changes anywhere.

- [ ] **Step 1: Write the failing tests** in `internal/auth/token_test.go` (append; match the file's stdlib style — `testSecret` and `mustNewJWTVerifier` already exist there):

```go
func TestGenerateOnNilVerifier(t *testing.T) {
	var v *JWTVerifier
	_, err := v.Generate("principal-1", time.Hour)
	if !errors.Is(err, ErrNilVerifier) {
		t.Fatalf("expected ErrNilVerifier, got %v", err)
	}
}

func TestVerifyOnNilVerifier(t *testing.T) {
	var v *JWTVerifier
	_, err := v.Verify("some-token")
	if !errors.Is(err, ErrNilVerifier) {
		t.Fatalf("expected ErrNilVerifier, got %v", err)
	}
}
```

Add `"errors"` to the test file's imports if not present.

- [ ] **Step 2: Run and verify they fail by panicking**

Run: `go test -run 'OnNilVerifier' ./internal/auth/`
Expected: FAIL — runtime panic (invalid memory address / nil pointer dereference). That panic IS the bug reproduction.

- [ ] **Step 3: Add the sentinel and guards** in `internal/auth/token.go`. In the `var (...)` error block add:

```go
	// ErrNilVerifier indicates a token operation on a nil JWTVerifier — the
	// gateway is running without JWT auth, so no token can be issued or checked.
	ErrNilVerifier = errors.New("jwt verifier is not configured")
```

First lines of `Verify`:

```go
func (v *JWTVerifier) Verify(tokenString string) (principalID string, err error) {
	if v == nil {
		return "", ErrNilVerifier
	}
```

First lines of `Generate`:

```go
func (v *JWTVerifier) Generate(principalID string, expiresIn time.Duration) (string, error) {
	if v == nil {
		return "", ErrNilVerifier
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/auth/`
Expected: PASS, including the two new tests.

- [ ] **Step 5: Fix the wiring** in `internal/gateway/gateway.go`. Replace lines 370–383 (the `webAdminCfg := webadmin.NewConfig{...}` literal ending in `gw.webAdmin = webadmin.NewWithConfig(webAdminCfg)`) with:

```go
	webAdminCfg := webadmin.NewConfig{
		Store:        sqlStore,
		Manager:      gw.agentManager,
		Conversation: convService,
		Broadcaster:  eventBroadcaster,
		Registry:     packRegistry,
		Config: webadmin.Config{
			BaseURL:             webAdminBaseURL,
			TrustForwardedProto: cfg.Server.TrustForwardedProto,
		},
		PrincipalStore: sqlStore,
	}
	// Assign only a non-nil verifier: a typed-nil *JWTVerifier in the interface
	// field would defeat webadmin's `tokenGenerator == nil` guard and panic.
	if grpcResult.jwtVerifier != nil {
		webAdminCfg.TokenGenerator = grpcResult.jwtVerifier
	}
	gw.webAdmin = webadmin.NewWithConfig(webAdminCfg)
```

- [ ] **Step 6: Add the regression test at the observed crash site.** In `internal/webadmin/webadmin_admin_test.go`, in the `--- createOwnerPrincipal ---` section (~line 624): read the section's existing fixtures first and reuse them. The test must construct an Admin whose PrincipalStore succeeds (so execution reaches the Generate call) and whose TokenGenerator is a typed-nil `*auth.JWTVerifier`. Prefer the section's existing store fixture; ONLY if none reaches the Generate call, add this minimal in-file mock (matching neighboring mock style):

```go
type okPrincipalStore struct{}

func (okPrincipalStore) CreatePrincipal(ctx context.Context, p *store.Principal) error { return nil }
func (okPrincipalStore) GetPrincipalByPubkey(ctx context.Context, fp string) (*store.Principal, error) {
	return nil, nil
}
func (okPrincipalStore) AddRole(ctx context.Context, subjectType store.RoleSubjectType, subjectID string, role store.RoleName) error {
	return nil
}
```

The test (import `"github.com/2389/coven-gateway/internal/auth"` — test-only import):

```go
func TestCreateOwnerPrincipal_TypedNilTokenGenerator(t *testing.T) {
	// Regression: gateway wiring assigned a nil *auth.JWTVerifier into the
	// TokenGenerator interface field; the non-nil interface defeated the nil
	// guard and Generate panicked on a nil receiver (token.go:90).
	var nilVerifier *auth.JWTVerifier
	a := NewWithConfig(NewConfig{
		PrincipalStore: okPrincipalStore{},
		TokenGenerator: nilVerifier,
	})
	got := a.createOwnerPrincipal(context.Background(), "typed-nil-user")
	if got != "" {
		t.Fatalf("expected empty token from unconfigured generator, got %q", got)
	}
}
```

(With the Step 3 guard, Generate returns `ErrNilVerifier`, createOwnerPrincipal's existing error path at webadmin.go:728–731 logs it and returns `""` — no panic. Before Step 3 this test would panic; you may temporarily verify that with `git stash` of token.go if quick, but it is not required.)

- [ ] **Step 7: Full verification**

Run: `go test ./...` and `golangci-lint run`
Expected: all green, no new warnings.

- [ ] **Step 8: Commit**

```bash
git add internal/auth/token.go internal/auth/token_test.go internal/gateway/gateway.go internal/webadmin/webadmin_admin_test.go
git commit -m "fix: guard nil JWTVerifier and keep typed nil out of TokenGenerator"
```

---

### Task 2: fake-agent `-token` flag (Bearer auth)

`cmd/fake-agent` currently sends zero auth metadata, so it only works against `allow_anonymous` gateways. The gateway's JWT path (internal/auth/interceptor.go:248–258) reads gRPC metadata key `authorization` with value `Bearer <token>`. Add an optional `-token` flag that attaches exactly that.

**Files:**
- Modify: `cmd/fake-agent/main.go`
- Create: `cmd/fake-agent/main_test.go`
- Modify: `gotchas.md` (line 7 — the "fake-agent has no auth" entry is stale after this task)

**Interfaces:**
- Consumes: gateway expects metadata `authorization: Bearer <jwt>` (interceptor.go `authenticateWithJWT`). Tokens are minted via `coven-admin token create --principal <id>` or the bootstrap/setup flows.
- Produces: `withBearer(ctx context.Context, token string) context.Context` helper (unexported).

- [ ] **Step 1: Write the failing test** — create `cmd/fake-agent/main_test.go`:

```go
// ABOUTME: Tests for fake-agent's auth metadata helper.
// ABOUTME: Verifies -token attaches a Bearer authorization header and empty token attaches nothing.

package main

import (
	"context"
	"testing"

	"google.golang.org/grpc/metadata"
)

func TestWithBearerAppendsAuthorization(t *testing.T) {
	ctx := withBearer(context.Background(), "tok123")
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("expected outgoing metadata")
	}
	got := md.Get("authorization")
	if len(got) != 1 || got[0] != "Bearer tok123" {
		t.Fatalf("authorization = %v, want [Bearer tok123]", got)
	}
}

func TestWithBearerEmptyTokenAddsNoMetadata(t *testing.T) {
	ctx := withBearer(context.Background(), "")
	if _, ok := metadata.FromOutgoingContext(ctx); ok {
		t.Fatal("expected no outgoing metadata for empty token")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./cmd/fake-agent/`
Expected: FAIL — `undefined: withBearer`.

- [ ] **Step 3: Implement** in `cmd/fake-agent/main.go`. Add the flag beside the existing three, thread it through `run`, and add the helper. Current code (main.go:23–48) has `flag.Parse()` then `run(*addr, *name, *agentID)`. Changes:

```go
	token := flag.String("token", "", "Bearer token for gateway auth (omit for allow_anonymous gateways)")
```

`run` signature becomes `func run(addr, name, agentID, token string) error`; the call becomes `run(*addr, *name, *agentID, *token)`. Inside `run`, immediately before `client.AgentStream(ctx)`:

```go
	ctx = withBearer(ctx, token)
```

And the helper (imports gain `"google.golang.org/grpc/metadata"`):

```go
// withBearer returns ctx carrying gRPC authorization metadata when token is non-empty.
// The gateway's JWT interceptor reads metadata key "authorization" as "Bearer <token>".
func withBearer(ctx context.Context, token string) context.Context {
	if token == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
}
```

- [ ] **Step 4: Run tests + build**

Run: `go test ./cmd/fake-agent/ && go build ./cmd/fake-agent/`
Expected: PASS, clean build. (Use `mise exec -- go` if plain go hits the GOROOT mismatch.)

- [ ] **Step 5: Update gotchas.md line 7.** Replace the sentence `fake-agent has no auth (no SSH/JWT).` at the start of that entry with: `fake-agent sends no auth by default; pass -token <jwt> to authenticate against an auth-enforcing gateway (mint one with coven-admin token create).` Keep the rest of the entry (config-dependent e2e shape, mise build note, panic warning) — but update the panic warning's tail from `— pre-existing.` to `— fixed 2026-08-23 (nil-verifier guard).` ONLY if Task 1 is already committed on this branch; otherwise leave it.

- [ ] **Step 6: Full verification**

Run: `go test ./...` and `golangci-lint run`
Expected: green, no new warnings.

- [ ] **Step 7: Commit**

```bash
git add cmd/fake-agent/main.go cmd/fake-agent/main_test.go gotchas.md
git commit -m "feat: add -token flag to fake-agent for Bearer auth"
```

---

### Task 3: vitest 4.0.18 → 4.1.11 (policy bump)

Closes the open advisory GHSA-5xrq-8626-4rwp (affects 4.0.18; fixed in 4.1.x). Peer-dep analysis done 2026-08-23: vitest@4.1.11 wants `vite ^6||^7||^8` (have 6.4.1 ✓), `jsdom *` (have 28.1.0 ✓), `@types/node ^20||^22||>=24` (have 22.20.1 ✓); `@testing-library/svelte@5.3.1` declares `vitest: *` ✓. No `@vitest/*` companion packages exist in package.json. Clean.

**Files:**
- Modify: `web/package.json` (one line), `web/package-lock.json` (regenerated)

- [ ] **Step 1: Bump** (from `web/`):

```bash
npm install --save-exact vitest@4.1.11
```

- [ ] **Step 2: Verify the diff is exactly the bump**

Run: `git diff web/package.json`
Expected: only the vitest line changes, `"4.0.18"` → `"4.1.11"`, exact pin (no `^`). If npm touched anything else in package.json, stop and investigate — do not commit collateral changes.

- [ ] **Step 3: Full web gate**

Run (from `web/`): `npm run check` then `npm test`; then from repo root: `make web`
Expected: check 0 errors 0 warnings; all 143 tests pass; build clean. Zero new warnings in vitest output.

- [ ] **Step 4: Commit**

```bash
git add web/package.json web/package-lock.json
git commit -m "chore(deps): bump vitest to 4.1.11" -m "Security advisory GHSA-5xrq-8626-4rwp affects 4.0.18; fixed in 4.1.x. Deliberate bump per frontend dependency policy: check + test + make web green."
```

---

### Task 4: Shared `formatTime` helper

Four components hand-roll near-identical `formatTime` functions: ThreadsPage.svelte:39–44, LogsPage.svelte:27–32 (only copy with `second: '2-digit'`), TodosPage.svelte:30–35, BoardPage.svelte:35–39 (only copy using `'—'` instead of a literal `'—'`). Consolidate into one helper; the em-dash inconsistency dies with it.

**Files:**
- Create: `web/src/lib/utils/time.ts`
- Create: `web/src/lib/utils/time.test.ts`
- Modify: `web/src/lib/components/ThreadsPage.svelte`, `LogsPage.svelte`, `TodosPage.svelte`, `BoardPage.svelte` (delete local `formatTime`, import shared one)

**Interfaces:**
- Produces: `formatTime(iso: string, opts?: { seconds?: boolean }): string` from `$lib`-relative path `../utils/time.js` (the `.js` extension is how Vite/ESM resolves `.ts` — same convention as `dataTable.js`).

**Constraint:** The characterization tests (LogsPage.test.ts, TodosPage.test.ts, ThreadsPage.test.ts) and all other existing tests MUST pass UNCHANGED. LogsPage.test.ts:39–44 and TodosPage.test.ts:56–61 assert on literal `'—'` (U+2014) — the helper must return exactly that for empty input.

- [ ] **Step 1: Write the failing test** — create `web/src/lib/utils/time.test.ts`:

```ts
// ABOUTME: Unit tests for the shared formatTime helper.
// ABOUTME: Uses shape regexes so assertions hold in any local timezone.

import { describe, it, expect } from 'vitest';
import { formatTime } from './time.js';

describe('formatTime', () => {
  it('returns an em-dash for empty input', () => {
    expect(formatTime('')).toBe('—');
  });

  it('formats as "Mon DD HH:MM" without seconds by default', () => {
    expect(formatTime('2026-08-23T14:05:09Z')).toMatch(/^[A-Z][a-z]{2} \d{2} \d{2}:\d{2}$/);
  });

  it('includes seconds when requested', () => {
    expect(formatTime('2026-08-23T14:05:09Z', { seconds: true })).toMatch(
      /^[A-Z][a-z]{2} \d{2} \d{2}:\d{2}:\d{2}$/
    );
  });
});
```

- [ ] **Step 2: Run to verify failure**

Run (from `web/`): `npm test -- run src/lib/utils/time.test.ts`
Expected: FAIL — cannot resolve `./time.js`.

- [ ] **Step 3: Implement** — create `web/src/lib/utils/time.ts`:

```ts
// ABOUTME: Shared timestamp formatting for admin pages.
// ABOUTME: Formats ISO strings as "Mon DD HH:MM" (optionally with seconds); empty input renders an em-dash.

const EM_DASH = '—';

export function formatTime(iso: string, opts?: { seconds?: boolean }): string {
  if (!iso) return EM_DASH;
  const d = new Date(iso);
  return (
    d.toLocaleDateString('en-US', { month: 'short', day: '2-digit' }) +
    ' ' +
    d.toLocaleTimeString('en-US', {
      hour: '2-digit',
      minute: '2-digit',
      ...(opts?.seconds ? { second: '2-digit' as const } : {}),
      hour12: false,
    })
  );
}
```

- [ ] **Step 4: Run the new test to verify it passes**

Run (from `web/`): `npm test -- run src/lib/utils/time.test.ts`
Expected: PASS (3 tests).

- [ ] **Step 5: Migrate the four components.** In each, delete the local `formatTime` function and add `import { formatTime } from '../utils/time.js';` to the script block. Call sites stay identical EXCEPT LogsPage, whose call sites change from `formatTime(x)` to `formatTime(x, { seconds: true })` to preserve its seconds display. Do not touch the other em-dash literals in markup (LogsPage.svelte:66, TodosPage.svelte:83/98/103) — they are already consistent.

- [ ] **Step 6: Full web gate — characterization tests unchanged**

Run (from `web/`): `npm run check` then `npm test`; from repo root: `make web`
Expected: check 0/0; ALL tests pass with zero edits to any existing test file; build clean.

- [ ] **Step 7: Commit**

```bash
git add web/src/lib/utils/time.ts web/src/lib/utils/time.test.ts web/src/lib/components/ThreadsPage.svelte web/src/lib/components/LogsPage.svelte web/src/lib/components/TodosPage.svelte web/src/lib/components/BoardPage.svelte
git commit -m "refactor: extract shared formatTime helper"
```

---

## Endgame

After all four tasks: push `chore/post-cleanup-followups`, open a PR titled "Post-cleanup follow-ups: auth panic fix, fake-agent token, vitest bump, formatTime helper" whose body lists the four items with one line each and links this plan. The human merges.
