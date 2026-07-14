# Docker Frontend Assets Design

## Problem

Clean Docker builds compile `coven-gateway` without first building the Vite frontend. The Go embed directory therefore contains only its placeholder, `assets.Manifest` remains nil at runtime, and production pages emit development URLs for `http://localhost:5173`.

## Scope

Fix the Docker image build only. Preserve local Vite development behavior and leave the separate GoReleaser pipeline unchanged.

## Design

Add a dedicated Node 22 build stage to the Dockerfile. It will install the locked web dependencies, generate CSS from the design tokens, and run the Vite production build. The existing Go builder will copy that stage's `web/dist` directory into `internal/assets/dist` after copying the repository and before compiling either Go binary.

The runtime image remains unchanged. Node, npm, source files, and frontend build dependencies stay in build stages and do not increase the production image contents beyond the assets already embedded in the Go binary.

## Failure Handling

The Docker build must fail if dependency installation, token generation, or Vite compilation fails. It must also explicitly verify that `.vite/manifest.json` exists before compiling Go, preventing the current silent fallback to development mode.

## Verification

Add an automated regression check that builds the image from a clean source context, starts the real container, and verifies:

- `/login` contains same-origin `/static/` asset URLs.
- `/login` does not contain `localhost:5173`.
- A referenced bundled asset returns successfully.
- The existing Go and frontend checks remain clean.
