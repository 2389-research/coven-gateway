# Phase 2: Design System Core — Build What Chat Needs

**Status:** COMPLETE — Merged to main 2026-02-18, PR #50
**Weeks:** 3–4
**Depends on:** [Phase 1 gate](phase-1-foundation.md#exit-gate)
**Goal:** Build the minimum component library required to implement the chat migration in Phase 3. Not the full inventory — only components that the chat interface and dashboard will consume.

**Scope discipline:** The full component inventory (design doc Section 2) lists ~50 components. Phase 2 builds only the ~12 needed for Phase 3. Other components are built just-in-time when their consuming page is migrated.

## Deliverables

| # | Task | Detail |
|---|------|--------|
| 1 | **Layout:** `AppShell`, `Stack`, `Card` | Core layout primitives. `AppShell` defines sidebar + header + main. These shape every page. |
| 2 | **Navigation:** `SidebarNav`, `Tabs` | Agent sidebar and settings tabs. Required for chat interface chrome. |
| 3 | **Inputs:** `Button`, `IconButton`, `TextField`, `TextArea` | Chat input needs all four. Button variants: primary, secondary, ghost, danger. |
| 4 | **Feedback:** `Spinner`, `Alert`, `Toast` | Loading states, error display, action confirmation. |
| 5 | **Overlays:** `Dialog` | Settings modal and confirmation dialogs. Focus trap + escape close. |
| 6 | **Data Display:** `Badge`, `StatusDot` | Agent status indicators in sidebar. |
| 7 | **Real-time:** `SSEStream` (headless), `ConnectionBadge` (upgrade) | Headless `SSEStream` wraps EventSource with reconnection. `ConnectionBadge` refactored to use it. |
| 8 | Generate Tailwind theme extension | Now that token pipeline is proven, extend `build-tokens.ts` to output a `@theme` block that maps token CSS variables to Tailwind utilities (e.g., `--color-accent: var(--cg-accent-500)`). This integrates with Tailwind v4's CSS-first config via the existing `@import 'tailwindcss'` pipeline — no `tailwind.config.js` needed. Components use Tailwind utilities backed by tokens. |
| 9 | Storybook stories for all Phase 2 components | Storybook 10.x (upgraded from 8.6 due to Svelte 5 Snippet incompatibility). Every component: all variants, all sizes, light theme, interactive states, edge cases. Uses `_storyHelpers.ts` for CSF3 → Snippet bridging. |
| 10 | Replace CDN Tailwind with local build | Remove `cdn.tailwindcss.com` from `base.html`. Tailwind now runs via Vite build. Tokens generate the theme. |
| 11 | Accessibility audit | All P0 components pass axe-core automated checks. Keyboard navigation verified for Dialog, SidebarNav, Tabs. |

## Component API Conventions (Established Here, Used Everywhere)

Codify these patterns now so Phase 3+ components are consistent:

- **Props via `$props()`** with TypeScript interface
- **Snippet-based composition** for render delegation (not slots)
- **CSS via Tailwind utilities** referencing design tokens — no component-scoped `<style>` blocks
- **SSE stores return `close()`** and consumers must call it on unmount — enforce via lint rule or wrapper
- **`data-testid` attributes** on all interactive elements for Playwright

## Exit Gate

| Criterion | How to verify |
|-----------|---------------|
| All 12 components render in Storybook | `npm run storybook` — visual verification of all variants |
| Token round-trip works | Edit `tokens.json` → run `build-tokens.ts` → `variables.css` AND `tailwind.ts` both regenerate correctly |
| Theme switching | `data-theme="dark"` on `<html>` toggles all token values. Verified in Storybook (light-only ships; dark validated). |
| CDN Tailwind removed | `base.html` no longer references `cdn.tailwindcss.com`. All pages render correctly with local Tailwind. |
| Bundle budget | Total JS < 50KB gzipped (components only, no page logic yet). CSS < 15KB gzipped. |
| Accessibility | axe-core reports zero critical violations for all P0 components |
| CI passes | Storybook builds, Vitest unit tests pass, token freshness verified |

## Drift Adaptation

| If this happens... | Then adjust... |
|--------------------|---------------|
| Tailwind v4 `@theme` integration doesn't work as expected | Phase 1 showed `@import 'tailwindcss'` via Vite plugin is the primary integration path. If `@theme` block injection fails, define token-to-utility mappings directly in `app.css` after the `@import`. Keep Tailwind for spacing/layout and use CSS custom properties directly for colors. |
| Storybook 8.6 setup is painful with Svelte 5 | Replace with a lightweight HTML harness (`web/dev/preview.html`) that mounts components directly. Defer Storybook to Phase 4. Visual regression still via Playwright screenshots. |
| Component count creeps above 12 | Stop. If a Phase 3 component needs a primitive not on this list, build the simplest version inline in the consumer first. Extract to component library only if reused. |
| Dark theme has contrast issues | Ship light-only through Phase 4. Dark theme becomes a Phase 5 deliverable after more tokens are battle-tested. |
| Removing CDN Tailwind breaks existing Go templates | Run old and new Tailwind in parallel temporarily: CDN for unrewritten templates, Vite-built for islands. Remove CDN only when the consuming template is migrated. |

## Bundle Budget

| Asset | Limit (gzipped) |
|-------|-----------------|
| JS (components) | 50KB |
| CSS (tokens + utilities) | 15KB |
