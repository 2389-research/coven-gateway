# Security-Critical Fixes (Phase 1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the two unambiguous "wide-open by default" holes from the audit: a gateway that silently serves with zero authentication when `jwt_secret` is unset, and HTTP API handlers that accept unbounded request bodies.

**Architecture:** Both fixes are additive and surgical. Fix 1 preserves the existing intentional anonymous mode but makes it an *explicit opt-in* (`auth.allow_anonymous`) enforced only on the `serve` path — a fresh or misconfigured deployment now fails fast with an actionable message instead of coming up open. Fix 2 adds one top-level HTTP middleware that wraps every request body in `http.MaxBytesReader`, mirroring the limit the MCP server already enforces. Neither fix removes functionality.

**Tech Stack:** Go 1.25.5, stdlib `net/http`, `slog`, existing table-driven test style.

## Global Constraints

- Go version floor: `1.25.5` (from `go.mod`). Do not lower.
- Every new file starts with two `// ABOUTME: ` comment lines.
- Structured logging via `slog` only. Error wrapping via `fmt.Errorf("context: %w", err)`.
- TDD: write the failing test first, watch it fail, then implement. Conventional-commit messages, imperative mood.
- No existing test may regress. `go build ./...`, `go vet ./...`, `golangci-lint run`, and `go test ./...` must all be clean before each commit.
- Do all work on a branch: `git switch -c fix/security-critical-phase1` before Task 1.
- Anonymous mode is NOT being removed. It remains available behind the explicit opt-in. Do not delete `NoAuthUnaryInterceptor`/`NoAuthStreamInterceptor` or the `JWTSecret == ""` branches in `gateway.go`.

---

### Task 1: Refuse to serve without authentication unless explicitly opted in

**Problem (audit C-2):** `internal/gateway/gateway.go` selects `createUnauthenticatedGRPCServer` / the no-auth HTTP path whenever `cfg.Auth.JWTSecret == ""`, and `NoAuthUnaryInterceptor` injects `Roles: []string{"admin"}` for every caller (`internal/auth/interceptor.go:115,136`). Because `config.example.yaml` sets `jwt_secret: "${COVEN_JWT_SECRET}"`, a deployment that forgets the env var comes up as a fully open, admin-for-everyone control plane with only a `Warn` log. This task makes the `serve` command fail fast unless the operator either sets a secret or explicitly opts into anonymous mode.

**Files:**
- Modify: `internal/config/config.go` — add `AllowAnonymous` to `AuthConfig` (after line 38); add `ValidateServable()` method (after `Validate()`, ~line 185)
- Test: `internal/config/config_test.go` — add `TestValidateServable_Auth` (imports `strings`, `testing` already present)
- Modify: `cmd/coven-gateway/main.go` — call `cfg.ValidateServable()` inside `runServe` (after the `config.Load` block at lines 135-138)
- Modify: `config.example.yaml` — document `allow_anonymous` and the fail-fast behavior in the `auth:` block (lines 61-69)

**Interfaces:**
- Produces: `AuthConfig.AllowAnonymous bool` (yaml key `allow_anonymous`)
- Produces: `func (c *Config) ValidateServable() error` — returns a non-nil error when `JWTSecret == "" && !AllowAnonymous`; nil otherwise. Distinct from `Validate()`, which is unchanged and still called by `Load`.

- [ ] **Step 1: Write the failing test**

Add to `internal/config/config_test.go`:

```go
func TestValidateServable_Auth(t *testing.T) {
	base := func() Config {
		return Config{
			Server:   ServerConfig{GRPCAddr: ":50051", HTTPAddr: ":8080"},
			Database: DatabaseConfig{Path: "./test.db"},
		}
	}
	tests := []struct {
		name          string
		auth          AuthConfig
		wantErr       bool
		wantErrSubstr string
	}{
		{
			name:          "empty secret without opt-in is rejected",
			auth:          AuthConfig{JWTSecret: "", AllowAnonymous: false},
			wantErr:       true,
			wantErrSubstr: "auth.jwt_secret is required",
		},
		{
			name:    "empty secret with explicit opt-in is allowed",
			auth:    AuthConfig{JWTSecret: "", AllowAnonymous: true},
			wantErr: false,
		},
		{
			name:    "configured secret is allowed",
			auth:    AuthConfig{JWTSecret: "s3cret-value"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base()
			cfg.Auth = tt.auth
			err := cfg.ValidateServable()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ValidateServable() = nil, want error containing %q", tt.wantErrSubstr)
				}
				if !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Fatalf("ValidateServable() error = %q, want substring %q", err, tt.wantErrSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateServable() = %v, want nil", err)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/config/ -run TestValidateServable_Auth -v`
Expected: compile failure — `cfg.Auth.AllowAnonymous undefined` and `cfg.ValidateServable undefined`.

- [ ] **Step 3: Add the field and method**

In `internal/config/config.go`, change `AuthConfig` (lines 36-39) to:

```go
// AuthConfig holds authentication configuration.
type AuthConfig struct {
	JWTSecret             string `yaml:"jwt_secret"`
	AgentAutoRegistration string `yaml:"agent_auto_registration"` // "approved", "pending", or "disabled"
	// AllowAnonymous permits serving with no authentication when JWTSecret is empty.
	// INSECURE: every caller is granted the admin role. Local development only.
	AllowAnonymous bool `yaml:"allow_anonymous"`
}
```

Add after the `Validate()` method (after line 185):

```go
// ValidateServable checks preconditions required to serve traffic safely.
// It is stricter than Validate and is intended to be called only by the
// `serve` command, immediately after loading config. Non-serving subcommands
// (token generation, bootstrap) are unaffected.
func (c *Config) ValidateServable() error {
	if c.Auth.JWTSecret == "" && !c.Auth.AllowAnonymous {
		return errors.New(
			"auth.jwt_secret is required to serve: set COVEN_JWT_SECRET, or set " +
				"auth.allow_anonymous: true to run without authentication " +
				"(INSECURE — every caller becomes admin; local development only)")
	}
	return nil
}
```

(`errors` is already imported in `config.go`.)

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/config/ -run TestValidateServable_Auth -v`
Expected: PASS (all three subtests).

- [ ] **Step 5: Enforce it on the serve path**

In `cmd/coven-gateway/main.go`, `runServe`, replace the load block (lines 135-138):

```go
	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
```

with:

```go
	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Refuse to stand up open listeners unless the operator explicitly opted
	// into anonymous mode. Prevents a missing COVEN_JWT_SECRET from silently
	// producing a fully open, admin-for-everyone control plane.
	if err := cfg.ValidateServable(); err != nil {
		return fmt.Errorf("refusing to serve: %w", err)
	}
```

- [ ] **Step 6: Document the setting in the example config**

In `config.example.yaml`, replace the `auth:` block (lines 61-69) so the secret's necessity and the opt-in are explicit. Keep the existing `${COVEN_JWT_SECRET}` default:

```yaml
auth:
  # jwt_secret secures the HTTP API and the gRPC client/admin services.
  # If it is empty, the gateway REFUSES TO START unless allow_anonymous is true.
  # Generate one with: openssl rand -base64 32
  jwt_secret: "${COVEN_JWT_SECRET}"

  # allow_anonymous: run with NO authentication when jwt_secret is empty.
  # INSECURE — every caller is granted the admin role. Local development only.
  # allow_anonymous: false

  # agent_auto_registration controls whether connecting agents are auto-approved.
  # "approved" trusts any agent that reaches the gRPC port. Use "pending" on
  # untrusted networks and approve agents manually via coven-admin.
  agent_auto_registration: "approved"
```

- [ ] **Step 7: Verify the whole suite and static checks are clean**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all pass. If any existing test called `config.Load` on a fixture with an empty secret AND relied on serving, it will surface here — fix by setting `allow_anonymous: true` in that fixture. (None is expected: `Load` calls `Validate`, not `ValidateServable`.)
Run: `golangci-lint run`
Expected: clean.

- [ ] **Step 8: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go cmd/coven-gateway/main.go config.example.yaml
git commit -m "fix(auth): refuse to serve without auth unless explicitly opted in"
```

---

### Task 2: Bound HTTP request body sizes

**Problem (audit C-3):** No gateway or webadmin HTTP handler limits request body size before `json.NewDecoder(r.Body).Decode(...)` (`internal/gateway/api.go:554,759,1090,1347,1413`). The MCP server already caps bodies at 1 MiB (`internal/mcp/server.go:330`), but the main API does not — so a caller can POST an arbitrarily large body and hold server memory. Combined with Task 1's previously-open default, this was an unauthenticated memory-exhaustion vector. One top-level middleware fixes every handler at once (one source of truth), including webadmin POSTs.

**Files:**
- Create: `internal/gateway/middleware.go`
- Test: `internal/gateway/middleware_test.go`
- Modify: `internal/gateway/gateway.go` — wrap the mux in the `http.Server` construction (lines 409-413)

**Interfaces:**
- Produces: `const MaxAPIBodySize int64 = 1 << 20`
- Produces: `func maxBytesMiddleware(next http.Handler) http.Handler` — returns a handler that replaces `r.Body` with `http.MaxBytesReader(w, r.Body, MaxAPIBodySize)`. Once the limit is exceeded, `Read` returns an error and the handler's decode fails cleanly with `400`-class handling; the response stream is unaffected (only the request body is wrapped).

- [ ] **Step 1: Write the failing test**

Create `internal/gateway/middleware_test.go`:

```go
// ABOUTME: Tests for gateway HTTP middleware.
// ABOUTME: Verifies request body size limits are enforced.

package gateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMaxBytesMiddleware_RejectsOversizedBody(t *testing.T) {
	var readErr error
	h := maxBytesMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, readErr = io.ReadAll(r.Body)
		if readErr != nil {
			http.Error(w, "too big", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	oversized := strings.NewReader(strings.Repeat("a", int(MaxAPIBodySize)+1))
	req := httptest.NewRequest(http.MethodPost, "/api/send", oversized)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusRequestEntityTooLarge)
	}
	if readErr == nil {
		t.Fatal("expected a read error from the oversized body, got nil")
	}
}

func TestMaxBytesMiddleware_AllowsNormalBody(t *testing.T) {
	h := maxBytesMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			http.Error(w, "unexpected", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/send",
		strings.NewReader(`{"sender":"me","content":"hi"}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/gateway/ -run TestMaxBytesMiddleware -v`
Expected: compile failure — `maxBytesMiddleware` and `MaxAPIBodySize` undefined.

- [ ] **Step 3: Implement the middleware**

Create `internal/gateway/middleware.go`:

```go
// ABOUTME: HTTP middleware for the gateway API surface.
// ABOUTME: Bounds request body sizes to protect against memory exhaustion.

package gateway

import "net/http"

// MaxAPIBodySize is the maximum accepted HTTP request body (1 MiB).
// Matches the MCP server's limit so every POST surface is bounded.
const MaxAPIBodySize int64 = 1 << 20

// maxBytesMiddleware caps the request body size for every request passing
// through it. http.MaxBytesReader returns an error from Read once the limit is
// exceeded, so handlers decoding JSON get a clean error instead of buffering
// unbounded input. Only the request body is wrapped; response streaming
// (e.g. SSE) is unaffected.
func maxBytesMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, MaxAPIBodySize)
		}
		next.ServeHTTP(w, r)
	})
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/gateway/ -run TestMaxBytesMiddleware -v`
Expected: PASS (both subtests).

- [ ] **Step 5: Wire the middleware into the HTTP server**

In `internal/gateway/gateway.go`, change the `http.Server` construction (lines 409-413) from:

```go
	gw.httpServer = &http.Server{
		Addr:              cfg.Server.HTTPAddr,
		Handler:           webadmin.CSPMiddleware(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
```

to:

```go
	gw.httpServer = &http.Server{
		Addr:              cfg.Server.HTTPAddr,
		Handler:           webadmin.CSPMiddleware(maxBytesMiddleware(mux)),
		ReadHeaderTimeout: 10 * time.Second,
	}
```

- [ ] **Step 6: Verify build, vet, full suite, and lint**

Run: `go build ./... && go vet ./... && go test ./... && golangci-lint run`
Expected: all clean. Existing SSE and `/api/send` tests continue to pass (their bodies are well under 1 MiB).

- [ ] **Step 7: Commit**

```bash
git add internal/gateway/middleware.go internal/gateway/middleware_test.go internal/gateway/gateway.go
git commit -m "fix(api): bound HTTP request body sizes to prevent memory exhaustion"
```

---

## Deferred — need your decision before I plan them

These audit items are real but each needs a design call I won't guess at (per "never invent technical details"):

- **C-1 — MCP unauthenticated access (`gateway.go:401`).** Verified nuance: an anonymous MCP caller does NOT get "full compromise." `handleToolsList` returns the *entire tool catalog* to callers with empty capabilities (`server.go:480`), and `handleToolsCall` will execute any tool whose `RequiredCapabilities` is empty (`server.go:534` + `hasRequiredCapabilities:719`); cap-gated tools stay protected. **Decision needed:** should MCP require a capability token for *all* access (breaks the current "bare `/mcp`" convenience), or only restrict `tools/list` disclosure and keep zero-cap tools open? And do any registered builtin/pack tools declare zero required capabilities today (that's the actual exposed set)? Answer drives whether this is a config flag (`mcp.require_auth`) or a catalog-visibility change.
- **Important security batch (own plan):** constant-time CSRF compare (`webadmin.go:475`), enforce CSRF on logout (`webadmin.go:602`), add `HSTS` + `X-Content-Type-Options` (`csp.go`), and HTTP `ReadTimeout`/`WriteTimeout` (needs SSE-aware handling via `http.ResponseController`, so not a one-liner). These are small but I haven't read `csp.go`/`webadmin.go` closely enough to write no-placeholder steps yet.
- **Rate limiting on login/link/invite** and **JWT revocation (`jti` + store)** — both add mechanism/deps; worth a YAGNI conversation about scope first.
- **`agent_auto_registration: "approved"` default** — flipping the example to `"pending"` is a one-line doc change but a real behavior/security posture decision for your users.
