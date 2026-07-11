# UNIT-001 — webadmin auth core + shared store helper

## Objective

Create the shared real-store test helper every later webadmin unit uses, and cover the
auth/setup surface: login, requireAuth, setup flow, constructors, and small validators.

## Inputs / context

- `internal/webadmin/webadmin.go` — read selectively (grep for the function, then read
  that region; the file is 2,829 lines — do not read it whole).
- Existing helpers: `newTestAdmin()` (webadmin_tools_test.go), `requestWithUser(r)`,
  `findCookie` (session_test.go). Study `internal/webadmin/csrf_test.go` and
  `session_test.go` for the package's test idiom.
- How the gateway builds a real store: grep `internal/gateway/gateway_test.go` /
  `internal/store/store_test.go` for the SQLiteStore constructor (temp-dir DB file).
- Baseline gaps: `baseline-covfunc-webadmin.txt` lines for: `New`, `NewWithConfig`,
  `requireAuth`, `handleLogin`, `handleLoginPage`, `showLoginError`, `timingSafeCompare`,
  `validateUsername`, `handleSetupPage`, `handleSetupSubmit`, `parseSetupForm`,
  `showSetupError`, `getSQLiteStore` — all 0%.

## Approach

1. **`webadmin_helpers_test.go`** — define `newTestAdminWithStore(t *testing.T) *Admin`:
   open a real `*store.SQLiteStore` on a file in `t.TempDir()` (close via `t.Cleanup`),
   then build the Admin through `NewWithConfig` (this covers the constructor) with the
   store, `slog` logger, and zero-value/minimal remaining fields. Add a small helper to
   create an admin user with a known bcrypt password in the store if login tests need
   one (check `internal/store` for the AdminUser creation API and whether a bcrypt hash
   helper exists — hash a fixed password like "correct horse" with
   `golang.org/x/crypto/bcrypt` which is already a dependency).
2. **`webadmin_auth_test.go`** — table of scenarios, one test func per behavior:
   - `New(...)` delegates to `NewWithConfig` (call it, assert non-nil fields).
   - `requireAuth`: no session cookie → redirect to login; valid session (create one
     via the store or the package's session-creation path) → wrapped handler runs and
     sees the user in context; invalid/expired session → redirect.
   - `handleLoginPage` renders (status 200, body contains a login form marker).
   - `handleLogin`: missing CSRF → rejected; empty username/password → error rendered;
     unknown username → generic error; wrong password → generic error; correct
     credentials → session cookie set (assert attributes) + redirect. Follow the CSRF
     double-submit pattern the existing csrf_test.go uses (cookie + form field).
   - `timingSafeCompare`: equal, unequal, differing-length inputs.
   - `validateUsername`: valid, empty, too long / bad chars (read the function first).
   - Setup flow: `handleSetupPage` when no admin exists (renders) and when one exists
     (redirects or 403 — read the code); `handleSetupSubmit` happy path creates the
     admin user (assert via store) + error paths (`parseSetupForm` failures: password
     mismatch, short password, bad username → `showSetupError` branch).
3. Run `go test -count=1 -cover ./internal/webadmin/` — green; note the %.
4. Commit: `test(webadmin): cover auth, setup, and constructors; add real-store helper`.

## Constraints

- Do not modify webadmin_tools_test.go, csrf_test.go, or session_test.go.
- Everything in the contract's Hard prohibitions.
- If session creation for requireAuth proves awkward via the store, drive it through
  `handleLogin` success and reuse the returned cookie — real behavior over shortcuts.

## Acceptance criteria

- [ ] (runnable) `go test -count=1 ./internal/webadmin/` passes.
- [ ] (runnable) `go tool cover -func` shows `New`, `NewWithConfig`, `requireAuth`,
      `handleLogin`, `handleSetupSubmit`, `timingSafeCompare`, `validateUsername` all > 0%,
      with `handleLogin` and `requireAuth` ≥ 70%.
- [ ] (assertional) `newTestAdminWithStore` uses a real SQLiteStore in t.TempDir(), and
      every test asserts observable behavior (status/body/cookie/store state).
- [ ] (assertional) New files carry two `// ABOUTME:` lines; no production code touched.

## Dependencies

none
