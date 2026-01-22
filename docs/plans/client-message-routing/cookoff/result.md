# Cookoff Results: Client Message Routing

## Design
docs/plans/client-message-routing/design.md

## Implementations
| Impl | Plan Approach | Tests | Lines | Score | Result |
|------|---------------|-------|-------|-------|--------|
| impl-1 | MessageSender+EventSaver, text accumulation | 12 pkg pass | +439 | 23/25 | eliminated |
| impl-2 | MessageRouter+EventSaver (type assertion) | 12 pkg pass | +499 | 22/25 | eliminated |
| impl-3 | MessageRouter, fail-fast storage | 14 tests | +416 | 24/25 | **WINNER** |

## Plans Generated
- impl-1: docs/plans/client-message-routing/cookoff/impl-1/plan.md
- impl-2: docs/plans/client-message-routing/cookoff/impl-2/plan.md
- impl-3: docs/plans/client-message-routing/cookoff/impl-3/plan.md

## Scoring Summary

| Criterion | impl-1 | impl-2 | impl-3 |
|-----------|--------|--------|--------|
| Fitness for Purpose | 5 | 4 | 5 |
| Justified Complexity | 4 | 4 | 5 |
| Readability | 5 | 5 | 5 |
| Robustness & Scale | 4 | 4 | 4 |
| Maintainability | 5 | 5 | 5 |
| **TOTAL** | 23/25 | 22/25 | **24/25** |

## Winner Selection
**Winner: impl-3**

**Reason:** Smallest implementation (243 lines), cleanest design with single MessageRouter interface, includes ThreadID tracking for conversation correlation, and importantly fails fast if inbound event storage fails (defensive design).

**Trade-offs:**
- impl-1's text chunk accumulation could produce cleaner final messages
- impl-2's type assertion pattern for EventSaver is more flexible

## Cleanup
- Worktrees removed: 3
- Branches deleted: client-msg-routing/cookoff/impl-1, impl-2, impl-3
- Winner merged to: main
