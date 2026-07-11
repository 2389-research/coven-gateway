# UNIT-007 — gateway api.go handler gaps

## Objective

Cover the api.go HTTP handlers with low/zero coverage: answer-question, tool-approval,
send-to-agent + SSE streaming legs, thread messages/usage, usage stats, agent routes,
and binding error paths.

## Inputs / context

- Existing helpers in api_test.go: `newTestGateway(t)`, `newTestGatewayWithMockManager(t)`,
  `newTestGatewayWithMockManagerAndStore(t)`, `newTestGatewayWithAgentForBinding(...)`,
  `mockAgentManager` (scriptable SendMessage/ListAgents), `createTestBindingV2`.
  Study how existing handler tests drive SSE responses through `mockAgentManager`.
- Baseline gaps owned: `handleAnswerQuestion` (0%), `validateAnswerQuestionRequest` (0%),
  `handleToolApproval` (0%), `handleSendToAgent` (0%), `sendAgentMessage` (0%),
  `startSSEStream` (0%), `streamResponses` (41.7%), `handleSendError` (0%),
  `handleCreateBindingError` (0%), `handleThreadMessages` (28.1%),
  `handleThreadUsage` (59.3%), `handleUsageStats` (46.9%), `fetchUsageStats` (62.5%),
  `handleAgentRoutes` (55.6%), `handleAgentHistoryImpl` (75%), `handleSendMessage`
  (75.6%), `handleCreateBinding` (72%), `handleDeleteBinding` (64.3%),
  `handleGetSingleBinding` (76.5%), `deleteExistingBinding` (77.8%),
  `verifyThreadExists` (71.4%), `eventToMessageResponse` if UNIT-006 left arms.

## Approach

1. Read each handler region first; map its branches (method checks, body validation,
   lookup failures, success). Route requests the same way existing tests do (direct
   handler call or through the API mux — follow the file's own pattern).
2. **Send/stream paths:** `handleSendToAgent` + `sendAgentMessage` + `startSSEStream` +
   `streamResponses` via `mockAgentManager` scripted to emit a response channel with
   text/tool/done events — assert the SSE frames land in the recorder; also script an
   error return → `handleSendError` branch. Client-disconnect leg: cancel the request
   context mid-stream.
3. **Answer-question + tool-approval:** valid POST (scripted manager expectation),
   validation failures (bad JSON, missing fields → `validateAnswerQuestionRequest`
   arms), unknown agent/request IDs.
4. **Thread messages/usage + usage stats:** seed the store with threads/messages/usage
   events (real store helper already exists), hit the handlers: pagination/limits,
   unknown thread → 404, malformed params, and `fetchUsageStats` window branches.
5. **Bindings:** the uncovered arms are error paths — duplicate create,
   `handleCreateBindingError` rendering, delete of missing binding, get of missing
   binding, `verifyThreadExists` false leg.
6. `go test -count=1 -cover ./internal/gateway/`; note %. Commit:
   `test(gateway): cover api handler error paths and SSE send/stream legs`.

## Constraints

- Contract prohibitions. Reuse `mockAgentManager` — do not define a second manager
  mock. No live gRPC needed anywhere in this unit.

## Acceptance criteria

- [ ] (runnable) `go test -count=1 ./internal/gateway/` passes.
- [ ] (runnable) `go tool cover -func`: every 0% function listed above > 0%;
      `streamResponses` ≥ 70%; `handleThreadMessages` ≥ 70%.
- [ ] (assertional) SSE tests assert frame sequences (event names in order), and every
      handler family covers ≥ 2 error branches.

## Dependencies

UNIT-006
