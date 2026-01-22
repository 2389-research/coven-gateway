# Client Message Routing - Design Doc

## Problem

The `ClientService.SendMessage` RPC accepts messages from clients and returns "accepted" with a message ID, but **never actually routes them to agents**. The `processClientMessage` function is a stub that just generates a UUID.

Agents connected via `Manager` never receive client messages, making the direct client API non-functional.

## Current State

```
Client → SendMessage RPC → ClientService.processClientMessage (STUB) → returns UUID
                                     ↓
                              NEVER calls Manager
                                     ↓
Agent ← (nothing)
```

## Desired State

```
Client → SendMessage RPC → ClientService.processClientMessage
                                     ↓
                              Manager.SendMessage(agentID, content)
                                     ↓
                              Events stored for streaming
                                     ↓
Agent ← ServerMessage (via gRPC stream)
         ↓
Agent → MessageResponse events
         ↓
Store → Events persisted
         ↓
Client ← StreamEvents RPC returns stored events
```

## Key Components

### Existing (Working)

1. **`agent.Manager.SendMessage`** (`internal/agent/manager.go:77-137`)
   - Routes messages to agents by ID
   - Returns channel of Response events
   - Already handles streaming responses from agents

2. **`store.EventStore`** (`internal/store/events.go`)
   - Stores conversation events
   - Has methods for inserting and retrieving events

3. **`ClientService.StreamEvents`** (`internal/client/stream_events.go`)
   - Streams events to client
   - Needs events in store to stream

### Missing (To Implement)

1. **`ClientService.processClientMessage`** needs to:
   - Accept conversation_key as agent ID
   - Call `Manager.SendMessage` to route to agent
   - Consume response events from agent
   - Store events in EventStore
   - Return message ID to client

## Design Decisions

### conversation_key Semantics

For direct client messages, `conversation_key` IS the agent ID. The client specifies which agent should receive the message.

### Event Storage

Events from agent responses must be stored so `StreamEvents` can retrieve them. The store already supports this pattern.

### Concurrency

`Manager.SendMessage` returns a channel. The implementation should:
1. Send message to agent
2. Spawn goroutine to consume response channel
3. Store events as they arrive
4. Return immediately with message ID (async processing)

### Error Handling

- Agent not found → return gRPC error
- Agent offline → return gRPC error
- Agent errors → store as error event

## Dependencies

`ClientService` needs access to:
- `agent.Manager` - to route messages
- `store.EventStore` - already has this

## Files to Modify

1. `internal/client/send_message.go` - implement `processClientMessage`
2. `internal/client/service.go` (if exists) - wire up Manager dependency
3. `internal/gateway/gateway.go` - ensure ClientService gets Manager

## Acceptance Criteria

1. Client can send message via `SendMessage` RPC
2. Agent receives message via bidirectional stream
3. Agent responses are stored in EventStore
4. Client can retrieve responses via `StreamEvents` RPC
5. Existing tests continue to pass
6. New tests cover the routing path

## Complexity Assessment

- **Scope**: Small-medium feature (3-4 files)
- **Risk**: Low (connecting existing working pieces)
- **Estimated size**: ~100-150 lines of Go code
- **Recommendation**: 2-3 parallel implementations
