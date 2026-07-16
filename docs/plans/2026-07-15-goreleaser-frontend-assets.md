# GoReleaser Frontend Assets Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Ensure every GoReleaser archive embeds the production Vite frontend before `v0.6.1` is published.

**Architecture:** GitHub Actions selects Node 22 and invokes the existing `make web` build recipe before GoReleaser. A focused Go contract test protects the required workflow ordering, and a snapshot release is exercised as a real server before tagging.

**Tech Stack:** GitHub Actions, Go 1.25, Node.js 22, npm, Vite, GoReleaser 2, Zig, Bash, curl

---

### Task 1: Protect Release Build Ordering

**Files:**

- Create: `internal/contract/release_workflow_test.go`

**Step 1: Write the failing test**

Add `TestReleaseWorkflowBuildsFrontendBeforeGoReleaser`, which reads `.github/workflows/release.yml` and requires all of the following in order:

1. `Set up Node.js` with `node-version: '22'`.
2. `Build frontend assets` with `run: make web`.
3. `Run GoReleaser`.

**Step 2: Verify the red state**

Run:

```bash
go test ./internal/contract -run TestReleaseWorkflowBuildsFrontendBeforeGoReleaser -count=1
```

Expected: FAIL because the release workflow contains neither Node setup nor `make web`.

**Step 3: Commit the regression test**

Use `@fresh-eyes-review`, then commit with:

```bash
git add internal/contract/release_workflow_test.go
git commit -m "test: require frontend assets in releases"
```

### Task 2: Build Frontend Assets Before GoReleaser

**Files:**

- Modify: `.github/workflows/release.yml`

**Step 1: Add the minimal workflow steps**

After Go setup, add `actions/setup-node@v4` configured for Node 22 and the locked npm cache. Immediately before GoReleaser, add `run: make web`.

**Step 2: Verify the green state**

Run:

```bash
go test ./internal/contract -run TestReleaseWorkflowBuildsFrontendBeforeGoReleaser -count=1
```

Expected: PASS with pristine output.

**Step 3: Commit the workflow fix**

Use `@fresh-eyes-review`, then commit with:

```bash
git add .github/workflows/release.yml
git commit -m "fix: embed frontend assets in releases"
```

### Task 3: Verify Real Release Artifacts

**Files:**

- Verify only

**Step 1: Build the frontend**

Run `make web` under Node 22 and confirm generated token sources remain unchanged.

**Step 2: Build a snapshot release**

Run:

```bash
goreleaser release --snapshot --clean
```

Expected: all configured Darwin and Linux archives build without publishing.

**Step 3: Exercise the native release binary**

Extract the Darwin ARM64 archive, start its `coven-gateway` with a disposable configuration and database, fetch `/login`, reject Vite development URLs, and fetch the referenced `/static/` JavaScript asset successfully.

**Step 4: Run complete checks**

Run frontend type checks and unit tests, `go test -race ./...`, both Go builds, all pre-commit hooks, and `git diff --check`.

### Task 4: Integrate and Release

**Step 1: Fresh-eyes review the complete branch diff**

Verify the change remains limited to release build ordering, its regression test, required documentation, and correction journal.

**Step 2: Merge and push `main`**

Fast-forward local `main`, rerun post-merge checks, push, and wait for GitHub CI to pass.

**Step 3: Publish `v0.6.1`**

Create the same tag type used by `v0.6.0`, push it, and wait for both Docker and Release workflows. Verify the GHCR image and published native archive serve production assets before declaring the bug fixed.
