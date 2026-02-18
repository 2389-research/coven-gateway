# Phase 6: Evaluate SPA Transition

**Weeks:** 12+
**Depends on:** [Phase 5 gate](phase-5-login-auth.md#exit-gate)

> **Critical reframing:** This phase is an *evaluation*, not an assumed transition. Three independent expert analyses concluded that islands may be the sustainable end-state for this control plane. SPA adds client-side routing, auth guards, global state management, and build complexity that may not be justified for an internal tool with 1-2 developers.

**Prerequisite:** All pages functional on new design system. Team has lived with the islands architecture for 2+ months.

## SPA Decision Criteria

Proceed to SPA **only if** at least 3 of these conditions are true:

| # | Condition | Evidence Required |
|---|-----------|-------------------|
| 1 | Cross-page state sharing is causing bugs or duplication | Documented instances of state sync issues between islands |
| 2 | Page transitions are noticeably slow (full page reload) | User complaints or metrics showing > 500ms transition times |
| 3 | Offline or optimistic UI is a product requirement | Feature request from users or product roadmap item |
| 4 | Deep linking / browser history needs exceed what HTMX provides | Specific UX flows that require URL-driven client-side state |
| 5 | The team has grown beyond 2 developers and can absorb the maintenance cost | Actual headcount, not projected |

## If SPA Is Approved

| # | Task | Detail |
|---|------|--------|
| 1 | Choose routing strategy | Option A: SvelteKit with `adapter-static` (pre-rendered, no SSR Node dependency). Option B: Lightweight client router (e.g., TanStack Router) under Vite. **Never SSR** — conflicts with single-binary. |
| 2 | Partial SPA first | Start with `/chat` as a SPA route. Admin pages stay as Go template + islands. Prove the pattern before full migration. |
| 3 | JSON API completeness audit | Verify every data need is served by existing JSON endpoints. Add missing endpoints. |
| 4 | Feature-flagged rollback | Every SPA page must have a Go MPA fallback. `COVEN_SPA_ENABLED=1` toggles globally. Ability to revert within 1 deploy. |
| 5 | Gradual page migration | Migrate pages one at a time from Go templates to SPA routes. Run both in parallel with feature flags. |
| 6 | Remove old code | Only after all pages are stable on SPA for 2+ weeks: remove Go templates, HTMX, and island auto-loader. |

## If SPA Is Not Approved (Recommended Default)

The islands architecture becomes the permanent state. Optimize it:

| # | Task | Detail |
|---|------|--------|
| 1 | Polish HTMX + islands patterns | Document standard patterns for future pages. Create a "page template" that new pages copy. |
| 2 | Remove HTMX if unused | If all interactive pages are Svelte islands and HTMX only handles page navigation, evaluate replacing it with `<a>` tags + `View Transitions API` for smoother navigation. |
| 3 | Component library hardening | Add P1/P2 components as needed. Storybook as living documentation. |
| 4 | Performance optimization | Pre-compression (brotli/gzip at build time), HTTP/2 push hints, aggressive caching. |

## Exit Gate

| Criterion | How to verify |
|-----------|---------------|
| Decision documented | Written document: SPA yes/no, rationale, evidence for each criterion |
| If SPA: partial SPA stable for 2 weeks | `/chat` route works as SPA with zero regressions |
| If SPA: rollback verified | Toggle flag off → all pages revert to MPA → no data loss |
| If no SPA: patterns documented | Developer guide for "how to add a new page" using islands pattern |
