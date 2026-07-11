# UNIT-003 — webadmin secrets/principals/agents/tools CRUD

## Objective

Cover the admin CRUD surface: secrets (create/update/delete/get-value/list), principals
(approve/delete/revoke/list), agents (page/revoke/list), tools JSON, and their helpers.

## Inputs / context

- `newTestAdminWithStore(t)` (UNIT-001) — the real SQLiteStore matters here: secrets
  handlers call `getSQLiteStore()` which type-asserts `*store.SQLiteStore`.
- Baseline 0% functions owned: `handleSecretsPage`, `handleSecretsJSON`,
  `handleSecretsCreate`, `handleSecretsUpdate`, `handleSecretsDelete`,
  `handleSecretsGetValue`, `parseSecretForm`, `listSecretItems`, `handlePrincipalsPage`,
  `handlePrincipalsJSON`, `handlePrincipalApprove`, `handlePrincipalDelete`,
  `handlePrincipalRevoke`, `handleAgentsPage`, `handleAgentRevoke`, `listAgentItems`,
  `handleToolsJSON`, `sortedToolItems`, plus `listPackItems` (10%).
- Existing tools-page tests: `webadmin_tools_test.go` (registry seeding pattern for
  packs). `internal/packs` for building a Registry with a fake pack if needed.
- Principal store API: grep `PrincipalStore` interface in webadmin.go and its
  implementation in `internal/store` / `internal/admin` for seeding principals.

## Approach

1. **Secrets:** seed via `handleSecretsCreate` (form POST with CSRF per the package's
   pattern) then exercise list/JSON/get-value/update/delete against what was created —
   assert store round-trips (created secret appears, updated value read back, deleted
   secret gone). Error branches: missing/blank name or value (`parseSecretForm`),
   unknown secret on update/delete/get-value, and the CSRF-rejected path.
2. **Principals:** seed pending/active principals through the store, then approve /
   revoke / delete via the handlers; assert resulting store state and redirect/status.
   Unknown-ID error branches included.
3. **Agents:** `handleAgentsPage` with no manager/agents (read nil-handling first) and
   `handleAgentRevoke` against a seeded agent principal; `listAgentItems` via the page.
4. **Tools:** `handleToolsJSON` empty registry + seeded registry (reuse the
   webadmin_tools_test.go registry pattern); assert JSON payload; `sortedToolItems`
   ordering; `listPackItems` remaining branches.
5. Package tests with `-cover`; note %. Commit:
   `test(webadmin): cover secrets, principals, agents, and tools handlers`.

## Constraints

- Contract prohibitions. Secrets tests must never read or reference the repo's real
  `coven-gateway.db` — temp store only.
- Assert secret values only through the handlers' own responses (get-value endpoint),
  matching real behavior.

## Acceptance criteria

- [ ] (runnable) `go test -count=1 ./internal/webadmin/` passes.
- [ ] (runnable) `go tool cover -func`: every listed function > 0%; secrets handler
      family averages ≥ 75%.
- [ ] (assertional) CRUD tests assert store round-trips (create→read→update→delete),
      not just status codes; each family covers at least one validation-error branch
      and one unknown-ID branch.

## Dependencies

UNIT-002
