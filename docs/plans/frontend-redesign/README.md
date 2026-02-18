# Frontend Redesign — Implementation Plans

**Design Document:** [`../2026-02-17-frontend-redesign-design.md`](../2026-02-17-frontend-redesign-design.md)
**Status:** Planning complete, ready for Phase 1 execution

## Guiding Principles

This plan is designed for **adaptive execution**. Each phase has concrete entry/exit gates, but the plan accounts for the reality that earlier phases may be implemented differently than originally specified. Every phase gate includes a **drift assessment** — a structured checkpoint to evaluate how actual implementation diverged from plan and what downstream adjustments are needed.

**Key constraints validated by multi-model expert consensus (GPT-5-Codex, GPT-5, O3):**

- Islands architecture is a sound intermediate *and potentially permanent* state — SPA is not assumed as the end goal
- Single-binary via `go:embed` is a hard requirement that rules out SSR and Node-dependent tooling
- Scope must stay ruthlessly trimmed — build only what the next phase needs, not the full component inventory
- Phase gates must be measurable deliverables, not percentage estimates
- The current codebase (35 Go templates, HTMX 1.9.10, CDN Tailwind, SSE chat) is the starting point

## Phase Sequence

| Phase | Name | Weeks | Depends On |
|-------|------|-------|------------|
| [1](phase-1-foundation.md) | Foundation — Prove the Pipeline | 1–2 | — |
| [2](phase-2-design-system.md) | Design System Core — Build What Chat Needs | 3–4 | Phase 1 gate |
| [3](phase-3-chat-migration.md) | Chat Migration — Highest User Value | 5–7 | Phase 2 gate |
| [4](phase-4-dashboard-admin.md) | Dashboard & Admin Pages | 8–10 | Phase 3 gate |
| [5](phase-5-login-auth.md) | Login & Auth | 11 | Phase 4 gate |
| [6](phase-6-spa-evaluation.md) | Evaluate SPA Transition | 12+ | Phase 5 gate |

Cross-cutting concerns: [Cross-Phase Risks & Budgets](cross-phase-risks.md)

## How to Use These Plans

1. **Start with Phase 1.** Don't read ahead until you've passed the Phase 1 gate.
2. **At each gate, do a drift assessment** (see [cross-phase-risks.md](cross-phase-risks.md#handling-implementation-drift)). Evaluate what actually happened vs. what was planned. Update the *next* phase's plan before starting it.
3. **Each phase file is self-contained.** It has deliverables, exit gates, and drift adaptation tables. You don't need the design doc open while working.
4. **The design doc is reference material.** Consult it for token definitions, component APIs, build pipeline details, and architecture decisions.

## Executing with Claude Code

See **[RUNBOOK.md](RUNBOOK.md)** for session management strategy: prompt templates for each session type, skill usage, and rules for avoiding context smearing across sessions.
