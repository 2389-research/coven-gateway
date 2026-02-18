# Cross-Phase Risk Mitigation

Concerns that span multiple phases: preventing migration stall, handling implementation drift, and enforcing bundle budgets.

## Preventing Migration Stall

The #1 cause of frontend migration failure is **lost momentum** — the migration stalls halfway, leaving the codebase in a split-brain state.

**Mitigations:**
- **No new features on old stack.** Once Phase 3 ships, all chat changes go through Svelte. Old `chat_app.html` is frozen.
- **Lint rules enforce new patterns.** Custom ESLint rule warns when importing from deprecated CDN paths. CI bot comments on PRs that add Go template logic to already-migrated pages.
- **Each phase has a hard deadline.** If a phase isn't done by 150% of estimated time, hold a retrospective: rescope or accept current state as permanent.

## Handling Implementation Drift

Every phase gate includes a **drift assessment** — a 30-minute review asking:

1. **What did we actually build vs. what we planned?** List differences.
2. **Which differences are improvements?** Keep them. Update the plan.
3. **Which differences create downstream problems?** Identify which future phases are affected. Adjust those phase plans before starting them.
4. **What assumptions from the design document proved wrong?** Update the document. Plans are living documents.

**Example drift scenarios:**
- "We planned small islands but built one big chat island" → Phase 4 admin pages can follow the big-island pattern too. Update the auto-loader to support both.
- "Storybook was too painful, we used a custom harness" → Phase 4 visual testing uses Playwright screenshots instead of Chromatic. Update CI pipeline.
- "Tailwind v4 @theme didn't work well" → Phase 2+ uses CSS custom properties directly. Update component conventions.

## Bundle Budget Enforcement

| Phase | JS Budget (gzipped) | CSS Budget (gzipped) | Enforced By |
|-------|---------------------|----------------------|-------------|
| Phase 1 | 15KB (auto-loader + ConnectionBadge + Svelte runtime) | 5KB (tokens only) | CI check |
| Phase 2 | 50KB (components) | 15KB (tokens + utilities) | CI check |
| Phase 3 | 100KB (chat + marked + DOMPurify) | 25KB | CI check |
| Phase 4 | 150KB (all pages) | 30KB | CI check + nightly Lighthouse |
| Phase 5 | 150KB (unchanged) | 30KB (unchanged) | CI check |

If a budget is exceeded, the phase cannot exit until the bundle is optimized (tree-shaking, code splitting, or dropping dependencies).

## Key Decisions Log

| # | Decision | Rationale | Alternatives |
|---|----------|-----------|-------------|
| 1 | **Svelte 5** over React/Vue | Smallest runtime (~3KB), runes are clean, compiles to vanilla JS | React (heavy), Vue (larger), Lit (too low-level) |
| 2 | **Islands first, SPA optional** | Avoids big-bang rewrite. Islands may be the permanent end-state for an internal control plane with 1-2 devs. SPA evaluated at Phase 6 against concrete criteria. | Immediate SPA (too risky), commit to SPA upfront (unjustified) |
| 3 | **Inter** font family | Purpose-built for screens, variable font, excellent at 12-16px | DM Sans (current), system UI stack (no consistency) |
| 4 | **HSL channel format** | Enables Tailwind `bg-accent/50` alpha composition | Hex (no alpha), full HSL (no Tailwind alpha support) |
| 5 | **Named island entries** | Heavy islands eager, light ones lazy. Vite deduplicates chunks | Single bundle (penalizes light pages), per-page (fragile) |
| 6 | **`/static/` prefix** | Clear separation from API routes. Standard convention | `/assets/` (potential API conflict) |
| 7 | **npm** | Zero config, widest CI support. Not a monorepo | pnpm (adds complexity), yarn (no advantage) |
| 8 | **Custom token script** | Simple: one input, two outputs. Style Dictionary is overkill | Style Dictionary (heavyweight), manual (drift risk) |
| 9 | **Storybook 10.x** | Visual testing, theme switching, a11y, Chromatic integration. Upgraded from 8.6 → 10.x during Phase 2 (8.6 couldn't handle Svelte 5 Snippet props). Uses `_storyHelpers.ts` for CSF3 → Snippet bridging. | HTML harness (no visual regression), Histoire (smaller ecosystem) |
| 10 | **Playwright** | Cross-browser, native WebAuthn via CDP | Cypress (no WebAuthn), Testing Library only (no E2E) |
| 11 | **`data-theme` attribute** | CSS-only, no JS runtime. Server-settable via cookie | Class toggle, media query only (no override), JS-managed vars |
| 12 | **WeakMap** for instances | Auto-GC on DOM removal. Clean HTMX integration | Map (leak risk), globals (same), data-attr (no object ref) |
| 13 | **16px base font** | Web standard, zoomable via browser. Addresses "too small" complaint | 14px (too small), 18px (wastes space) |
| 14 | **4px spacing grid** | Matches Tailwind, consistent rhythm | 8px (too coarse), arbitrary (inconsistent) |
| 15 | **Props via `<script type="application/json">`** | Avoids data-attribute quoting/size limits. Supports complex nested objects. | `data-props` (size limited), URL params (fragile) |
| 16 | **Feature flags per phase** | Safe rollback during migration. Both old and new UI functional simultaneously. | Big-bang cutover (risky), branch-based (merge conflicts) |
| 17 | **Self-hosted Inter font** | Single-binary constraint requires no external CDN dependency. Variable font = one file, all weights. | Google Fonts CDN (external dependency), system fonts (inconsistent) |
| 18 | **Tailwind v4 CSS-first config** | `@theme` directive replaces `tailwind.config.js`. Tokens defined in CSS, not JS. Better alignment with CSS custom properties pipeline. | Tailwind v3 JS config (additional build step), pure CSS (loses utility classes) |

## Open Questions — Resolved

| # | Question | Resolution | Resolved By |
|---|----------|------------|-------------|
| 1 | **Dev server strategy** | Vite proxy to Go (dev default). Go serves templates, Vite proxies `/api` and `/health`. When manifest absent, `ScriptTags()` injects Vite HMR client directly. | Phase 1 implementation |
| 2 | **Pre-compression** | Defer to Phase 4 optimization. Start with runtime serving. Binary size matters more than compression savings until bundle exceeds 200KB. | Expert consensus |
| 3 | **Font hosting** | Self-host. Embed `Inter.var.woff2` (~300KB) via `go:embed`. Eliminates Google Fonts CDN. | Single-binary constraint |
| 4 | **Bundle budget** | Progressive budgets per phase (see table above). Phase 1: 15KB (revised — Svelte runtime alone is ~8KB gzip), Phase 3: 100KB, Phase 4: 150KB. Enforced by CI. | Expert consensus + Phase 1 implementation |
| 5 | **Dark theme timing** | Light-only through Phase 3. Dark theme validated in Storybook during Phase 2. Ships when all pages are on token system (Phase 5 at latest). | Risk of contrast issues |

## Remaining Open Questions

To be resolved during implementation:

1. ~~**Tailwind v4 compatibility**~~: **RESOLVED in Phase 1.** Svelte 5 + Tailwind v4 works via `@tailwindcss/vite` plugin + `@import 'tailwindcss'`. No `@theme` directive or `tailwind.config.js` needed. Token integration via `@import './styles/generated/variables.css'`.
2. ~~**HTMX version upgrade**~~: **DEFERRED.** HTMX 1.9.10 lifecycle events (`beforeSwap`, `beforeCleanupElement`, `afterSwap`, `load`) cover all mount/unmount cases. No upgrade needed for Phase 2. Re-evaluate if we need features from 2.x.
3. **Shared state between islands**: If two islands on the same page need shared state (e.g., agent list sidebar + chat thread), use a shared Svelte store module imported by both. If this proves insufficient, consider promoting to a single bigger island.
4. **WebAuthn library**: Current implementation is hand-rolled. Consider `@simplewebauthn/browser` for more robust browser support. Evaluate during Phase 5.

## References

Industry patterns and expert sources consulted during plan development:

- [Frontend Migration Guide (Frontend Mastery)](https://frontendmastery.com/posts/frontend-migration-guide/) — phased migration strategies, drift prevention, failure modes
- [Islands Architecture (patterns.dev)](https://www.patterns.dev/vanilla/islands-architecture/) — canonical pattern definition
- [Embed Vite App in Go Binary](https://www.tushar.ch/writing/embed-vite-app-in-go-binary/) — Go embed.FS + Vite manifest pattern
- [Tailwind CSS v4 Migration Guide](https://tailwindcss.com/docs/theme) — @theme directive, CSS-first configuration
- [SvelteKit State Management](https://svelte.dev/docs/kit/state-management) — shared store patterns for SSE across components
- [Integrating Design Tokens with Tailwind CSS](https://medium.com/@nicolalazzari_79244/integrating-design-tokens-with-tailwind-css-79a088b06297) — token pipeline best practices
- Multi-model expert consensus: GPT-5-Codex (advocate), GPT-5 (critic), O3 (neutral) — all validated architecture with 7-8/10 confidence
