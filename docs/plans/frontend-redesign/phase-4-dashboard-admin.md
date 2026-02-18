# Phase 4: Dashboard & Admin Pages

**Weeks:** 8–10
**Depends on:** [Phase 3 gate](phase-3-chat-migration.md#exit-gate)
**Goal:** Migrate the admin dashboard and management pages. These are lower-traffic than chat but still benefit from the component library and design tokens.

**Prerequisite:** Chat is stable on new stack. Feature flag removed (new chat is default).

## Deliverables

| # | Task | Detail |
|---|------|--------|
| 1 | **Data Display:** `Table` component | Headless data table with sorting, optional row selection. Start without virtual scroll — add if agent/log lists exceed 1000 rows. |
| 2 | **Data Display:** `MetricCard`, `CodeBlock`, `JSONViewer` | Stats display, code/config viewing, JSON tree for agent responses. |
| 3 | **Data Display:** `KeyValue`, `MarkdownRenderer` | Detail views for agents, threads. |
| 4 | **Navigation:** `Breadcrumbs`, `PageHeader` | Admin page chrome. Consistent header with title + actions. |
| 5 | **Feedback:** `Skeleton`, `EmptyState`, `ConfirmDialog` | Loading placeholders, empty list states, destructive action confirmation. |
| 6 | Dashboard stats island | Replace `/admin/dashboard` Go template with Svelte island. MetricCards for agent count, message volume, token usage. SSE-powered live updates. |
| 7 | Agent management island | Replace `/admin/agents` + agent detail pages. Table with filtering, status badges, approve/revoke actions. |
| 8 | Admin page shells | New Go template shells for each admin page. Load appropriate island entry points. |
| 9 | Remaining admin pages | Migrate in priority order: Threads → Logs → Secrets → Principals → Todos → Board → Usage. Each page = Go shell + Svelte island. |
| 10 | Bundle analysis | Vite bundle analyzer run. Identify shared chunks. Add `manualChunks` config if CSS or JS is double-loading across islands. |
| 11 | Performance comparison | Lighthouse audit: new admin pages must achieve equal or lower TTI vs old Go templates (±5% tolerance). |

## Exit Gate

| Criterion | How to verify |
|-----------|---------------|
| All admin pages migrated | Every route under `/admin/` renders via Go shell + Svelte island |
| Performance parity | Lighthouse TTI for dashboard, agents, threads pages ≤ old UI TTI (±5%) |
| Bundle budget | Total JS across all pages < 150KB gzipped. CSS < 30KB gzipped. No shared chunk double-loading. |
| Table performance | Agent list with 100 rows renders in < 200ms. Sort/filter operations < 100ms. |
| E2E coverage | Playwright: navigate all admin pages, verify data loads, test agent approve/revoke flow |
| No regressions | Chat still works perfectly. All Phase 3 E2E tests pass. |

## Drift Adaptation

| If this happens... | Then adjust... |
|--------------------|---------------|
| Phase 3 resulted in a "big island" pattern instead of small islands | Lean into it. Admin pages can follow the same "one island per page" pattern. The auto-loader still handles mount/unmount. |
| Some admin pages are too simple to justify an island | Keep them as pure Go templates with HTMX. Not every page needs Svelte. Apply design tokens via CSS only (variables.css works in Go templates too). |
| Table component gets complex (virtual scroll, column resize, etc.) | Use a third-party headless table library (e.g., TanStack Table Svelte adapter) instead of building from scratch. Wrap it in your component API. |
| Bundle grows beyond budget | Split admin islands into separate named entries (one per admin section). Heavy pages lazy-load. Light pages stay as Go templates + HTMX. |
| TTI is worse than old UI for simple admin pages | The old Go templates were zero-JS SSR. For simple pages, keep Go templates and apply tokens via CSS only. Reserve Svelte for pages that genuinely need interactivity. |

## Bundle Budget

| Asset | Limit (gzipped) |
|-------|-----------------|
| JS (all pages) | 150KB |
| CSS | 30KB |
