# Implementation Plan: Client Message Routing (Team 2)

## Overview

This implementation connects the `ClientService.SendMessage` RPC to the `agent.Manager.SendMessage` function, enabling direct client-to-agent message routing.

## Design Approach

**Key Insight**: The `conversation_key` in the client request IS the agent ID. The client explicitly specifies which agent should receive the message.

**Concurrency Model**: Asynchronous processing. The client receives a message ID immediately, while a background goroutine consumes agent responses and stores them as events. This matches the existing `StreamEvents` pattern where clients poll for updates.

## Architecture

```
ClientService.SendMessage
    │
    ├── Validation (idempotency, content, etc.)
    │
    ├── Get agent from Manager (conversation_key = agent_id)
    │   └── If not found → return gRPC NotFound error
    │
    ├── Store inbound event (client's message)
    │
    ├── Call Manager.SendMessage(agentID, content)
    │   └── Returns <-chan *Response
    │
    ├── Spawn goroutine to consume responses
    │   └── For each response: store as LedgerEvent
    │
    └── Return message ID immediately (async)
```

## Interface Changes

### New Interface: MessageRouter

Instead of coupling directly to `*agent.Manager`, we define an interface that captures the subset of behavior we need:

```go
// MessageRouter routes messages to agents and returns a response channel.
type MessageRouter interface {
    SendMessage(ctx context.Context, req *agent.SendRequest) (<-chan *agent.Response, error)
}
```

This interface:
1. Allows mock implementations for testing
2. Follows Go idiom of accepting interfaces, returning concrete types
3. Matches the existing `AgentLister` pattern in the codebase

### Updated ClientService struct

```go
type ClientService struct {
    pb.UnimplementedClientServiceServer
    store      EventStore
    principals PrincipalStore
    dedupe     DedupeCache
    agents     AgentLister
    router     MessageRouter  // NEW: for routing messages
}
```

### New Constructor

```go
func NewClientServiceComplete(
    eventStore EventStore,
    principalStore PrincipalStore,
    dedupeCache *dedupe.Cache,
    agents AgentLister,
    router MessageRouter,
) *ClientService
```

## Event Storage

When agent responses arrive, convert them to `store.LedgerEvent`:

| Response Event | LedgerEvent Type | Direction |
|---------------|------------------|-----------|
| Text          | message          | outbound_from_agent |
| Thinking      | system           | outbound_from_agent |
| ToolUse       | tool_call        | outbound_from_agent |
| ToolResult    | tool_result      | outbound_from_agent |
| Error         | error            | outbound_from_agent |
| Done          | message (final)  | outbound_from_agent |

## Error Handling

| Scenario | Response |
|----------|----------|
| Agent not found | gRPC `NotFound` error |
| SendMessage fails | gRPC `Internal` error |
| Background storage fails | Log error, continue |

## Files to Modify

1. **`internal/client/send_message.go`**
   - Add `MessageRouter` interface
   - Implement `processClientMessage` with routing logic
   - Add `storeAgentResponses` goroutine helper

2. **`internal/client/history.go`**
   - Add `router` field to `ClientService` struct
   - Add new constructor `NewClientServiceComplete`

3. **`internal/gateway/gateway.go`**
   - Update `NewClientServiceFull` call to pass `agentMgr` as router

4. **`internal/client/send_message_test.go`**
   - Add tests for agent routing
   - Add mock `MessageRouter` for testing

## Test Plan

### Unit Tests

1. **TestProcessClientMessage_AgentNotFound** - conversation_key doesn't match any agent
2. **TestProcessClientMessage_RoutesToAgent** - verifies Manager.SendMessage is called
3. **TestProcessClientMessage_StoresInboundEvent** - client message stored in ledger
4. **TestProcessClientMessage_StoresResponseEvents** - agent responses stored

### Integration Considerations

The existing `TestSendMessage_*` tests should continue to pass - they test idempotency/validation which happens before routing.

## Implementation Steps

1. Write failing tests for routing behavior
2. Add `MessageRouter` interface and `EventSaver` interface
3. Update `ClientService` struct and constructors
4. Implement `processClientMessage` with routing
5. Update `gateway.go` to wire dependencies
6. Run all tests to verify

## Complexity

- **Lines of code**: ~120-150 new lines
- **Risk**: Low - connecting existing working pieces
- **Dependencies**: None new
