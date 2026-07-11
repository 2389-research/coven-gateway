# UNIT-006 — gateway SSE format helpers

## Objective

Cover api.go's pure SSE/event formatting helpers — all currently 0%: fast, deterministic
unit tests, no server needed.

## Inputs / context

- `internal/gateway/api.go` — grep each function; they are small pure functions near
  each other: `responseToSSEEvent`, `textSSE`, `fileToSSE`, `toolUseToSSE`,
  `toolResultToSSE`, `toolStateToSSE`, `toolApprovalToSSE`, `usageToSSE`,
  `malformedEvent`, `eventToMessageResponse`, `writeSSEEvent` (66.7%),
  `sendJSONError` (75%), `extractPathSegment` (66.7%).
- Input types: `internal/agent` response/event structs and `internal/store` event
  records — read the type definitions the helpers consume.
- Existing `formatSSEEvent` tests in api_test.go show the expected assertion style.

## Approach

1. One test file, one test func per helper (or per family). Build the input struct,
   call the helper, assert the exact SSE string / returned struct: event name, `data:`
   payload (decode the JSON inside and check fields), trailing newlines.
2. Cover each helper's branches: nil/empty fields, every event type arm in
   `responseToSSEEvent` and `eventToMessageResponse` (text, file, tool use/result/state,
   usage, done, error, malformed).
3. `writeSSEEvent`: happy path to a recorder + the flusher/error branch reachable with
   a non-flushing writer. `sendJSONError`: assert status + JSON body + the encode-error
   branch if reachable with a failing writer. `extractPathSegment`: table of paths
   (present, missing, trailing slash, empty).
4. `go test -count=1 -cover ./internal/gateway/` — note %. Commit:
   `test(gateway): cover SSE and event formatting helpers`.

## Constraints

- Contract prohibitions. Assert exact frame content, not just "no error" — these
  helpers ARE the wire format the TUI parses.

## Acceptance criteria

- [ ] (runnable) `go test -count=1 ./internal/gateway/` passes.
- [ ] (runnable) `go tool cover -func`: every listed helper ≥ 90% except
      `writeSSEEvent`/`sendJSONError` ≥ 75%.
- [ ] (assertional) Each event-type arm has a test decoding the `data:` JSON and
      asserting at least one field value.

## Dependencies

none (first unit of the gateway session)
