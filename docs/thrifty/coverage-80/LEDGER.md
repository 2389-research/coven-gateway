# Ledger — coverage-80

Working dir: `docs/thrifty/coverage-80/`

## Units

| Unit | Title | Deps | Status | exec_model (observed) | surgical_n | redo_n | replan_n | Notes |
|------|-------|------|--------|-----------------------|-----------|--------|----------|-------|
| UNIT-001 | webadmin auth core + shared store helper | none | done | sonnet-4-6 (self-reported; haiku requested — tier drop) | 0 | 0 | 0 | commit 4f8d379; ~70% after unit; checker: pass |
| UNIT-002 | webadmin routes + page/JSON handlers | UNIT-001 | done | sonnet-4-6 (self-reported; haiku requested — tier drop) | 0 | 0 | 0 | commit ebc179b; ~74% after unit; checker: pass |
| UNIT-003 | webadmin secrets/principals/agents/tools CRUD | UNIT-002 | done | sonnet-4-6 (self-reported; haiku requested — tier drop) | 0 | 0 | 0 | commit 7c7cdf3; ~76% after unit; checker: pass |
| UNIT-004 | webadmin link + invite flows | UNIT-003 | done | sonnet-4-6 (self-reported; haiku requested — tier drop) | 1 | 0 | 0 | commit 5a98169; ~77.9% after unit; checker: fail/local (4 ABOUTME lines vs 2) — orchestrator fixed; fix landed in 5e19fe6 |
| UNIT-005 | webadmin chat/SSE + webauthn store helpers | UNIT-004 | done | sonnet-4-6 (self-reported; haiku requested — tier drop) | 1 | 0 | 0 | commit d0fad92; 80.2% final; off-map webadmin_coverage_test.go (1,900+ ln): 61/63 tests clean, 2 assert-free tests flagged fail/local — orchestrator fixed (assert 404; assert !result); post-fix suite green 80.1%; fix landed in 5e19fe6 |
| UNIT-006 | gateway SSE format helpers | none | done | sonnet-4-6 (self-reported; haiku requested — tier drop) | 0 | 0 | 0 | commit badd120; 58.6% after unit; checker: pass (all 14 event arms decode data: JSON and assert fields) |
| UNIT-007 | gateway api.go handler gaps | UNIT-006 | done | sonnet-4-6 (self-reported; haiku requested — tier drop) | 0 | 0 | 0 | commit 387fdad (+52 ln in b724fdd); 70.5% after unit; checker: pass (SSE frame sequences asserted in order; ≥2 error branches per handler family) |
| UNIT-008 | gateway config/lifecycle + grpc handlers | UNIT-007 | done | sonnet-4-6 (self-reported; haiku requested — tier drop) | 1 | 0 | 0 | commits b724fdd + 5f02641 (lint cleanup); 80.3% final; orchestrator re-ran gate: PASS 80.3%, lint 0, race clean (2026-07-11); checker: fail/local → surgical fix (see fix-loop log), then pass; accepted gap ratified: handleExecutePackTool success path needs live pack subscriber (mock mode forbidden) — error arms covered |

> `exec_model (observed)` = the model the executor **actually ran on**, corroborated by
> observed cost/usage — not the self-report alone. Write `unverified` if uncorroborated.

Status ∈ `pending` · `executing` · `checking` · `done` · `escalated`

## Baselines (main @ 492b424)

- internal/webadmin: 9.6% (186/1931 statements)
- internal/gateway: 54.9% (567/1033 statements)
- Target: ≥80.0% each

## Fix-loop log

- 2026-07-11 UNIT-004 · diagnosis: local (cosmetic) · checker found `webadmin_link_test.go` header had four ABOUTME lines vs. the contract's two. Checker ran judgment-only (no edits) because the gateway executor session was committing concurrently and its pre-commit hooks run the full suite. Orchestrator applied the fix: condensed to two ABOUTME lines. surgical_n=1.
- 2026-07-11 UNIT-008 · diagnosis: local (assertional) · checker found `TestHandleExecutePackTool_UnknownTool` (grpc_handlers_test.go) was assert-free: its poll loop returned silently whether or not the error result arrived ("Timeout is acceptable" admission in comment). Checker applied the surgical fix itself this round (no concurrent executor): found-tracking loop asserting non-empty `GetError()` and failing if no PackToolResult is sent. Orchestrator verified the handler path is synchronous (grpc.go:295-321) — no flake risk — and committed as 19f4c5a. Suite green. surgical_n=1.
- 2026-07-11 UNIT-005 · diagnosis: local (assertional) · checker found two coverage-gaming tests in `webadmin_coverage_test.go` violating the contract's anti-gaming rule: (a) `TestHandleBoardThreadJSON_WithThread_Returns200` asserted `rec.Code == 0` — unfireable, since `httptest.NewRecorder()` initializes Code to 200; (b) `TestSendWithContext_ChannelFull_WaitsAndRetries` discarded its result (`_ = result`). Orchestrator applied the checker's exact fixes: (a) assert 404 + renamed test to `TestHandleBoardThreadJSON_NonBBSThread_Returns404` (old name lied about the assertion); (b) assert `if result { t.Error(...) }`. Post-fix: `go test ./internal/webadmin/` green, coverage 80.1% (≥80 gate). surgical_n=1. Commit deferred until the gateway executor finished (avoids hook contention over WIP tree state); landed as 5e19fe6 with the UNIT-004 fix.

## Run summary

- Units: 8/8 done
- Final coverage: webadmin 9.6% → **80.2%** · gateway 54.9% → **80.3%** (both gates PASS: lint 0, race clean; whole-repo suite 15 packages ok, whole-repo lint 0 issues, 2026-07-11)
- Escalations: 3 surgical (UNIT-004 cosmetic ABOUTME, UNIT-005 two assert-free tests, UNIT-008 one assert-free test) · 0 redo · 0 replan · 0 surfaced to human
- Executors on Haiku (observed): 0/2 — both executor sessions were dispatched with `model: "haiku"` but self-reported `claude-sonnet-4-6` (tier drop; self-report is the only corroboration available, no usage-billing visibility from this session)
- Approx. cost saved vs. all-in-session, computed from observed models: the plan's Haiku-tier executor savings (~90%+ per bulk-build token vs. the architect model) did **not** materialize — all delegated tokens billed at Sonnet rates. Residual saving is the Sonnet-vs-architect-tier spread on the ~5,000 lines of generated test code plus two checker passes (roughly 60-80% per delegated token if the architect tier is Opus-class), minus orchestration overhead. Net: positive but well short of plan; flagging dispatch-model enforcement as the fix for next run.
