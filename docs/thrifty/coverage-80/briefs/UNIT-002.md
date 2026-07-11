# UNIT-002 — webadmin routes + page/JSON handlers

## Objective

Cover route registration and the read-only page + JSON handlers: dashboard, threads,
thread detail, board, logs, usage, todos. This is the bulk lift for webadmin.go and
drags templates.go up via real renders.

## Inputs / context

- `newTestAdminWithStore(t)` from UNIT-001; `requestWithUser(r)` for auth bypass.
- Grep each handler in `internal/webadmin/webadmin.go` before testing it (registration:
  `RegisterRoutes`, `registerRootRoutes`, `registerAdminRoutes`).
- Baseline 0% functions this unit owns: `RegisterRoutes`, `registerRootRoutes`,
  `registerAdminRoutes`, `handleDashboard`, `handleDashboardJSON`, `handleThreadsPage`,
  `handleThreadsJSON`, `handleThreadDetail`, `handleThreadDetailJSON`, `handleBoardPage`,
  `handleBoardJSON`, `handleBoardThreadJSON`, `handleLogsPage`, `handleLogsJSON`,
  `handleUsagePage`, `handleUsageJSON`, `handleTodosPage`, `handleTodosJSON`.
- The store API for seeding: `internal/store` (threads, messages, events). Seed real
  rows through the store so pages render non-empty content you can assert on.

## Approach

1. **Routing:** build an Admin, call `RegisterRoutes` on a fresh `http.ServeMux`, and
   drive requests through the mux: unauthenticated request to an admin page → login
   redirect (proves requireAuth wiring); authenticated request (session cookie or the
   pattern login gives you — if the mux path can't reuse `requestWithUser`, obtain a
   real session cookie once via the UNIT-001 login helper) → 200. A handful of
   representative routes suffices to cover the three registration functions.
2. **Per page:** for each Page/JSON pair — empty store (renders, 200) and seeded store
   (body contains the seeded thread title / log line / etc.). For JSON endpoints,
   decode the response and assert fields, not just status.
3. **Detail + error branches:** `handleThreadDetail(JSON)` with an unknown thread ID →
   assert the not-found behavior (read the code for whether it's 404 or a rendered
   error). Malformed IDs / missing query params on any handler that parses them.
4. Direct handler calls with `requestWithUser` are fine for most cases; the mux path is
   only needed for the registration functions themselves.
5. Run package tests with `-cover`; note the %. Expect a big jump — this unit should
   land webadmin well above 40%.
6. Commit: `test(webadmin): cover route registration and page/JSON handlers`.

## Constraints

- Contract prohibitions. Assert on stable content (seeded titles, JSON field values),
  not on volatile markup details.
- If a handler needs a dependency `newTestAdminWithStore` doesn't wire (e.g. manager
  for a dashboard section), read the handler's nil-handling first; if it genuinely
  panics on nil and can't render without a live agent manager, note it blocked in your
  report rather than editing source.

## Acceptance criteria

- [ ] (runnable) `go test -count=1 ./internal/webadmin/` passes.
- [ ] (runnable) `go tool cover -func` shows every listed handler > 0% and the three
      registration functions ≥ 80%.
- [ ] (assertional) Seeded-store tests assert seeded content appears; JSON tests decode
      and check fields; at least one error branch per handler family is covered.

## Dependencies

UNIT-001
