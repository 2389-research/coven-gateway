# Frontend Verification Repair Design

## Problem

The Docker frontend-asset fix works, but the repository cannot be handed off with pristine verification. `npm run check` reports 83 errors, `npm test` prints intentional warning output, and running the frontend checks with the machine's Node 26 installation emits Node's `DEP0205` runtime deprecation. CI and the Docker build already use Node 22.

The type-check failures have four confirmed causes:

- All 31 Storybook files use component instance types where Storybook expects component constructor types, causing most of the errors and their invalid-prop cascades.
- Node-backed token scripts lack Node type declarations, and the Vitest setup file uses an unimported test global.
- A small set of Svelte components contain genuine contract errors: an invalid SSE status comparison, null narrowing that is lost across a nested snippet, unsupported `Alert` props, and unused state or imports.
- Token utility tests deliberately trigger warnings but do not capture and assert them.

## Scope

Restore pristine frontend verification while preserving the existing Docker asset fix. Do not upgrade the frontend framework or build-tool stack, exclude files from type-checking, weaken compiler rules, or change unrelated application behavior.

## Runtime Design

Add a repository-level `mise.toml` that selects Node 22. This gives local development the same Node major already selected by both GitHub Actions and the Docker `node:22-bookworm-slim` build stage. Keep the existing CI and Docker declarations explicit because those systems do not consume the local mise configuration.

The configuration file will include the required two-line `ABOUTME` header. Frontend verification will run through mise so the current interactive shell's Node 26 installation cannot reintroduce `DEP0205`.

## Type-Contract Repairs

Update every Storybook component metadata declaration from `Meta<Component>` to `Meta<typeof Component>`. Stories that declare `StoryObj` directly from a component will instead derive it from the metadata object with `StoryObj<typeof meta>`. These are the Storybook-supported constructor and inferred-args contracts; story behavior and rendered examples remain unchanged.

Add `@types/node` for the supported Node 22 major so the token-building scripts receive declarations for `node:fs`, `node:path`, `process`, and `import.meta.dirname`. Import `beforeEach` directly from Vitest in the global setup file rather than broadening TypeScript globals.

Repair the genuine Svelte errors at their source:

- Compare the chat stream's status with its real connected value, `open`, so the status indicator reflects the stream contract.
- Bind the non-null selected board thread to a block-local constant inside the existing conditional and have nested snippets read that single value.
- Remove redundant `role="alert"` call-site props because `Alert` already owns role selection according to its variant.
- Remove only state, variables, and imports proven unused by the type-checker.

No compatibility layer or duplicated state will be added.

## Diagnostic Output

The token utilities will retain their real warning behavior. Tests that deliberately exercise unresolved or cyclic token references will intercept the emitted warnings, assert their exact expected content, and restore `console.warn` after each assertion. Unexpected output will continue to reach the test runner and fail the pristine-output requirement.

## Test-Driven Implementation

The existing `npm run check` failure is the red regression test for the type-contract repairs. Before changing runtime behavior, add or strengthen focused tests for the connected chat status and any other behavior not already covered. For warning-output cleanup, first add assertions that expose the uncaptured warnings, then make the capture pass without changing production logging.

Implementation will be divided into independently reviewable commits:

1. Declare and verify the Node 22 development runtime.
2. Correct Storybook and tooling type contracts.
3. Repair genuine component contracts with focused tests.
4. Capture and assert expected token warnings.

## Success Criteria

From the isolated task worktree:

- `mise exec -- npm run check` in `web/` exits successfully with no warnings or errors.
- `mise exec -- npm test` in `web/` exits successfully with no unexpected stdout or stderr.
- `mise exec -- npm run build` in `web/` exits successfully with no warnings or errors.
- `go test -race ./...` and `go vet ./...` exit successfully with pristine output.
- `scripts/check-docker-assets.sh` builds and exercises the real image successfully, confirming the original Docker regression remains fixed.
- A fresh-eyes review finds no disabled checks, excluded files, unrelated refactors, or uncommitted changes.

## Rejected Alternatives

Upgrading Vite, Tailwind, Storybook, and related dependencies for Node 26 would make the change substantially larger and introduce unrelated migration risk. Excluding stories, scripts, or tests from TypeScript would conceal broken contracts and reduce coverage. Both approaches conflict with the requirement to make the smallest root-cause repair.
