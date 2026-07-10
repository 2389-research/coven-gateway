# Security Hardening Batch Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the deferred web-surface hardening findings from the 2026-07 audit: constant-time CSRF comparison, baseline security headers, HTTP server idle timeout, and correct logout cookie attributes (retiring all four G124 lint findings).

**Architecture:** All changes are small, local edits to existing code paths: `internal/webadmin/` (CSRF validation, headers middleware, logout cookies) and `internal/gateway/gateway.go` (one `http.Server` field). No new packages, no schema changes, no API changes.

**Tech Stack:** Go stdlib only (`crypto/subtle`, `net/http`, `sync`, `time`). No new dependencies.

**Evidence base:** `.superpowers/sdd/hardening-investigation.md` (read-only investigation, verified by controller spot-checks 2026-07-09).

## Global Constraints

- Branch: `fix/security-hardening` (base = main @ e9807f7). Conventional commits, imperative mood.
- NEVER stage `proto/coven-proto` (pre-existing dirty submodule pointer), `coven-gateway.db`, or `coven-gateway.db.bak`. `git add` only the files your task names. Never `git add -A`.
- NEVER use `--no-verify` or any hook-bypass flag.
- NEVER run the built server against the repo's `coven-gateway.db` (live data).
- **`http.Server.WriteTimeout` MUST remain 0 (unset).** This one server serves SSE at `/api/send`, `/chat/{id}/stream`, and `/api/health/stream`; any WriteTimeout kills active streams. Same for `ReadTimeout`: leave unset. Only `ReadHeaderTimeout` (existing) and `IdleTimeout` (Task 3) may be set.
- **Cookie `Secure` MUST stay conditional: `r.TLS != nil`.** Never literal `true` — plain-HTTP tailnet deployments (Tailscale without `https: true`) must still send/clear cookies. This applies to every `http.SetCookie` in webadmin.
- Exit criteria for the whole branch: `go build ./...`, `go vet ./...`, `gofmt -l .` empty, `golangci-lint run ./...` **zero issues** (this branch retires the 4 pre-existing G124 findings), `go test ./...` all pass, `go test -race ./internal/webadmin/ ./internal/gateway/` clean.
- Test style: match existing package tests (`httptest.NewRequest`/`NewRecorder`, table-less explicit asserts, `strings.Contains` for body checks). Test files start with two `ABOUTME:` comment lines.
- Do not modify historical plan documents under `docs/plans/` when doing rename-safety greps — they are records, not live code.

## Explicitly Out of Scope (do not "fix" these)

- **`agent_auto_registration` default** — already safe. `gateway.go:144-147` normalizes empty config to `"disabled"`, and `interceptor.go` independently treats `""` as disabled. The audit item is closed with no code change.
- **`/mcp` authentication** — deliberately unauthenticated; the tailnet is the auth boundary (Harper's decision, 2026-07-08). Do not add auth or flag it in reviews.
- **JWT hard revocation (JTI deny-list)** — deferred. Soft revocation already exists: every request re-checks principal status in the DB (`checkPrincipalStatus`), so revoking a principal cuts access on the next request even with a valid JWT.
- **Rate limiting (all surfaces, including webadmin login)** — dropped from this batch by Harper's decision (2026-07-09). Login brute force remains bounded only by bcrypt latency; tracked as a backlog GitHub issue instead. The `/api/*` and gRPC-connect surfaces were never in scope.
- **CSRF double-submit weakness** (token not bound to session server-side) — pre-existing design, not in scope. Task 1 only fixes the comparison timing.

---

### Task 1: Constant-time CSRF token comparison

**Files:**
- Modify: `internal/webadmin/webadmin.go` (function `validateCSRF`, currently ~line 462-476)
- Create: `internal/webadmin/csrf_test.go`

**Interfaces:**
- Consumes: `CSRFCookieName` const, `newTestAdmin(nil)` helper from `webadmin_tools_test.go`.
- Produces: no signature changes — `validateCSRF(r *http.Request) bool` unchanged. Task 4 relies on CSRF behavior being pinned by these tests.

**TDD note (honest framing):** A timing side-channel is not observable from a unit test — `==` and `subtle.ConstantTimeCompare` are behaviorally identical. So this task has **no RED phase**. The tests below are *characterization tests*: they pin the accept/reject behavior before the swap so the swap cannot regress it. The security property itself is enforced by the implementation step and verified by review. The codebase already uses this standard for login (`timingSafeCompare`, webadmin.go:537) — this task brings CSRF up to the same bar.

- [ ] **Step 1: Write characterization tests**

Create `internal/webadmin/csrf_test.go`:

```go
// ABOUTME: Tests for CSRF token validation behavior in the admin UI.
// ABOUTME: Pins accept/reject semantics across form-field and header token sources.

package webadmin

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// csrfRequest builds a POST with the given cookie and form token values.
// Empty cookieVal means no cookie is attached at all.
func csrfRequest(cookieVal, formVal string) *http.Request {
	form := url.Values{}
	if formVal != "" {
		form.Set("csrf_token", formVal)
	}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if cookieVal != "" {
		req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: cookieVal})
	}
	return req
}

func TestValidateCSRF_MatchingFormToken(t *testing.T) {
	admin := newTestAdmin(nil)
	if !admin.validateCSRF(csrfRequest("tok-abc123", "tok-abc123")) {
		t.Error("matching cookie and form token should validate")
	}
}

func TestValidateCSRF_MatchingHeaderToken(t *testing.T) {
	admin := newTestAdmin(nil)
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "tok-abc123"})
	req.Header.Set("X-CSRF-Token", "tok-abc123")
	if !admin.validateCSRF(req) {
		t.Error("matching cookie and X-CSRF-Token header should validate")
	}
}

func TestValidateCSRF_MismatchedToken(t *testing.T) {
	admin := newTestAdmin(nil)
	if admin.validateCSRF(csrfRequest("tok-abc123", "tok-abc124")) {
		t.Error("mismatched token must not validate")
	}
}

func TestValidateCSRF_DifferentLengthToken(t *testing.T) {
	admin := newTestAdmin(nil)
	if admin.validateCSRF(csrfRequest("tok-abc123", "tok-abc")) {
		t.Error("different-length token must not validate")
	}
}

func TestValidateCSRF_MissingCookie(t *testing.T) {
	admin := newTestAdmin(nil)
	if admin.validateCSRF(csrfRequest("", "tok-abc123")) {
		t.Error("request without CSRF cookie must not validate")
	}
}

func TestValidateCSRF_EmptyFormAndHeader(t *testing.T) {
	admin := newTestAdmin(nil)
	if admin.validateCSRF(csrfRequest("tok-abc123", "")) {
		t.Error("request without any submitted token must not validate")
	}
}
```

- [ ] **Step 2: Run tests against current code — all must PASS (characterization)**

Run: `go test ./internal/webadmin/ -run TestValidateCSRF -v`
Expected: 6 tests PASS. (If any fail, STOP — the characterization is wrong; report BLOCKED.)

- [ ] **Step 3: Swap to constant-time comparison**

In `internal/webadmin/webadmin.go`, add `"crypto/subtle"` to the imports, then change the final return of `validateCSRF` from:

```go
	return formToken != "" && formToken == cookie.Value
```

to:

```go
	if formToken == "" {
		return false
	}
	// Constant-time compare: == short-circuits on the first differing byte,
	// leaking token prefixes through response timing.
	return subtle.ConstantTimeCompare([]byte(formToken), []byte(cookie.Value)) == 1
}
```

(Keep the rest of the function — cookie fetch, form/header fallback — exactly as-is.)

- [ ] **Step 4: Run tests again — all must still PASS**

Run: `go test ./internal/webadmin/ -run TestValidateCSRF -v`
Expected: 6 tests PASS.

Run: `go build ./... && go vet ./internal/webadmin/`
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add internal/webadmin/webadmin.go internal/webadmin/csrf_test.go
git commit -m "fix(webadmin): use constant-time comparison for CSRF token validation"
```

---

### Task 2: Baseline security headers on every HTTP response

**Files:**
- Modify: `internal/webadmin/csp.go` (rename `CSPMiddleware` → `SecurityHeadersMiddleware`, add headers, update ABOUTME)
- Modify: `internal/webadmin/csp_test.go` (update call sites, add new tests)
- Modify: `internal/gateway/gateway.go` (one call site, currently line 411)

**Interfaces:**
- Consumes: `assets.Manifest` (existing), `cspProd`/`cspDev` consts (existing, unchanged).
- Produces: `func SecurityHeadersMiddleware(next http.Handler) http.Handler` — replaces `CSPMiddleware`. `internal/gateway/gateway.go` is the only production caller.

**Header set (exact values):**

| Header | Value | When |
|---|---|---|
| `Content-Security-Policy` | existing `cspProd`/`cspDev` | always (unchanged) |
| `X-Content-Type-Options` | `nosniff` | always |
| `Referrer-Policy` | `strict-origin-when-cross-origin` | always |
| `X-Frame-Options` | `DENY` | always (belt-and-suspenders with CSP `frame-ancestors 'none'`) |
| `Permissions-Policy` | `camera=(), microphone=(), geolocation=()` | always (does not list `publickey-credentials-*`, so WebAuthn keeps its default `self` allowlist) |
| `Strict-Transport-Security` | `max-age=63072000; includeSubDomains` | **only when `r.TLS != nil`** |

**HSTS constraint:** never emit HSTS on a plain-HTTP connection — it would poison browsers against plain-HTTP tailnet deployments. `r.TLS != nil` is true for the Tailscale `https: true` mode (`tls.NewListener` in-process). Under Funnel, TLS may terminate at the Tailscale edge; if `r.TLS` is nil there, we simply don't emit HSTS — that is the accepted limitation, do not try to detect Funnel via config.

- [ ] **Step 1: Write failing tests**

Append to `internal/webadmin/csp_test.go`:

```go
func TestSecurityHeaders_SetOnEveryResponse(t *testing.T) {
	handler := SecurityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
		"X-Frame-Options":        "DENY",
		"Permissions-Policy":     "camera=(), microphone=(), geolocation=()",
	}
	for header, value := range want {
		if got := rec.Header().Get(header); got != value {
			t.Errorf("%s = %q, want %q", header, got, value)
		}
	}
	if rec.Header().Get("Content-Security-Policy") == "" {
		t.Error("Content-Security-Policy missing")
	}
	if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("HSTS must not be set on plain-HTTP connections, got %q", got)
	}
}

func TestSecurityHeaders_HSTSOnlyOverTLS(t *testing.T) {
	handler := SecurityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.TLS = &tls.ConnectionState{}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	const want = "max-age=63072000; includeSubDomains"
	if got := rec.Header().Get("Strict-Transport-Security"); got != want {
		t.Errorf("Strict-Transport-Security = %q, want %q", got, want)
	}
}
```

Add `"crypto/tls"` to csp_test.go imports.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/webadmin/ -run TestSecurityHeaders -v`
Expected: FAIL to compile — `undefined: SecurityHeadersMiddleware`. That is the RED state.

- [ ] **Step 3: Implement — rename and add headers**

Replace the `CSPMiddleware` function in `internal/webadmin/csp.go` with:

```go
// SecurityHeadersMiddleware wraps an http.Handler and sets baseline security
// headers on every response: Content-Security-Policy, X-Content-Type-Options,
// Referrer-Policy, X-Frame-Options, Permissions-Policy, and — on TLS
// connections only — Strict-Transport-Security.
// In dev mode (no Vite manifest), the CSP permits the Vite dev server origin.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	// Evaluate once at startup: manifest is loaded during init().
	policy := cspProd
	if assets.Manifest == nil {
		policy = cspDev
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", policy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if r.TLS != nil {
			// HSTS only over TLS: emitting it on a plain-HTTP tailnet
			// deployment would poison browsers against the plain listener.
			// Under Funnel, TLS may terminate at the Tailscale edge before
			// this process; in that case r.TLS is nil and no HSTS is sent.
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}
```

Update the file's ABOUTME header to:

```go
// ABOUTME: Security headers middleware for all HTTP responses (CSP, nosniff, HSTS, etc.)
// ABOUTME: Restricts script/style/connect sources to same-origin for XSS protection
```

Update all `CSPMiddleware(` call sites to `SecurityHeadersMiddleware(`:
- `internal/gateway/gateway.go:411` — `Handler: webadmin.SecurityHeadersMiddleware(maxBytesMiddleware(mux)),`
- `internal/webadmin/csp_test.go` — 4 existing test functions call `CSPMiddleware(`; update the calls. Keep the existing test **names** (`TestCSPMiddleware_*`) — they still describe the CSP behavior they assert.

- [ ] **Step 4: Rename safety check**

Run: `grep -rn "CSPMiddleware" --include="*.go" .`
Expected: zero hits. (Hits in `docs/` markdown are historical records — leave them.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/webadmin/ -v -run "TestSecurityHeaders|TestCSPMiddleware"`
Expected: all PASS (2 new + 4 existing).

Run: `go build ./... && go test ./internal/gateway/`
Expected: clean, all pass.

- [ ] **Step 6: Commit**

```bash
git add internal/webadmin/csp.go internal/webadmin/csp_test.go internal/gateway/gateway.go
git commit -m "feat(webadmin): set baseline security headers on all HTTP responses"
```

---

### Task 3: HTTP server IdleTimeout + SSE-safe timeout invariant test

**Files:**
- Modify: `internal/gateway/gateway.go` (the `http.Server` literal, currently lines 409-413)
- Modify: `internal/gateway/gateway_test.go` (add one test; use existing `testConfig(t)` + `testLogger()` helpers)

**Interfaces:**
- Consumes: `New(cfg *config.Config, logger *slog.Logger) (*Gateway, error)`, `gw.httpServer` (unexported field, same package), `testConfig(t)`, `testLogger()`.
- Produces: nothing new — one struct field addition.

- [ ] **Step 1: Write the failing test**

Add to `internal/gateway/gateway_test.go`:

```go
func TestHTTPServerTimeouts(t *testing.T) {
	cfg := testConfig(t)
	logger := testLogger()

	gw, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer gw.Shutdown(context.Background())

	if got, want := gw.httpServer.ReadHeaderTimeout, 10*time.Second; got != want {
		t.Errorf("ReadHeaderTimeout = %v, want %v", got, want)
	}
	if got, want := gw.httpServer.IdleTimeout, 120*time.Second; got != want {
		t.Errorf("IdleTimeout = %v, want %v", got, want)
	}
	// WriteTimeout and ReadTimeout MUST stay 0: this server holds SSE streams
	// open indefinitely (/api/send, /chat/{id}/stream, /api/health/stream);
	// either timeout would kill active streams mid-flight.
	if gw.httpServer.WriteTimeout != 0 {
		t.Errorf("WriteTimeout = %v, must be 0 (SSE)", gw.httpServer.WriteTimeout)
	}
	if gw.httpServer.ReadTimeout != 0 {
		t.Errorf("ReadTimeout = %v, must be 0 (SSE)", gw.httpServer.ReadTimeout)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/gateway/ -run TestHTTPServerTimeouts -v`
Expected: FAIL — `IdleTimeout = 0s, want 2m0s`. (The other three asserts already hold.)

- [ ] **Step 3: Add IdleTimeout**

In `internal/gateway/gateway.go`, change the server literal to:

```go
	gw.httpServer = &http.Server{
		Addr:              cfg.Server.HTTPAddr,
		Handler:           webadmin.SecurityHeadersMiddleware(maxBytesMiddleware(mux)),
		ReadHeaderTimeout: 10 * time.Second,
		// IdleTimeout only reaps idle keep-alive connections between requests;
		// it does not touch in-flight requests, so active SSE streams are safe.
		// WriteTimeout/ReadTimeout are deliberately unset: they count against
		// long-lived SSE responses and would kill streams mid-flight.
		IdleTimeout: 120 * time.Second,
	}
```

(If Task 2 has not run yet in your session, the Handler line still says `CSPMiddleware` — leave whatever name is current; this task only adds `IdleTimeout` and the comment.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/gateway/ -run TestHTTPServerTimeouts -v`
Expected: PASS.

Run: `go test ./internal/gateway/`
Expected: all pass (SSE handler tests unaffected).

- [ ] **Step 5: Commit**

```bash
git add internal/gateway/gateway.go internal/gateway/gateway_test.go
git commit -m "fix(gateway): add IdleTimeout to HTTP server and pin SSE-safe timeout invariants"
```

---

### Task 4: Logout clear-cookie attributes + retire all G124 lint findings

**Files:**
- Modify: `internal/webadmin/webadmin.go` (`handleLogout` clear-cookies, currently ~lines 612-626; `ensureCSRFToken` cookie ~line 450; `createSession` cookie ~line 497)
- Create: `internal/webadmin/session_test.go`

**Interfaces:**
- Consumes: `SessionCookieName`, `CSRFCookieName` consts; `newTestAdmin(nil)`; `handleLogout` (no store needed when the request carries no session cookie).
- Produces: lint-zero on `./internal/webadmin/` — the branch exit criterion depends on this task.

**Background:** `golangci-lint` currently reports 4 gosec G124 findings (webadmin.go 450, 497, 614, 623). Lines 614/623 are real gaps: the logout clear-cookies set neither `Secure` nor `SameSite`. Lines 450/497 are false positives: all three attributes are set, but gosec cannot prove the non-literal `Secure: r.TLS != nil`. After this task's fix, 614/623 will use the same non-literal pattern and will be flagged for the same reason — so all four sites get a `//nolint` with justification.

**Why the clear-cookies must mirror the set-cookies:** browsers apply "secure cookie protection" (RFC 6265bis) — a `Set-Cookie` from a non-secure context may not evict a `Secure` cookie. `Secure: r.TLS != nil` on the clear naturally matches the set-time value because logout happens over the same scheme as login. `SameSite` mirrors set-time values for the same reason: session = `Lax` (matching `createSession`), CSRF = `Strict` (matching `ensureCSRFToken`).

- [ ] **Step 1: Write failing tests**

Create `internal/webadmin/session_test.go`:

```go
// ABOUTME: Tests for admin session and CSRF cookie attributes.
// ABOUTME: Asserts Secure/SameSite/HttpOnly on set and clear paths, over HTTP and TLS.

package webadmin

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

// findCookie returns the last Set-Cookie entry with the given name, or nil.
func findCookie(t *testing.T, cookies []*http.Cookie, name string) *http.Cookie {
	t.Helper()
	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == name {
			found = c
		}
	}
	if found == nil {
		t.Fatalf("no Set-Cookie for %q", name)
	}
	return found
}

func TestHandleLogout_ClearCookieAttributes_PlainHTTP(t *testing.T) {
	admin := newTestAdmin(nil)
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	rec := httptest.NewRecorder()

	admin.handleLogout(rec, req)

	cookies := rec.Result().Cookies()

	session := findCookie(t, cookies, SessionCookieName)
	if session.MaxAge >= 0 {
		t.Errorf("session clear cookie MaxAge = %d, want negative", session.MaxAge)
	}
	if !session.HttpOnly {
		t.Error("session clear cookie must be HttpOnly")
	}
	if session.Secure {
		t.Error("session clear cookie must not be Secure over plain HTTP (must mirror set-time value)")
	}
	if session.SameSite != http.SameSiteLaxMode {
		t.Errorf("session clear cookie SameSite = %v, want Lax (mirrors createSession)", session.SameSite)
	}

	csrf := findCookie(t, cookies, CSRFCookieName)
	if csrf.MaxAge >= 0 {
		t.Errorf("CSRF clear cookie MaxAge = %d, want negative", csrf.MaxAge)
	}
	if !csrf.HttpOnly {
		t.Error("CSRF clear cookie must be HttpOnly")
	}
	if csrf.Secure {
		t.Error("CSRF clear cookie must not be Secure over plain HTTP (must mirror set-time value)")
	}
	if csrf.SameSite != http.SameSiteStrictMode {
		t.Errorf("CSRF clear cookie SameSite = %v, want Strict (mirrors ensureCSRFToken)", csrf.SameSite)
	}
}

func TestHandleLogout_ClearCookieAttributes_TLS(t *testing.T) {
	admin := newTestAdmin(nil)
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.TLS = &tls.ConnectionState{}
	rec := httptest.NewRecorder()

	admin.handleLogout(rec, req)

	cookies := rec.Result().Cookies()
	for _, name := range []string{SessionCookieName, CSRFCookieName} {
		c := findCookie(t, cookies, name)
		if !c.Secure {
			t.Errorf("%s clear cookie must be Secure over TLS (must mirror set-time value)", name)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/webadmin/ -run TestHandleLogout_ClearCookie -v`
Expected: FAIL — SameSite asserts fail in the PlainHTTP test (`SameSite = 0, want Lax/Strict`), and both Secure asserts fail in the TLS test.

- [ ] **Step 3: Fix the clear-cookies and annotate all four G124 sites**

In `handleLogout` (webadmin.go, currently ~line 612), replace the two clear-cookie blocks with:

```go
	// Clear session cookie. Attributes mirror createSession: browsers may
	// refuse to evict a Secure cookie via a Set-Cookie lacking Secure.
	//nolint:gosec // G124 false positive: Secure is intentionally conditional on r.TLS — plain-HTTP tailnet deployments must still clear this cookie.
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

	// Clear CSRF cookie. Attributes mirror ensureCSRFToken.
	//nolint:gosec // G124 false positive: Secure is intentionally conditional on r.TLS — plain-HTTP tailnet deployments must still clear this cookie.
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	})
```

Then add the same style of annotation to the two existing false-positive sites — place the `//nolint` line immediately above `http.SetCookie(w, &http.Cookie{`:

In `ensureCSRFToken` (~line 449):
```go
	// Set cookie (path "/" so it works for both root and /admin routes)
	//nolint:gosec // G124 false positive: Secure is intentionally conditional on r.TLS — plain-HTTP tailnet deployments must still receive this cookie.
	http.SetCookie(w, &http.Cookie{
```

In `createSession` (~line 496):
```go
	// Set cookie (path "/" so it works for both root and /admin routes)
	//nolint:gosec // G124 false positive: Secure is intentionally conditional on r.TLS — plain-HTTP tailnet deployments must still receive this cookie.
	http.SetCookie(w, &http.Cookie{
```

(Do not change any attribute on the 450/497 cookies — they are already correct.)

**Note on nolint placement:** golangci-lint applies `//nolint` on its own line to the *next* line; the finding column is the composite literal on the `http.SetCookie(` line, so the placement above works. If lint still reports a site after Step 4's run, move that annotation to end-of-line on the flagged line instead — verify with the lint run, don't guess.

- [ ] **Step 4: Run tests and lint to verify**

Run: `go test ./internal/webadmin/ -run TestHandleLogout_ClearCookie -v`
Expected: PASS (both tests).

Run: `golangci-lint run ./internal/webadmin/`
Expected: **0 issues** — all four G124 findings retired.

Run: `go test ./internal/webadmin/`
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/webadmin/webadmin.go internal/webadmin/session_test.go
git commit -m "fix(webadmin): set Secure and SameSite on logout clear-cookies, retire G124 findings"
```

---

## Final Verification (whole branch)

- [ ] `go build ./...` — clean
- [ ] `go vet ./...` — clean
- [ ] `gofmt -l .` — empty (proto-generated files excluded if pre-existing)
- [ ] `golangci-lint run ./...` — **0 issues** (was 4 G124 on main)
- [ ] `go test ./...` — all pass
- [ ] `go test -race ./internal/webadmin/ ./internal/gateway/` — clean
