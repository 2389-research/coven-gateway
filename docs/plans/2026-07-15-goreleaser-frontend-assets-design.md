# GoReleaser Frontend Assets Design

## Problem

GoReleaser compiles `coven-gateway` without first replacing the development-only embedded asset directory with a Vite production build. Published archives therefore serve HTML that points browsers at `localhost:5173`.

## Decision

The release workflow will select Node 22 and run the repository's existing `make web` source of truth immediately before GoReleaser. `make web` installs locked frontend dependencies, generates tokens, builds Vite assets, and copies `web/dist` into `internal/assets/dist`; GoReleaser then embeds those files in every target binary.

The workflow will not duplicate frontend commands inside `.goreleaser.yml`, and generated frontend output will not be committed. Those alternatives would create a second build recipe or make stale generated files releasable.

## Verification

A Go contract test will require Node 22 setup and `make web` to appear before the GoReleaser action. TDD must demonstrate that this contract fails against the current workflow and passes after the minimal workflow edit.

Before publishing, a GoReleaser snapshot will build all configured archives. The native archive's binary will be started with a disposable configuration, and `/login` plus its referenced `/static/` JavaScript asset will be fetched to prove the release artifact—not merely the source tree—serves production assets.

No Docker workflow, release permissions, action versions, or artifact formats change in this repair.
