# Follow-ups Round 2: formatTime Consolidation + E2E Agent Auth

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Finish the formatTime consolidation started in PR #118 (five remaining real copies) and wire agent auth into the playwright chat e2e so the three "Chat with connected agent" tests pass against an auth-enforcing gateway.

**Architecture:** Task 1 widens the shared helper to `string | null`, migrates four byte-identical page copies plus LinkPage (which deliberately gains a date prefix on expiry timestamps), and renames ChatMessage's unrelated clock-time formatter so it stops miscounting as a copy. Task 2 makes `ensureAdminUser` capture the API token that the setup completion page displays, then passes it to fake-agent's existing `-token` flag, with a `COVEN_E2E_AGENT_TOKEN` env override for reused-DB runs.

**Tech Stack:** Svelte 5 + Vitest (web unit), Playwright (e2e, local-only — not in CI), Go templates (one possible data-testid addition in internal/webadmin).

**Spec:** This plan is its own spec. Requirements are fixed by PR #118's final review (findings I1/M1), Harper's selection on 2026-08-23 ("formatTime consolidation" + "Wire -token into e2e harness"), and the two investigation reports summarized inline. There is no external spec document; rulings resolve against this plan's text.

## Global Constraints

- Branch: `chore/formattime-e2e-token` from current main (ea0af7b). All work on this branch; the human merges the PR.
- Conventional commits, imperative, present tense. NEVER bypass hooks — `--no-verify`, `--no-hooks`, `--no-pre-commit-hook` are forbidden. Pre-commit runs go fmt/vet/test + tidy + hygiene (~1 min).
- If plain `go` fails with a GOROOT error, use `env -u GOROOT mise exec -- go ...`.
- NEVER stage `proto/coven-proto`, `docs/plans/2026-08-23-qr-pairing-gateway-design.md`, or `docs/plans/2026-08-23-qr-pairing-gateway-implementation.md` (a parallel session owns the QR docs). NEVER `git add -A` — stage explicit paths only.
- NO dependency changes: `web/package.json` and `web/package-lock.json` must be untouched.
- Zero new warnings in test or lint output. Existing test files stay byte-unchanged except where a task names them.
- New hand-written source files start with 2-line `ABOUTME:` header comments.
- `docs/plans/frontend-redesign/**` is off-limits.
- Never edit the committed `web/playwright.config.ts`. Local e2e verification uses an uncommitted temp copy: gateway HTTP on :9090 (`baseURL` changed accordingly), temp DB and temp gateway config under /tmp, run with `--config <temp>`, delete afterward. Repo-root :8080 is usually held by an unrelated `agentsview` process.
- Web commands run from `web/`: `npm test` (vitest), `npm run check` (svelte-check), `make web` from repo root for the production build.

---

### Task 1: formatTime consolidation (five copies + ChatMessage rename)

**Files:**
- Modify: `web/src/lib/utils/time.ts` (widen signature)
- Modify: `web/src/lib/utils/time.test.ts` (null-input test)
- Modify: `web/src/lib/components/AgentDetailPage.svelte` (~lines 42–47: local formatTime)
- Modify: `web/src/lib/components/PrincipalsPage.svelte` (~lines 113–118)
- Modify: `web/src/lib/components/SecretsPage.svelte` (~lines 151–156)
- Modify: `web/src/lib/components/ThreadDetailPage.svelte` (~lines 36–41)
- Modify: `web/src/lib/components/LinkPage.svelte` (~lines 34–38 + call site ~151)
- Modify: `web/src/lib/components/ChatMessage.svelte` (~lines 30–32: rename only)
- Create: `web/src/lib/components/LinkPage.test.ts`

**Interfaces:**
- Consumes: `formatTime(iso, opts?)` from `web/src/lib/utils/time.ts` (landed in PR #118).
- Produces: `formatTime(iso: string | null, opts?: { seconds?: boolean }): string` — same body, wider input type. No other task consumes this.

**Investigation facts (verified 2026-08-23):**
- AgentDetailPage, SecretsPage, ThreadDetailPage: local copies byte-equivalent to the shared default variant (`'—'` is the same character as `'—'`). Call sites pass non-null strings: `t.UpdatedAt` (AgentDetailPage:206), `s.UpdatedAt` (SecretsPage:288), `thread.CreatedAt`/`thread.UpdatedAt`/`msg.CreatedAt` (ThreadDetailPage:91,95,143).
- PrincipalsPage: identical format but signature `(iso: string | null)`; call site `p.LastSeen` (line 198) where `LastSeen: string | null` — null is common. This forces the helper widening.
- LinkPage: `toLocaleTimeString('en-US', {hour,minute,second,'2-digit',hour12:false})` only — time-with-seconds, NO date. Single call site: `{formatTime(code.ExpiresAt)}` (line 151), `ExpiresAt: string`. Migrating to `formatTime(x, { seconds: true })` adds the date prefix — a DELIBERATE visible change (an expiry time without a date is ambiguous). Flag it in the commit body and PR.
- ChatMessage: `formatTime(date: Date)` → `date.toLocaleTimeString([], {hour,minute:'2-digit'})` — Date input, system locale, time-only chat bubbles. NOT a copy of the shared helper; consolidating it would either bloat the helper API for one caller or change chat UX. Decision: keep it local, rename to `formatClockTime`, add an intent comment. `ChatMessage.test.ts` exists but asserts nothing about time — it must stay byte-unchanged.
- None of the five migrated pages has a test file. `renders.test.ts` covers only design-system primitives, not pages. Do NOT add page tests beyond `LinkPage.test.ts` (that one covers the only visible behavior change). Model its scaffolding on `web/src/lib/components/ThreadsPage.test.ts` (same render/fixture pattern, jsdom).

- [ ] **Step 1: Write the failing null-input test**

Append to `web/src/lib/utils/time.test.ts`:

```typescript
it('renders em-dash for null input', () => {
  expect(formatTime(null)).toBe('—');
});
```

- [ ] **Step 2: Verify red at the type level, green at runtime**

Run from `web/`: `npm test -- src/lib/utils/time.test.ts` → PASSES (the transpiled `!iso` guard already handles null). Then `npm run check` → FAILS: `formatTime(null)` is not assignable to `iso: string`. The type checker is the red gate for this step; say so in the report rather than claiming a runtime red.

- [ ] **Step 3: Widen the helper signature**

In `web/src/lib/utils/time.ts`, change only the signature line:

```typescript
export function formatTime(iso: string | null, opts?: { seconds?: boolean }): string {
```

Body unchanged. Run `npm run check` → clean. `npm test -- src/lib/utils/time.test.ts` → all pass.

- [ ] **Step 4: Migrate the four byte-identical pages**

In each of `AgentDetailPage.svelte`, `PrincipalsPage.svelte`, `SecretsPage.svelte`, `ThreadDetailPage.svelte`: delete the entire local `formatTime` function and add to the script block's import section:

```typescript
import { formatTime } from '../utils/time.js';
```

(`.js` extension is required — Vite/ESM resolution for `.ts` modules.) Call sites are untouched: every existing call is the default no-seconds variant. Run `npm test` and `npm run check` → clean.

- [ ] **Step 5: Write the failing LinkPage test**

Create `web/src/lib/components/LinkPage.test.ts` (mirror the scaffolding — imports, cleanup, any fetch/SSE stubs — of `ThreadsPage.test.ts`):

```typescript
// ABOUTME: Renders LinkPage with fixture link codes and asserts expiry timestamp formatting.
// ABOUTME: Locks in the shared formatTime migration: date+time+seconds, em-dash when empty.
import { render, screen } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import LinkPage from './LinkPage.svelte';

const baseCode = {
  ID: 'lc1',
  Code: 'ABC123',
  Fingerprint: 'SHA256:abcdef1234567890abcd',
  DeviceName: 'Test Device',
  Status: 'pending',
  CreatedAt: '2026-08-23T10:00:00Z',
};

describe('LinkPage', () => {
  it('renders expiry as date + time with seconds', () => {
    render(LinkPage, {
      props: { codes: [{ ...baseCode, ExpiresAt: '2026-08-23T14:30:45Z' }], csrfToken: 't' },
    });
    expect(screen.getByText(/^[A-Z][a-z]{2} \d{2} \d{2}:\d{2}:\d{2}$/)).toBeTruthy();
  });

  it('renders em-dash for empty expiry', () => {
    render(LinkPage, {
      props: { codes: [{ ...baseCode, ID: 'lc2', ExpiresAt: '' }], csrfToken: 't' },
    });
    expect(screen.getAllByText('—').length).toBeGreaterThanOrEqual(1);
  });
});
```

Adjust ONLY the scaffolding (e.g. an `afterEach(cleanup)` or fetch stub) to match what `ThreadsPage.test.ts` actually does; the fixtures and assertions above are fixed. The date regex is TZ-agnostic by design (shape, not fixed values) — keep it that way.

- [ ] **Step 6: Verify LinkPage test fails for the right reason**

Run `npm test -- src/lib/components/LinkPage.test.ts` → the first test FAILS (current render is `"14:30:45"`-shaped — no date prefix, so the regex finds nothing). The em-dash test passes already. If the first test fails for a scaffolding reason (mount error) instead, fix the scaffolding until the failure is the missing date prefix.

- [ ] **Step 7: Migrate LinkPage**

In `LinkPage.svelte`: delete the local `formatTime` (lines ~34–38), add the shared import as in Step 4, and change the call site:

```svelte
{formatTime(code.ExpiresAt, { seconds: true })}
```

Run `npm test -- src/lib/components/LinkPage.test.ts` → both pass.

- [ ] **Step 8: Rename ChatMessage's local formatter**

In `ChatMessage.svelte`, rename `formatTime` → `formatClockTime` (definition ~line 30 and its single call site ~line 114) and put this comment directly above the function:

```typescript
// Chat bubbles deliberately show clock time only, in the system locale, from a Date —
// this is not the admin pages' "Mon DD HH:MM" format (web/src/lib/utils/time.ts).
```

No behavior change. `ChatMessage.test.ts` stays byte-unchanged and must still pass.

- [ ] **Step 9: Full verification**

From `web/`: `npm test` (expect 149+ tests across 25 files, zero failures, zero new warnings) and `npm run check` (0 errors, 0 warnings). From repo root: `make web` → clean production build. Confirm with `git diff --stat` that only the files this task names changed.

- [ ] **Step 10: Commit**

```bash
git add web/src/lib/utils/time.ts web/src/lib/utils/time.test.ts web/src/lib/components/AgentDetailPage.svelte web/src/lib/components/PrincipalsPage.svelte web/src/lib/components/SecretsPage.svelte web/src/lib/components/ThreadDetailPage.svelte web/src/lib/components/LinkPage.svelte web/src/lib/components/LinkPage.test.ts web/src/lib/components/ChatMessage.svelte
git commit -m "refactor: migrate remaining formatTime copies to shared helper

LinkPage expiry now renders date + time (was time-only) — deliberate:
an expiry without a date is ambiguous. ChatMessage's clock-time
formatter is a different function, not a copy; renamed formatClockTime."
```

---

### Task 2: E2E agent auth — capture setup token, feed fake-agent

**Files:**
- Modify: `web/e2e/chat.spec.ts` (token capture in `ensureAdminUser`, spawn args in `beforeAll`)
- Modify: `gotchas.md` (the fake-agent entry — document the new self-feeding harness + env override)

**Interfaces:**
- Consumes: `fake-agent -token <jwt>` (landed in PR #118, cmd/fake-agent/main.go) — appends `authorization: Bearer <jwt>` gRPC metadata.
- Produces: env contract `COVEN_E2E_AGENT_TOKEN` (optional override) — document in the spec's header comment and gotchas.md.

**Investigation facts (verified 2026-08-23):**
- Playwright has NO webServer/globalSetup: the gateway runs externally (HTTP :8080 per committed `baseURL`, gRPC :50051 hardcoded in chat.spec.ts). Suite is local-only — no CI impact.
- `chat.spec.ts` structure: module-level `setupDone` flag; `ensureAdminUser(page)` creates the first admin via `/setup` (fills username/display_name/password, submits, lands on a completion page); the "Chat with connected agent" `beforeAll` builds `bin/fake-agent` if missing and spawns it with `['-addr','localhost:50051','-name','Echo Agent','-id','e2e-echo-agent']`, awaiting `"registered as"` on stderr (10s timeout).
- Under enforced auth (jwt_secret set, fresh DB) today: fake-agent can't register → beforeAll times out → 1 failed + 2 blocked. A valid agent token requires an EXISTING principal (JWT `sub` → DB lookup, status approved/online/offline; no role needed for AgentStream) — so on a fresh DB the token can only exist AFTER first-time setup runs. Pre-minting is impossible; `coven-gateway bootstrap` would consume the /setup flow the tests themselves exercise.
- The setup flow: `handleSetupSubmit` (internal/webadmin/webadmin.go ~820) calls `createOwnerPrincipal` when the form's create-principal field is set, and `renderSetupComplete` (internal/webadmin/templates.go ~535) shows the API token when `hasToken`. Under `allow_anonymous` (no verifier) the token section is absent — PR #118 made that graceful.
- Serial mode within the file; but `fullyParallel: true` means OTHER spec files can win the /setup race in multi-file runs. Capture therefore only works when chat.spec performs first-time setup — acceptable; the env override covers every other shape. Document this.
- Setup form (both render paths): checkbox `name="create_principal"` defaults to CHECKED (`SetupForm.svelte:103-104`, `templates/setup.html:22`); the handler reads `r.FormValue("create_principal") == "on"` (webadmin.go:765). `ensureAdminUser` needs NO checkbox interaction — the default submit already requests the API principal.
- Token selector on the completion page: `[data-testid="api-token"]` already exists on the token `<pre>` in `SetupComplete.svelte:80` (the island playwright sees). Its `textContent` is the bare token. NO Go or template changes are needed anywhere in this task.

- [ ] **Step 1: Capture the token in ensureAdminUser**

In `web/e2e/chat.spec.ts`, add module state next to `setupDone` and extend `ensureAdminUser` — final shape:

```typescript
let setupDone = false;
let capturedAgentToken = '';

/** Create admin user via /setup if no users exist. Captures the API token when shown. */
async function ensureAdminUser(page: Page) {
  const resp = await page.goto('/setup', { waitUntil: 'domcontentloaded' });
  if (!resp) return;

  // If redirected to /login, setup is already done
  if (page.url().includes('/login')) return;

  // First-time setup — fill form and submit
  await page.fill('input[name="username"]', TEST_USER.username);
  await page.fill('input[name="display_name"]', TEST_USER.displayName);
  await page.fill('input[name="password"]', TEST_USER.password);
  await page.click('button[type="submit"]');

  // Setup renders a "complete" page (doesn't redirect). It also creates a session.
  await page.waitForLoadState('domcontentloaded');

  // Under enforced auth the completion page shows an API token (create_principal
  // defaults to checked) — capture it for fake-agent. Absent under allow_anonymous;
  // that path needs no token.
  const tokenEl = page.locator('[data-testid="api-token"]');
  if (await tokenEl.count()) {
    capturedAgentToken = (await tokenEl.first().textContent())?.trim() ?? '';
  }
}
```

Also update the spec's header comment (the `Requires:` block) to mention: enforced-auth gateways need either a fresh DB (token auto-captured) or `COVEN_E2E_AGENT_TOKEN` set.

- [ ] **Step 2: Feed the token to fake-agent**

In the "Chat with connected agent" `beforeAll`, make setup precede the spawn and thread the token in. Insert at the top of `beforeAll` (it currently starts with the binary check) — the hook signature gains `{ browser }`:

```typescript
test.beforeAll(async ({ browser }) => {
  // Setup must precede the agent spawn: under enforced auth the agent token
  // only exists after first-time setup creates a principal.
  if (!setupDone) {
    const page = await browser.newPage();
    await ensureAdminUser(page);
    await page.close();
    setupDone = true;
  }
  const agentToken = process.env.COVEN_E2E_AGENT_TOKEN || capturedAgentToken;
  // ... existing binary-build block unchanged ...
  fakeAgent = spawn(
    FAKE_AGENT_BIN,
    [
      '-addr', 'localhost:50051',
      '-name', 'Echo Agent',
      '-id', 'e2e-echo-agent',
      ...(agentToken ? ['-token', agentToken] : []),
    ],
    { cwd: PROJECT_ROOT, stdio: ['ignore', 'pipe', 'pipe'] },
  );
  // ... existing registration wait unchanged ...
});
```

The existing `beforeEach` blocks stay as they are (`ensureAdminUser` remains idempotent — already-done setup redirects to /login and returns).

- [ ] **Step 3: Verify both gateway shapes locally**

Per the Global Constraints e2e recipe (temp config on :9090, temp DB, uncommitted playwright config copy with matching `baseURL` and the gRPC addr passed through — note chat.spec.ts hardcodes `localhost:50051`; keep the temp gateway's gRPC on :50051 or adjust only via the temp setup, never the committed spec):

1. **Enforced auth:** fresh temp DB, gateway config with a real `jwt_secret` (32+ bytes), `allow_anonymous` absent. Run `npx playwright test e2e/chat.spec.ts --config=<temp>`. Expected: ALL tests in the file pass, including the 3 agent tests — this is the red→green proof (before this task, this exact shape fails with "fake-agent did not register in time").
2. **Anonymous:** fresh temp DB, `allow_anonymous: true`, empty jwt_secret. Same command. Expected: all pass (no token captured, none passed — current behavior preserved; the setup page's token section is absent, which the capture code treats as "no token").

Capture both run outputs in the report. Kill temp gateways and delete temp configs/DBs afterward.

- [ ] **Step 4: Update gotchas.md**

Rewrite the fake-agent entry's e2e sentence (the part about the 3 tests' config-dependent shape) to record the new truth — replace the clause from "The 3 \"Chat with connected agent\" e2e tests fail..." through "...neither shape is a regression signal (gap tracked in PR #116 follow-ups)." with:

```text
The chat e2e now self-feeds agent auth: first-time /setup captures the API token and passes it to fake-agent's -token; set COVEN_E2E_AGENT_TOKEN when reusing a DB where setup already ran (capture only works when chat.spec performs the first-time setup — other spec files can win the /setup race in multi-file runs).
```

Keep the rest of the entry (build note, panic note) intact.

- [ ] **Step 5: Full verification and commit**

`npm test` and `npm run check` from `web/` (unit suite untouched by this task — must stay green). Then:

```bash
git add web/e2e/chat.spec.ts gotchas.md
git commit -m "test: authenticate fake-agent in chat e2e via captured setup token

Fresh-DB enforced-auth runs now pass: ensureAdminUser captures the API
token from the setup completion page and beforeAll feeds it to
fake-agent -token. COVEN_E2E_AGENT_TOKEN overrides for reused DBs."
```

---

## Endgame

- Push `chore/formattime-e2e-token`, open a PR titled "Follow-ups round 2: formatTime consolidation + e2e agent auth".
- PR body: one line per task; link this plan; call out the two deliberate visible/behavioral changes (LinkPage expiry gains a date prefix; chat e2e now exercises enforced auth on fresh DBs) and the ChatMessage keep-local decision.
- The human merges.
