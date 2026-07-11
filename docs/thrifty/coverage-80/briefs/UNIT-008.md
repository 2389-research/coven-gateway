# UNIT-008 — gateway config/lifecycle + grpc handlers

## Objective

Cover gateway.go's config/lifecycle functions (except the tsnet accepted gaps) and
grpc.go's registration/pack-tool/leader handlers. Last gateway unit — after it, the
session gate must pass at ≥80%.

## Inputs / context

- `internal/gateway/gateway.go` (779 lines) and `grpc.go` (447) — read selectively.
- Helpers: `testConfig(t)`, `newTestGateway(t)`, `testMockStream` (grpc_test.go),
  existing grpc/bridge tests for the stream-mock idiom.
- gateway.go gaps: `resolveTailscaleAuthKey` (0%), `resolveTailscaleStateDir` (0%),
  `determineMCPEndpoint` (40%), `determineWebAdminBaseURL` (40%),
  `updateMCPEndpointFromStatus` (0%), `logTailscaleStatus` (0%),
  `warnIgnoredAddresses` (0%), `drainErrors` (0%), `appendCloseError` (66.7%),
  `initStore` (72.7%), `registerBuiltinPacks` (55.6%), `registerHTTPAPIRoutes` (39.3%),
  `createGRPCServer` (66.7%), `createAuthenticatedGRPCServer` (0%),
  `setupListeners` (50%), `setupTCPListeners` (66.7%), `Run` (77.8%),
  `Shutdown` (64.3%), `waitForShutdownSignal` (50%).
  **Accepted gaps — skip:** `createTailscaleHTTPListener`, `createTailscaleTLSListener`,
  `setupTailscaleListeners`.
- grpc.go gaps: `maybeGrantLeaderRole` (23.1%), `handleExecutePackTool` (45%),
  `receiveRegistration` (63.6%), `extractRegistrationInfo` (62.5%),
  `loadAgentSecrets` (62.5%), `maybeUpdateBindingsForWorkspace` (50%),
  `checkRecvError` (60%), `dispatchMessage` (66.7%), `sendPackToolError` (66.7%).

## Approach

1. **Pure config functions:** `resolveTailscaleAuthKey` / `resolveTailscaleStateDir`
   (config value, env var via `t.Setenv`, key-file via temp file, empty — read the
   precedence), `determineMCPEndpoint` / `determineWebAdminBaseURL` (all source arms),
   `updateMCPEndpointFromStatus` + `logTailscaleStatus` (construct the status struct
   they take), `warnIgnoredAddresses` (config combos — assert via a slog handler
   capturing records if the function only logs).
2. **Channel/error helpers:** `drainErrors` (buffered channel with N errors, closed,
   empty), `appendCloseError` remaining arm.
3. **Lifecycle:** `initStore` error path (unwritable/invalid path), `setupListeners` /
   `setupTCPListeners` error paths (occupied port / invalid addr),
   `createGRPCServer` + `createAuthenticatedGRPCServer` (build with auth config —
   assert non-nil, register succeeds), `registerHTTPAPIRoutes` (build gateway, hit a
   couple of registered routes), `registerBuiltinPacks` remaining arms, `Shutdown`
   remaining arms (double-shutdown, with/without started servers), `Run`'s remaining
   reachable leg and `waitForShutdownSignal` via context cancellation (do NOT send
   real signals in tests).
4. **grpc.go:** drive `receiveRegistration`/`extractRegistrationInfo` with
   `testMockStream` variants (valid registration, wrong-first-message, metadata
   variants); `maybeGrantLeaderRole` arms (first agent → leader, second → not, store
   error); `handleExecutePackTool` + `sendPackToolError` with a registry containing a
   test pack (existing pack-tool test patterns) — success, unknown tool, tool error;
   `loadAgentSecrets` (seeded secrets, store error); `maybeUpdateBindingsForWorkspace`
   arms; `checkRecvError` (nil, io.EOF, context.Canceled, other); `dispatchMessage`
   unknown-type arm.
5. **Session gate:** `sh docs/thrifty/coverage-80/gate-gateway.sh` — if <80%, fill the
   largest remaining non-accepted gaps (≤3 attempts). Commit:
   `test(gateway): cover config, lifecycle, and grpc registration handlers`.

## Constraints

- Contract prohibitions. Never send real OS signals; never bind fixed ports (use
  127.0.0.1:0). All goroutines joined; `-race` clean.
- If `Run`'s uncovered leg is only the tsnet serve path, leave it — accepted gap.

## Acceptance criteria

- [ ] (runnable) `sh docs/thrifty/coverage-80/gate-gateway.sh` prints GATE PASS
      (coverage ≥80%, lint clean, race clean).
- [ ] (runnable) `go tool cover -func`: every listed non-accepted function improved
      vs. baseline, with the pure config functions ≥ 85%.
- [ ] (assertional) grpc stream tests reuse/extend `testMockStream` rather than
      defining a parallel mock; no test sends OS signals or binds fixed ports.

## Dependencies

UNIT-007
