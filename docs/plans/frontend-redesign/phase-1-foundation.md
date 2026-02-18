# Phase 1: Foundation — Prove the Pipeline

**Weeks:** 1–2
**Depends on:** Nothing (first phase)
**Goal:** Establish the build toolchain and prove a single Svelte island can render inside a Go template shell, survive HTMX swaps, and ship as a production binary.

## Deliverables

| # | Task | Detail |
|---|------|--------|
| 1 | Initialize `web/` project | Vite + Svelte 5 + TypeScript + Tailwind CSS v4. Use `@theme` directive for CSS-first config (skip `tailwind.config.js`). |
| 2 | Create `tokens.json` + build script | `web/scripts/build-tokens.ts` generates `variables.css` only. Defer Tailwind theme extension to Phase 2 — start lean. |
| 3 | Set up `internal/assets/embed.go` | Manifest-based asset serving with `go:embed dist`. Include `containsHash()` helper (regex: `/\.[a-f0-9]{8,}\./`) and `mimeFromExt()` covering `.js`, `.css`, `.woff2`, `.svg`, `.map`. |
| 4 | Implement `ScriptTags()` with modulepreload | Recursively emit `<link rel="modulepreload">` for `e.Imports` to prevent waterfall loading on cold starts. |
| 5 | Wire `/static/` route | Add route in `internal/webadmin/webadmin.go` alongside existing HTMX routes. Set `Cache-Control: public, max-age=31536000, immutable` for hashed assets, `no-cache` for HTML shells. |
| 6 | Dev-mode asset injection | When `manifest.json` is absent (dev mode), `ScriptTags()` emits `<script type="module" src="http://localhost:5173/@vite/client">` + direct module URLs. This enables HMR while Go serves templates. |
| 7 | Create `auto.ts` island loader | Implement with full HTMX lifecycle coverage: `htmx:beforeSwap`, `htmx:afterSwap`, `htmx:beforeCleanup`, and `htmx:load` (for OOB swaps and fragment loads). WeakMap for instance tracking. |
| 8 | Props via `<script type="application/json">` | Instead of `data-props` attributes (which hit quoting/size limits), read props from a child `<script type="application/json">` element inside the island container. |
| 9 | First island: `ConnectionBadge` | Svelte component showing SSE connection status. Proves: token CSS loads → component mounts → SSE connects → HTMX swap doesn't leak → production binary embeds correctly. |
| 10 | CI pipeline skeleton | GitHub Actions: `npm ci` → `web-tokens` → `npm run build` → `go build` → `go test`. Fail if token build outputs are stale (compare `variables.css` timestamp vs `tokens.json`). |
| 11 | Set up Vitest + Storybook + Playwright configs | Configs only — no stories or E2E tests yet. EventSource mock in `test/setup.ts`. |
| 12 | Self-host Inter variable font | Embed `Inter.var.woff2` via `go:embed` in the static assets. Eliminates Google Fonts CDN dependency for single-binary. |

## Exit Gate

All of the following must pass before proceeding to Phase 2:

| Criterion | How to verify |
|-----------|---------------|
| `ConnectionBadge` renders in production binary | `make build && ./bin/coven-gateway serve` — badge visible on chat page |
| Vite manifest loads correctly | `ScriptTags("src/islands/auto.ts")` returns valid `<script>` + `<link>` tags with hashed filenames |
| Dev HMR works | `make web-dev` → edit `ConnectionBadge.svelte` → change appears without page reload while Go serves templates |
| HTMX swap survival | Navigate between pages via HTMX — island unmounts cleanly (no console errors), remounts on swap-in |
| Memory leak test | Mount/unmount cycle 100 times via automated script — no monotonic memory growth in DevTools |
| CI green | GitHub Actions pipeline builds and tests successfully |
| Token pipeline automated | `tokens.json` change → `make web-tokens` → `variables.css` regenerated → CI verifies freshness |

## Drift Adaptation

*What if the implementation diverges from the plan?*

| If this happens... | Then adjust... |
|--------------------|---------------|
| Tailwind v4 `@theme` doesn't integrate well with Svelte | Fall back to `tailwind.config.ts` with generated theme extension. Add Tailwind theme gen to `build-tokens.ts`. |
| HTMX lifecycle hooks miss edge cases | Add a `MutationObserver` fallback that watches for `data-island` attribute additions/removals. Document which HTMX events are reliable vs unreliable. |
| Dev-mode injection is too complex to maintain | Switch to "Vite as primary dev server" strategy — Vite serves everything in dev, proxies `/api` to Go. Accept that Go templates are only tested in production mode. |
| `go:embed dist` folder too large for fast builds | Exclude `.map` files from embed. Consider build-time gzip (embed `.gz` variants, negotiate in file server). |
| Props via `<script type="application/json">` causes template complexity | Fall back to `data-props` for simple islands, use the script pattern only for complex ones. Document the threshold. |

## Suggested Task Order

```
1. Initialize web/ project (Vite + Svelte 5 + TS)     ← scaffolding
2. Token build script (variables.css only)              ← design foundation
3. internal/assets/embed.go + ScriptTags()              ← Go integration
4. Wire /static/ route in webadmin                      ← plumbing
5. auto.ts island loader                                ← the key piece
6. ConnectionBadge island                               ← proof of concept
7. CI pipeline                                          ← lock it in
```

Items 1–4 are mechanical setup. Item 5 (the island loader) is where the real learning happens. Item 6 proves the full loop.

## Bundle Budget

"Initial JS" = all JS required to hydrate first paint (entry + vendor runtime).
The Svelte 5 runtime is ~8KB gzip — a fixed cost that amortizes as islands are added.

| Asset | Limit (gzipped) | Notes |
|-------|-----------------|-------|
| Initial JS (entry bundle) | 15KB | Includes Svelte runtime (~8KB) + island loader + app.css |
| Per-island chunk | 3KB | Each lazy-loaded island component |
| CSS (tokens only) | 5KB | |
