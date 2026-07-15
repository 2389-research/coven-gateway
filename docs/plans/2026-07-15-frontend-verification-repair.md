# Frontend Verification Repair Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make frontend type-checking, unit tests, production builds, and real browser verification pristine under the repository's supported Node 22 runtime while preserving the Docker asset fix.

**Architecture:** Keep CI and Docker on their existing Node 22 declarations and add a repository-local mise declaration for the same major. Repair Storybook constructor types and genuine Svelte contracts at their sources, retain production warning behavior while capturing it in tests, and verify the chat status fix through both the real Svelte/SSE stack and a real gateway-plus-agent browser flow.

**Tech Stack:** mise, Node.js 22, npm, TypeScript, Svelte 5, Storybook 10, Vitest, Testing Library, Playwright, Go, Docker

---

## Baseline

Run commands from the isolated worktree:

```text
/Users/harper/Public/src/2389/coven-projects/coven-gateway/.worktrees/fix-docker-frontend-assets
```

The confirmed Node 22 baseline is:

- `npm run check`: exits 1 with 83 errors, 0 warnings, and 41 files.
- Storybook contracts: 61 errors across all 31 story files.
- Node/Vitest tooling types: 7 errors.
- Svelte component contracts: 14 errors.
- Token test dead assignment: 1 error.
- `npm test`: 104 tests pass, but 6 intentional token warnings escape across 4 tests.
- Node 26 adds `DEP0205` through the current Tailwind/Vite dependency path; CI and Docker already use Node 22.

### Task 1: Declare Node 22 for Local Development

**Files:**

- Create: `mise.toml`

**Step 1: Verify the missing repository runtime declaration**

Run from the worktree root:

```bash
mise exec -- node --version
```

Expected before the file exists: `v26.5.0`, inherited from the machine rather than the repository.

**Step 2: Create the minimal mise configuration**

Create `mise.toml` with exactly:

```toml
# ABOUTME: Selects tool versions used for local coven-gateway development.
# ABOUTME: Keeps frontend commands on the same Node major as CI and Docker.

[tools]
node = "22"
```

If mise reports that the new file is untrusted, run `mise trust mise.toml`. Trust state is machine-local and must not be committed.

**Step 3: Verify the runtime changes**

Run:

```bash
mise exec -- node --version
```

Expected: installed Node `v22.x` rather than Node 26.

Run from `web/`:

```bash
mise exec -- npm run check
```

Expected: the known 83 type errors remain, but no `DEP0205` warning is printed. This proves runtime selection is fixed independently of source repairs.

**Step 4: Review and commit**

Use `@fresh-eyes-review`, then run:

```bash
git status --short
git add mise.toml
git commit -m "chore: standardize frontend on Node 22"
```

### Task 2: Restore Node and Vitest Tooling Types

**Files:**

- Modify: `web/package.json`
- Modify: `web/package-lock.json`
- Modify: `web/test/setup.ts`

**Step 1: Confirm the focused type failures are red**

From `web/`, run:

```bash
mise exec -- npm run check > /tmp/coven-tooling-types-red.log 2>&1
rg -n 'build-tokens.ts:|test/setup.ts:' /tmp/coven-tooling-types-red.log
```

Expected: six errors in `scripts/build-tokens.ts` for Node APIs and one unimported `beforeEach` error in `test/setup.ts`.

**Step 2: Install declarations for the supported Node major**

Run from `web/`:

```bash
mise exec -- npm install --save-dev '@types/node@^22'
```

Expected: only `package.json` and `package-lock.json` change; `@types/node` stays on major 22. Do not add or change `svelte-check`, which is already a direct dependency.

**Step 3: Make the Vitest dependency explicit**

Prepend the required source header to `web/test/setup.ts`, retain the existing JSDoc, and import the test hook:

```ts
// ABOUTME: Configures browser API test boundaries shared by frontend unit tests.
// ABOUTME: Resets controllable EventSource instances before every Vitest case.

/**
 * Vitest global setup — runs before every test file.
 * Provides browser API mocks that jsdom doesn't include.
 */

import { beforeEach } from 'vitest';
```

Do not broaden `tsconfig.json` with global Vitest or Node type lists. The direct import and normal `@types` discovery are sufficient.

**Step 4: Verify only unrelated failures remain**

Run:

```bash
mise exec -- npm run check > /tmp/coven-tooling-types-green.log 2>&1
rg -n 'build-tokens.ts:|test/setup.ts:' /tmp/coven-tooling-types-green.log
```

Expected: `npm run check` still exits 1 because later tasks are red, but `rg` prints no matches. The summary should report 76 errors.

Run:

```bash
mise exec -- npm test
```

Expected: 104 tests pass. The known token warnings remain until Task 5.

**Step 5: Review and commit**

Use `@fresh-eyes-review`, then run:

```bash
git status --short
git add web/package.json web/package-lock.json web/test/setup.ts
git commit -m "fix: restore frontend tooling types"
```

### Task 3: Correct Storybook Component Contracts

**Files:**

- Modify: `web/src/lib/components/AdminLayout.stories.ts`
- Modify: `web/src/lib/components/AgentList.stories.ts`
- Modify: `web/src/lib/components/Alert.stories.ts`
- Modify: `web/src/lib/components/AppShell.stories.ts`
- Modify: `web/src/lib/components/Badge.stories.ts`
- Modify: `web/src/lib/components/Breadcrumb.stories.ts`
- Modify: `web/src/lib/components/Button.stories.ts`
- Modify: `web/src/lib/components/Card.stories.ts`
- Modify: `web/src/lib/components/ChatInput.stories.ts`
- Modify: `web/src/lib/components/ChatMessage.stories.ts`
- Modify: `web/src/lib/components/ChatThread.stories.ts`
- Modify: `web/src/lib/components/CodeText.stories.ts`
- Modify: `web/src/lib/components/ConnectionBadge.stories.ts`
- Modify: `web/src/lib/components/CopyButton.stories.ts`
- Modify: `web/src/lib/components/Dialog.stories.ts`
- Modify: `web/src/lib/components/EmptyState.stories.ts`
- Modify: `web/src/lib/components/IconButton.stories.ts`
- Modify: `web/src/lib/components/LoginForm.stories.ts`
- Modify: `web/src/lib/components/RevealField.stories.ts`
- Modify: `web/src/lib/components/Select.stories.ts`
- Modify: `web/src/lib/components/SidebarNav.stories.ts`
- Modify: `web/src/lib/components/Spinner.stories.ts`
- Modify: `web/src/lib/components/Stack.stories.ts`
- Modify: `web/src/lib/components/StatusDot.stories.ts`
- Modify: `web/src/lib/components/Table.stories.ts`
- Modify: `web/src/lib/components/Tabs.stories.ts`
- Modify: `web/src/lib/components/TextArea.stories.ts`
- Modify: `web/src/lib/components/TextField.stories.ts`
- Modify: `web/src/lib/components/ThinkingIndicator.stories.ts`
- Modify: `web/src/lib/components/Toast.stories.ts`
- Modify: `web/src/lib/components/ToolCallView.stories.ts`

**Step 1: Confirm Storybook contracts are red**

From `web/`, run:

```bash
mise exec -- npm run check > /tmp/coven-story-types-red.log 2>&1
rg -n '\.stories\.ts:' /tmp/coven-story-types-red.log
```

Expected: 61 errors from component metadata, cascading props, and two unused imports.

**Step 2: Add the required source headers**

Each touched story file must begin with two `// ABOUTME:` lines naming its component and explaining that the file supplies Storybook variants/controls. Preserve all existing comments and story behavior.

Example:

```ts
// ABOUTME: Defines Storybook examples for the Button component.
// ABOUTME: Exercises Button variants and controls for visual review.
```

**Step 3: Repair the metadata constructor types**

In 29 files, change only:

```ts
} satisfies Meta<Component>;
```

to:

```ts
} satisfies Meta<typeof Component>;
```

Use the actual imported component identifier in each file, including `_BreadcrumbDemo` and `TableDemo`. Keep their existing `type Story = StoryObj<typeof meta>` declarations.

In `AgentList.stories.ts`, use:

```ts
const meta: Meta<typeof AgentList> = {
  // existing metadata remains unchanged
};

export default meta;
type Story = StoryObj<typeof meta>;
```

In `LoginForm.stories.ts`, use:

```ts
const meta: Meta<typeof LoginForm> = {
  // existing metadata remains unchanged
};

export default meta;
type Story = StoryObj<typeof meta>;
```

**Step 4: Remove only the proven unused imports**

Delete `createRawSnippet` from `Breadcrumb.stories.ts`. Change the helper import in `EmptyState.stories.ts` to:

```ts
import { htmlSnippet } from './_storyHelpers';
```

**Step 5: Verify the Storybook error class is gone**

Run:

```bash
mise exec -- npm run check > /tmp/coven-story-types-green.log 2>&1
rg -n '\.stories\.ts:' /tmp/coven-story-types-green.log
rg --pcre2 'Meta<(?!typeof )|StoryObj<(AgentList|LoginForm)>' src/lib/components -g '*.stories.ts'
```

Expected: both searches print no matches. The type-check summary should now contain only 15 errors in the component and token-test files.

**Step 6: Review and commit**

Use `@fresh-eyes-review`, then run:

```bash
git status --short
git add web/src/lib/components/*.stories.ts
git commit -m "fix: correct Storybook component types"
```

### Task 4: Repair Svelte Component Contracts with Real Behavior Coverage

**Files:**

- Modify: `web/test/setup.ts`
- Create: `web/src/lib/components/ChatApp.test.ts`
- Create: `web/src/lib/components/Alert.test.ts`
- Modify: `web/e2e/chat.spec.ts`
- Modify: `web/src/lib/components/BoardPage.svelte`
- Modify: `web/src/lib/components/ChatApp.svelte`
- Modify: `web/src/lib/components/InviteForm.svelte`
- Modify: `web/src/lib/components/LoginForm.svelte`
- Modify: `web/src/lib/components/PrincipalsPage.svelte`
- Modify: `web/src/lib/components/SecretsPage.svelte`
- Modify: `web/src/lib/components/SetupForm.svelte`

**Step 1: Export the existing EventSource test boundary**

In `web/test/setup.ts`, change only the existing class declaration:

```ts
export class MockEventSource {
```

The class remains installed as `globalThis.EventSource`; no production mock mode is introduced.

**Step 2: Write the failing ChatApp unit regression**

Create `ChatApp.test.ts`:

```ts
// ABOUTME: Verifies ChatApp renders connection state from the real chat SSE store.
// ABOUTME: Drives the browser EventSource boundary while asserting rendered user behavior.

import { render, screen, waitFor } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import { MockEventSource } from '../../../test/setup';
import ChatApp from './ChatApp.svelte';

describe('ChatApp', () => {
  it('shows the agent online when its SSE stream opens', async () => {
    render(ChatApp, {
      props: {
        agentId: 'agent-1',
        agentName: 'Agent One',
        csrfToken: 'csrf-token',
      },
    });

    expect(screen.getByTestId('status-dot').getAttribute('aria-label')).toBe('Offline');

    const source = MockEventSource._last();
    expect(source).toBeDefined();
    source!.simulateOpen();

    await waitFor(() => {
      expect(screen.getByTestId('status-dot').getAttribute('aria-label')).toBe('Online');
    });
  });
});
```

Run from `web/`:

```bash
mise exec -- npm test -- src/lib/components/ChatApp.test.ts
```

Expected: FAIL because the real stream status becomes `open` while `ChatApp` compares it with `connected`.

**Step 3: Add Alert ownership characterization tests**

Create `Alert.test.ts`:

```ts
// ABOUTME: Verifies Alert owns accessible live-region roles for every variant.
// ABOUTME: Protects form callers from duplicating or overriding Alert semantics.

import { render, screen } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { describe, expect, it } from 'vitest';
import Alert from './Alert.svelte';

const children = createRawSnippet(() => ({
  render: () => '<span>Message</span>',
}));

describe('Alert', () => {
  it('uses alert semantics for danger messages', () => {
    render(Alert, { props: { variant: 'danger', children } });
    expect(screen.getByRole('alert').textContent).toContain('Message');
  });

  it('uses status semantics for informational messages', () => {
    render(Alert, { props: { variant: 'info', children } });
    expect(screen.getByRole('status').textContent).toContain('Message');
  });
});
```

Run:

```bash
mise exec -- npm test -- src/lib/components/Alert.test.ts
```

Expected: PASS, characterizing the existing single owner of role selection. The caller type errors remain red.

**Step 4: Strengthen the real browser regression**

Add the required two-line `ABOUTME:` header before the existing documentation in `web/e2e/chat.spec.ts`. In `select agent from sidebar`, after asserting the `Echo Agent` heading, add:

```ts
const chatHeaderStatus = page
  .getByRole('heading', { name: 'Echo Agent' })
  .locator('..')
  .getByTestId('status-dot');
await expect(chatHeaderStatus).toHaveAttribute('aria-label', 'Online', { timeout: 5000 });
```

Build the current red image and start it against fresh container storage:

```bash
docker build --tag coven-gateway:chat-status-red .
docker run --detach --name coven-chat-status-red --publish 127.0.0.1:8080:8080 --publish 127.0.0.1:50051:50051 --env COVEN_JWT_SECRET='chat-status-e2e-secret-at-least-32-bytes' --volume "$PWD/config.example.yaml:/app/config.yaml:ro" coven-gateway:chat-status-red
```

From `web/`, run:

```bash
mise exec -- npx playwright test e2e/chat.spec.ts --project=chromium --grep 'select agent from sidebar'
```

Expected: FAIL because the header remains `Offline` after the real fake agent opens the gateway SSE stream.

Always clean up the red container from the worktree root:

```bash
docker rm --force coven-chat-status-red
```

**Step 5: Commit the red behavior regressions**

Use `@fresh-eyes-review`, then run:

```bash
git status --short
git add web/test/setup.ts web/src/lib/components/ChatApp.test.ts web/src/lib/components/Alert.test.ts web/e2e/chat.spec.ts
git commit -m "test: reproduce chat connection status mismatch"
```

**Step 6: Implement the minimal component repairs**

Add required two-line `ABOUTME:` headers to `BoardPage.svelte`, `ChatApp.svelte`, `PrincipalsPage.svelte`, and `SecretsPage.svelte`. The three form files already have compliant headers; preserve them.

In `BoardPage.svelte`, bind the narrowed value once inside the existing conditional:

```svelte
{#if selectedThread}
  {@const thread = selectedThread}
```

Replace only detail-view `selectedThread` reads inside that block with `thread`. Do not create new state.

In `ChatApp.svelte`, use the actual `SSEStatus` connected value:

```svelte
<StatusDot status={chat.status === 'open' ? 'online' : 'offline'} />
```

In `InviteForm.svelte`, `LoginForm.svelte`, and `SetupForm.svelte`, remove only the unsupported `role="alert"` prop from danger `Alert` calls. `Alert.svelte` continues to render the role.

In `PrincipalsPage.svelte` and `SecretsPage.svelte`, delete unused `loading` state and its assignments. Simplify each `refresh()` to its existing fetch/`res.ok`/JSON body without the now-empty `try/finally`; rejected fetches continue to propagate as before.

**Step 7: Verify focused unit and static contracts pass**

From `web/`, run:

```bash
mise exec -- npm test -- src/lib/components/ChatApp.test.ts src/lib/components/Alert.test.ts src/lib/components/LoginForm.test.ts
mise exec -- npm run check
```

Expected: component tests pass. Type-checking reports only the one unused `result` assignment in `scripts/build-tokens.test.ts`.

**Step 8: Verify the real browser regression passes**

Rebuild and start the fixed image:

```bash
docker build --tag coven-gateway:chat-status-green .
docker run --detach --name coven-chat-status-green --publish 127.0.0.1:8080:8080 --publish 127.0.0.1:50051:50051 --env COVEN_JWT_SECRET='chat-status-e2e-secret-at-least-32-bytes' --volume "$PWD/config.example.yaml:/app/config.yaml:ro" coven-gateway:chat-status-green
```

From `web/`, run:

```bash
mise exec -- npx playwright test e2e/chat.spec.ts --project=chromium --grep 'select agent from sidebar'
```

Expected: PASS against the real gateway, SQLite database, browser, SSE stream, and compiled fake agent.

Always clean up:

```bash
docker rm --force coven-chat-status-green
```

**Step 9: Review and commit the implementation**

Use `@fresh-eyes-review`, then run:

```bash
git status --short
git add web/src/lib/components/BoardPage.svelte web/src/lib/components/ChatApp.svelte web/src/lib/components/InviteForm.svelte web/src/lib/components/LoginForm.svelte web/src/lib/components/PrincipalsPage.svelte web/src/lib/components/SecretsPage.svelte web/src/lib/components/SetupForm.svelte
git commit -m "fix: repair frontend component contracts"
```

### Task 5: Capture and Assert Intentional Token Warnings

**Files:**

- Modify: `web/scripts/build-tokens.test.ts`

**Step 1: Add assertions without suppressing output**

Import `afterEach` and `vi`, then add:

```ts
afterEach(() => {
  vi.restoreAllMocks();
});
```

In only the four tests that intentionally produce warnings, first create the spy without a no-op implementation:

```ts
const warn = vi.spyOn(console, 'warn');
```

Assert exact ordered `warn.mock.calls`:

```ts
[['Unresolved token reference: {color.missing}']]
```

```ts
[['Cyclic token reference detected: {color.a}']]
```

```ts
[
  ['Unresolved token reference: {a.missing}'],
  ['Unresolved token reference: {b.missing}'],
]
```

```ts
[
  ['Unresolved token reference: {nonexistent}'],
  ['Unresolved token reference: {also.missing}'],
]
```

Also strengthen the cyclic test with:

```ts
expect(result).toBe('{color.a}');
```

Run from `web/`:

```bash
mise exec -- npm test -- scripts/build-tokens.test.ts
```

Expected: assertions pass, but the six warnings still print. The pristine-output acceptance criterion remains red.

**Step 2: Capture the expected output**

In those same four tests, change each spy to:

```ts
const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
```

This intercepts the real warning side effect only where the test asserts it. Extra or reordered warnings fail exact call-array assertions; warnings from every other test remain visible.

**Step 3: Verify focused tests and type-checking are clean**

Run:

```bash
mise exec -- npm test -- scripts/build-tokens.test.ts
mise exec -- npm run check
```

Expected: the token tests have no warning blocks, and `svelte-check` reports 0 errors and 0 warnings.

**Step 4: Review and commit**

Use `@fresh-eyes-review`, then run:

```bash
git status --short
git add web/scripts/build-tokens.test.ts
git commit -m "test: assert token resolution warnings"
```

### Task 6: Run Complete Verification and Record It

**Files:**

- Modify: `docs/plans/2026-07-15-frontend-verification-repair.md`

**Step 1: Run all frontend static and unit checks under Node 22**

From `web/`, run each command independently and preserve full logs under `/tmp` if any output is not pristine:

```bash
mise exec -- npm run check
mise exec -- npm test
mise exec -- npm run build
mise exec -- npm run storybook:build
```

Expected: all exit 0 with no warnings, deprecations, failed tests, or unexpected console output.

**Step 2: Run Go verification**

From the worktree root:

```bash
go test -race ./...
go vet ./...
```

Expected: both exit 0 with no failures or diagnostics.

**Step 3: Re-run the real Docker regression**

Run:

```bash
scripts/check-docker-assets.sh
```

Expected: the real container becomes healthy, `/login` or `/setup` uses same-origin embedded JavaScript, all Vite development markers are absent, and the referenced asset returns JavaScript successfully.

**Step 4: Inspect repository integrity**

Run:

```bash
git diff --check
git status --short --branch
git log --oneline --decorate -12
```

Expected: no whitespace errors and no uncommitted implementation changes before recording verification.

**Step 5: Record exact verification evidence**

Append an execution record to this plan containing each command, exit status, frontend test counts, type-check totals, browser result, Docker result, and any retained `/tmp` log paths. Do not describe a command as passing unless it was freshly run.

**Step 6: Run independent final reviews**

Use `@fresh-eyes-review` and `@superpowers:requesting-code-review`. Resolve every finding at its root and repeat affected verification before proceeding.

The final review must explicitly provide both views:

- Perfectionist: remaining maintainability, coverage, or environmental concerns.
- Pragmatist: whether the smallest approved repair is safe to integrate now.

**Step 7: Commit the verification record**

Run:

```bash
git status --short
git add docs/plans/2026-07-15-frontend-verification-repair.md
git commit -m "docs: record frontend verification"
```

**Step 8: Prepare the branch handoff**

Use `@superpowers:verification-before-completion` and `@superpowers:finishing-a-development-branch`. Do not merge or push without Doctor Biz's explicit direction.
