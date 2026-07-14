# Docker Frontend Assets Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make every clean Docker build embed the production Vite frontend so deployed pages never reference a developer's `localhost:5173`.

**Architecture:** A Node 22 build stage produces `web/dist`, then the Go build stage copies those files into `internal/assets/dist` before `go:embed` runs. A real-container regression script exercises `/login` and one referenced static asset, while a build-time manifest assertion prevents silent development-mode images.

**Tech Stack:** Docker BuildKit, Node.js 22, npm, Vite, Go 1.25, Bash, curl

---

### Task 1: Reproduce the Missing-Assets Image

**Files:**
- Create: `scripts/check-docker-assets.sh`

**Step 1: Write the failing end-to-end check**

Create this executable script:

```bash
#!/usr/bin/env bash
# ABOUTME: Builds and exercises the production container's embedded frontend assets.
# ABOUTME: Fails when login HTML uses Vite development URLs or serves missing bundles.

set -euo pipefail

IMAGE_TAG="${COVEN_DOCKER_TEST_IMAGE:-coven-gateway:frontend-assets-test}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/coven-docker-assets.XXXXXX")"
BUILD_LOG="$WORK_DIR/docker-build.log"
CONTAINER_LOG="$WORK_DIR/container.log"
container_id=""
passed=false

usage() {
    cat <<'USAGE'
Usage: scripts/check-docker-assets.sh

Builds the coven-gateway image, starts it with the example configuration,
and verifies that /login references a working embedded production bundle.

Environment:
  COVEN_DOCKER_TEST_IMAGE  Override the temporary image tag.
USAGE
}

cleanup() {
    if [[ -n "$container_id" ]]; then
        docker rm --force "$container_id" >/dev/null 2>&1 || true
    fi
    if [[ "$passed" == true ]]; then
        rm -rf "$WORK_DIR"
    else
        echo "Diagnostic logs: $WORK_DIR" >&2
    fi
}
trap cleanup EXIT

if [[ "${1:-}" == "--help" ]]; then
    usage
    exit 0
fi
if [[ $# -ne 0 ]]; then
    usage >&2
    exit 2
fi
for command in docker curl; do
    if ! command -v "$command" >/dev/null 2>&1; then
        echo "Required command not found: $command" >&2
        exit 1
    fi
done

cd "$ROOT_DIR"
echo "Building $IMAGE_TAG"
if ! docker build --load --tag "$IMAGE_TAG" . >"$BUILD_LOG" 2>&1; then
    tail -40 "$BUILD_LOG" >&2
    exit 1
fi

container_id="$(docker run --detach --rm \
    --publish 127.0.0.1::8080 \
    --env COVEN_JWT_SECRET='docker-assets-test-secret-at-least-32-bytes' \
    --volume "$ROOT_DIR/config.example.yaml:/app/config.yaml:ro" \
    "$IMAGE_TAG")"

port="$(docker port "$container_id" 8080/tcp | head -1 | awk -F: '{print $NF}')"
base_url="http://127.0.0.1:$port"
for _ in $(seq 1 40); do
    if curl --fail --silent "$base_url/health" >/dev/null; then
        break
    fi
    sleep 0.25
done

if ! login_html="$(curl --fail --silent --show-error "$base_url/login")"; then
    docker logs "$container_id" >"$CONTAINER_LOG" 2>&1 || true
    tail -40 "$CONTAINER_LOG" >&2
    exit 1
fi
if grep --quiet 'localhost:5173' <<<"$login_html"; then
    echo "Login page references the Vite development server" >&2
    exit 1
fi

asset_path="$(sed -n 's/.*<script type="module" src="\([^"]*\)".*/\1/p' <<<"$login_html" | head -1)"
if [[ "$asset_path" != /static/* ]]; then
    echo "Login page has no embedded production script" >&2
    exit 1
fi
curl --fail --silent --show-error "$base_url$asset_path" >/dev/null

passed=true
echo "Docker frontend assets are embedded and reachable"
```

Run: `chmod +x scripts/check-docker-assets.sh`

**Step 2: Run the check to verify it fails**

Run: `scripts/check-docker-assets.sh`

Expected: FAIL with `Login page references the Vite development server` because the current Dockerfile embeds only `internal/assets/dist/.gitkeep`.

**Step 3: Commit the regression check**

```bash
git add scripts/check-docker-assets.sh
git commit -m "test: reproduce missing Docker frontend assets"
```

### Task 2: Build the Frontend Before the Go Binary

**Files:**
- Modify: `Dockerfile:1-26`
- Modify: `.dockerignore`

**Step 1: Add a Node frontend build stage**

Insert before the Go builder:

```dockerfile
# Stage 1: Build frontend assets
FROM node:22-bookworm-slim AS web-builder

WORKDIR /app/web

COPY web/package.json web/package-lock.json ./
RUN npm ci --silent

COPY web/ ./
RUN npx tsx scripts/build-tokens.ts && npm run build
```

Rename the existing builder comment to `# Stage 2: Build Go binaries` and the runtime comment to `# Stage 3: Runtime`.

After the existing `COPY . .` in the Go builder, add:

```dockerfile
# Embed the production frontend in both Go binaries
COPY --from=web-builder /app/web/dist ./internal/assets/dist
RUN test -f internal/assets/dist/.vite/manifest.json
```

**Step 2: Exclude host frontend artifacts from the Docker context**

Add to `.dockerignore` under build artifacts:

```dockerignore
web/node_modules/
web/dist/
internal/assets/dist/
```

This prevents large or stale host artifacts from contaminating a clean, architecture-independent frontend build.

**Step 3: Run the end-to-end check to verify it passes**

Run: `scripts/check-docker-assets.sh`

Expected: PASS with `Docker frontend assets are embedded and reachable`.

**Step 4: Commit the Docker fix**

```bash
git add Dockerfile .dockerignore
git commit -m "fix: embed frontend assets in Docker image"
```

### Task 3: Complete Verification

**Files:**
- Verify only; no planned modifications

**Step 1: Run frontend tests and checks**

Run: `cd web && npm test && npm run check`

Expected: all Vitest tests pass and `svelte-check` reports zero errors and zero warnings.

**Step 2: Run Go verification**

Run: `go test -race ./... && go vet ./...`

Expected: all Go tests pass with no race reports, and `go vet` exits successfully with no output.

**Step 3: Re-run the container regression check from the final tree**

Run: `scripts/check-docker-assets.sh`

Expected: PASS with no failed HTTP requests and no development URLs.

**Step 4: Review the final diff**

Run: `git diff main...HEAD --check && git status --short && git diff main...HEAD -- Dockerfile .dockerignore scripts/check-docker-assets.sh`

Expected: only the approved design, implementation plan, regression script, Dockerfile, and `.dockerignore` are changed; `proto/coven-proto` and `coven-gateway.db.bak` remain uncommitted and excluded.
