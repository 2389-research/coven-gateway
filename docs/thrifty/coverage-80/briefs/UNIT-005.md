# UNIT-005 — webadmin chat/SSE + webauthn store helpers

## Objective

Cover the chat send/stream surface (chat.go + chat handlers in webadmin.go +
chat_app.go), the health SSE stream, and the reachable webauthn helpers. Last webadmin
unit — after it, the session gate must pass at ≥80%.

## Inputs / context

- Read `internal/webadmin/chat.go` (431 lines) and `chat_app.go` (84) fully; grep the
  chat handlers in webadmin.go: `handleChatSend`, `validateChatSendRequest`,
  `sendSessionMessage`, `SendUserQuestion`, `handleChatStream`,
  `setupChatStreamBroadcaster`, `runChatStreamLoop`, `sendBroadcastEvent`,
  `pipeAgentResponses`, `handlePipeResponse`, `handleHealthStream` — all 0%.
- Webauthn helpers at 0%: `storeWebAuthnCredential`, `lookupCredentialUser`,
  `finalizeWebAuthnLogin`; `cleanupLoop` at 45.5%. Study `webauthn_test.go` for the
  established fixtures (it already builds credentials/users).
- The gateway package's SSE tests (`internal/gateway/api_test.go`, grep "SSE") show the
  idiom for testing streaming handlers: cancellable request context +
  `httptest.NewRecorder`, or a flusher-capable recorder.
- `conversation.Service` / `EventBroadcaster` (`internal/conversation`) — read their
  constructors; chat stream handlers depend on them. Wire real instances in the helper
  file if `newTestAdminWithStore` doesn't already.

## Approach

1. **Validators/parsers first** (cheap, direct): `validateChatSendRequest` all branches.
2. **`handleChatSend`:** valid request with whatever backend it needs (real
   conversation.Service over the temp store; a stub agent manager only if the send path
   requires a live agent — read `sendSessionMessage` to see what it calls). Assert
   response JSON/status and any store effect. Error branches: invalid body, missing
   fields, backend error.
3. **Streams:** for `handleChatStream` / `handleHealthStream`: request with a context
   you cancel after the first event(s); run the handler in a goroutine writing to a
   recorder; assert the SSE preamble/headers and at least one event frame, then cancel
   and join (bounded by a deadline — no sleeps). Use the broadcaster to publish an
   event the stream must carry (`sendBroadcastEvent`, `runChatStreamLoop`,
   `setupChatStreamBroadcaster` get covered by this path).
4. **`pipeAgentResponses` / `handlePipeResponse`:** feed a channel of fake agent
   responses (the type is in `internal/agent`) and assert what lands in the stream or
   conversation store.
5. **chat_app.go** handlers: render/serve paths (25 statements — quick).
6. **Webauthn:** direct tests for `storeWebAuthnCredential` + `lookupCredentialUser`
   (round-trip through the temp store, reusing webauthn_test.go fixture patterns) and
   `finalizeWebAuthnLogin` (call with a stored user; assert session cookie + redirect).
   Top up `cleanupLoop` only if trivially reachable (short ticker + cancel).
7. **Session gate:** `sh docs/thrifty/coverage-80/gate-webadmin.sh` — if <80%, check
   `go tool cover -func` for the largest remaining 0% clusters and fill them (≤3 fix
   attempts). Commit: `test(webadmin): cover chat, SSE streams, and webauthn helpers`.

## Constraints

- Contract prohibitions. All goroutine tests must pass `-race` and never leak (join
  everything before test end; use deadlines ~2s, not sleeps).
- SendUserQuestion/chat paths that genuinely require a live gRPC agent: cover the
  reachable validation/error legs and note the rest in the report — do not fake a
  whole agent protocol just for coverage.

## Acceptance criteria

- [ ] (runnable) `sh docs/thrifty/coverage-80/gate-webadmin.sh` prints GATE PASS
      (coverage ≥80%, lint clean, race clean).
- [ ] (runnable) `go tool cover -func`: chat.go functions all > 0%;
      `storeWebAuthnCredential`, `lookupCredentialUser`, `finalizeWebAuthnLogin` ≥ 70%.
- [ ] (assertional) Stream tests assert actual SSE frames (event names/data), join all
      goroutines, and contain no `time.Sleep` synchronization.

## Dependencies

UNIT-004
