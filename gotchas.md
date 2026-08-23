# Project Gotchas

- Docker image ownership: build and verify Docker images in cloud CI. Do not install local Docker builder plugins or build images on a developer machine unless Doctor Biz explicitly requests a local diagnostic build.
- CI scope: when an existing cloud release workflow already owns Docker builds, do not expand it into PR validation, permission hardening, or release redesign unless Doctor Biz explicitly asks for that work.
- Root-cause scope: when one build defect affects multiple published artifact paths, repair every affected publisher before declaring the bug releasable. A separate workflow is not a separate bug.
- Playwright e2e locally: :8080 is usually held by an unrelated `agentsview` process. Copy playwright.config.ts to an uncommitted temp config with the gateway on :9090 (temp DB/config in /tmp), run with `--config`, delete after. Never edit the committed config.
- fake-agent has no auth (no SSH/JWT) but the gateway requires it, so the 3 "Chat with connected agent" e2e tests fail on every branch including main. Known gap (PR #116 follow-ups) — not a regression signal. Build it with `mise exec -- go build`.
- Frontend deps are exact-pinned with a written policy (CLAUDE.md, Frontend Dependency Policy). Never bump versions in passing — a bump is its own commit with check+test+make-web green and a one-line reason. Known open advisory: vitest 4.0.18 / GHSA-5xrq-8626-4rwp, fix = 4.1.x bump.
- Agent-orchestration harness here has no SendMessage tool (despite Agent tool results naming it) — dispatched subagents cannot be resumed; carry findings into a fresh dispatch instead.
