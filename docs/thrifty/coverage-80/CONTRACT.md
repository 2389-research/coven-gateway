# coverage-80 — Contract

**Decomposition mode:** partition (each unit owns its own new test files)
**Planning tier:** direct

**Execution note:** two sequential executor sessions — UNIT-001..005 (webadmin) in one,
UNIT-006..008 (gateway) in the other. A single session for all 8 would overflow the
executor's context (webadmin.go alone is 2,829 lines). Within a session, units run in
listed order. The webadmin session runs first.

## Objective

Raise test coverage of `internal/webadmin` (baseline 9.6%) and `internal/gateway`
(baseline 54.9%) to **≥80% each** by adding meaningful tests only. Production code must
not change. Baselines measured on main @ 492b424; per-function baseline lists are in
`baseline-covfunc-webadmin.txt` and `baseline-covfunc-gateway.txt` in this directory.

**The math (why error branches matter):** webadmin has 1,931 statements; 80% = 1,545
covered. With webauthn.go capped around ~70% (crypto ceremonies) and csp.go done, the
rest — webadmin.go (1,261), templates.go (254, dragged up by rendering handlers),
chat.go (160), chat_app.go (25) — must reach **~82% each**. Happy-path-only tests will
not get there: cover validation failures, error returns, and edge branches too.
Gateway needs +259 of ~420 gainable statements — comfortable, same principle.

## Conventions

- Follow the package's existing test style: `httptest.NewRequest`/`httptest.NewRecorder`,
  direct explicit asserts (`if got != want { t.Errorf(...) }`), `strings.Contains` on
  bodies. No new test frameworks.
- Every new test file starts with two `// ABOUTME:` comment lines.
- Every test asserts observable behavior: status code, headers, body content, store
  state, or channel/SSE output. A test that merely calls a function to inflate coverage
  is a defect and will be rejected.
- Pristine test output: no stray prints. Expected-error paths assert the error.
- No `time.Sleep` for synchronization — use channels, contexts with deadlines, or
  polling loops with a bounded deadline. Everything must pass `-race`.
- One conventional commit per unit: `test(webadmin): …` / `test(gateway): …` — stage
  ONLY files you created or edited in that unit (plus the shared helpers file when you
  extended it).

## Interfaces (cross-unit)

**Existing helpers — reuse, never redefine** (test files in one package share a
namespace; a duplicate definition breaks the whole build):

- webadmin: `newTestAdmin()` (no args; registry+logger-only Admin, no store),
  `requestWithUser(r)` (injects an AdminUser into the request context, bypassing
  requireAuth), `findCookie(t, cookies, name)`.
- gateway: `testConfig(t)`, `newTestGateway(t)`, `newTestGatewayWithMockManager(t)`,
  `newTestGatewayWithMockManagerAndStore(t)`,
  `newTestGatewayWithAgentForBinding(t, instanceID, workingDir, principalID)`,
  `mockAgentManager`, `testMockStream`, `createTestBindingV2`, `ptrString`.
- Before defining ANY new helper, grep the package's `*_test.go` files for the name.

**UNIT-001 owns `internal/webadmin/webadmin_helpers_test.go`**, which defines
`newTestAdminWithStore(t *testing.T) *Admin` — an Admin backed by a **real
`*store.SQLiteStore`** opened in `t.TempDir()` (webadmin's secrets handlers type-assert
`a.store.(*store.SQLiteStore)`, so MockStore cannot serve them; a real store serves
every handler). Units 002–005 consume this helper. If a later webadmin unit needs
another genuinely shared helper, ADD it to this file — never fork a variant.

Gateway units add helpers to their own files only; the existing api_test.go helpers
already cover the shared needs.

## Glossary

- **covered** — exercised by at least one test that asserts its observable behavior,
  not merely executed.
- **accepted gap** — deliberately untested (list below); do not burn fix attempts there.

## Accepted gaps (DO NOT attempt)

- `gateway.go`: `createTailscaleHTTPListener`, `createTailscaleTLSListener`,
  `setupTailscaleListeners` — require a live tsnet/tailnet.
- `webauthn.go` ceremony interiors needing real authenticator attestation beyond what
  existing webauthn_test.go patterns already reach (the storable/lookup/finalize
  helpers ARE reachable — see UNIT-005).

The 80% targets were computed with these staying near 0%.

## Hard prohibitions

- **NO production (non-test) code changes.** If a function cannot be tested without a
  source change, mark that item blocked in your report and move on — never edit source
  to make it testable.
- Never stage or touch: `proto/coven-proto`, `coven-gateway.db`, `coven-gateway.db.bak`,
  any `*.db*` file. Never `git add -A` or `git add .`.
- Never use `--no-verify` or any hook-bypass flag. If a pre-commit hook fails, fix the
  root cause.
- No new module dependencies. No mock modes. Do not weaken, skip, or delete existing
  tests.
- Never run the gateway binary against the repo's `coven-gateway.db`; tests use their
  own temp stores only.

## Ownership map

- UNIT-001 → `internal/webadmin/webadmin_helpers_test.go`, `internal/webadmin/webadmin_auth_test.go`
- UNIT-002 → `internal/webadmin/webadmin_pages_test.go`
- UNIT-003 → `internal/webadmin/webadmin_admin_test.go`
- UNIT-004 → `internal/webadmin/webadmin_link_test.go`
- UNIT-005 → `internal/webadmin/webadmin_chat_test.go`, `internal/webadmin/webauthn_store_test.go`
- UNIT-006 → `internal/gateway/api_sse_test.go`
- UNIT-007 → `internal/gateway/api_handlers_test.go`
- UNIT-008 → `internal/gateway/gateway_config_test.go`, `internal/gateway/grpc_handlers_test.go`

## Dependency graph

```text
UNIT-001 → UNIT-002 → UNIT-003 → UNIT-004 → UNIT-005   (webadmin session, linear)
UNIT-006 → UNIT-007 → UNIT-008                          (gateway session, linear)
(sessions independent; webadmin session runs first)
```

## Gates

- **Per-unit self-check** (executor, after each unit):
  `go test -count=1 -cover ./internal/<pkg>/` green; note the printed coverage % per
  unit in your final report so progress toward 80% is visible.
- **Session gate** (executor, end of your unit list):
  `sh docs/thrifty/coverage-80/gate-webadmin.sh` or `sh docs/thrifty/coverage-80/gate-gateway.sh`
  (coverage ≥80% + package lint + package race). Bounded self-fix: ≤3 attempts, then
  report the failure honestly.
- **Final gate** (orchestrator): both gate scripts + `go test -count=1 ./...` +
  `golangci-lint run ./...` + `go test -race -count=1 ./internal/gateway/ ./internal/webadmin/`.
