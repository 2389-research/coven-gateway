# Ledger — coverage-80

Working dir: `docs/thrifty/coverage-80/`

## Units

| Unit | Title | Deps | Status | exec_model (observed) | surgical_n | redo_n | replan_n | Notes |
|------|-------|------|--------|-----------------------|-----------|--------|----------|-------|
| UNIT-001 | webadmin auth core + shared store helper | none | pending | — | 0 | 0 | 0 | |
| UNIT-002 | webadmin routes + page/JSON handlers | UNIT-001 | pending | — | 0 | 0 | 0 | |
| UNIT-003 | webadmin secrets/principals/agents/tools CRUD | UNIT-002 | pending | — | 0 | 0 | 0 | |
| UNIT-004 | webadmin link + invite flows | UNIT-003 | pending | — | 0 | 0 | 0 | |
| UNIT-005 | webadmin chat/SSE + webauthn store helpers | UNIT-004 | pending | — | 0 | 0 | 0 | |
| UNIT-006 | gateway SSE format helpers | none | pending | — | 0 | 0 | 0 | |
| UNIT-007 | gateway api.go handler gaps | UNIT-006 | pending | — | 0 | 0 | 0 | |
| UNIT-008 | gateway config/lifecycle + grpc handlers | UNIT-007 | pending | — | 0 | 0 | 0 | |

> `exec_model (observed)` = the model the executor **actually ran on**, corroborated by
> observed cost/usage — not the self-report alone. Write `unverified` if uncorroborated.

Status ∈ `pending` · `executing` · `checking` · `done` · `escalated`

## Baselines (main @ 492b424)

- internal/webadmin: 9.6% (186/1931 statements)
- internal/gateway: 54.9% (567/1033 statements)
- Target: ≥80.0% each

## Fix-loop log

- (none yet)

## Run summary

- Units: TBD/8 done
- Escalations: TBD surgical · TBD redo · TBD replan · TBD surfaced to human
- Executors on Haiku (observed): TBD/2
- Approx. cost saved vs. all-in-session, computed from observed models: TBD
