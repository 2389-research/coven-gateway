# Client Message Routing - Implementation Plan (Team 3)

## Overview

This implementation connects the `ClientService.SendMessage` RPC to the agent manager, enabling messages from direct API clients to be routed to agents and their responses stored in the event store.

## Architectural Decisions

### 1. Interface-Based Dependency Injection

Instead of directly depending on `*agent.Manager`, we define a `MessageRouter` interface that captures only the methods we need. This keeps the client package loosely coupled and makes testing straightforward.

```go
type MessageRouter interface {
    SendMessage(ctx context.Context, req *agent.SendRequest) (<-chan *agent.Response, error)
}
```

### 2. Synchronous Initial Response, Async Event Storage

The design calls for immediate return of message ID with async processing of agent responses. Our approach:

1. Validate request and agent availability
2. Generate message ID and store inbound event synchronously
3. Start agent message delivery
4. Spawn goroutine to consume response channel and store events
5. Return message ID immediately

### 3. Event Storage

Agent responses are stored as ledger events using the existing `EventStore.SaveEvent` method (which we need to add to the interface). The conversation_key from the request is used to link all events together.

### 4. Error Semantics

- Agent not found: Return `codes.NotFound` error (synchronous)
- Agent offline/send failure: Return `codes.Unavailable` error (synchronous)
- Agent response errors: Stored as error events (async, non-blocking to caller)

## Files to Modify

### 1. `internal/client/history.go`

Extend the `EventStore` interface to include `SaveEvent`:

```go
type EventStore interface {
    GetEvents(ctx context.Context, params store.GetEventsParams) (*store.GetEventsResult, error)
    SaveEvent(ctx context.Context, event *store.LedgerEvent) error
}
```

### 2. `internal/client/send_message.go`

Add `MessageRouter` interface and update `processClientMessage`:

```go
type MessageRouter interface {
    SendMessage(ctx context.Context, req *agent.SendRequest) (<-chan *agent.Response, error)
}

// Update processClientMessage to:
// 1. Store inbound message event
// 2. Route message to agent via MessageRouter
// 3. Spawn goroutine to consume responses and store as events
// 4. Return message ID
```

Add new constructor that accepts the router:

```go
func NewClientServiceWithRouter(
    eventStore EventStore,
    principalStore PrincipalStore,
    dedupeCache *dedupe.Cache,
    agents AgentLister,
    router MessageRouter,
) *ClientService
```

### 3. `internal/gateway/gateway.go`

Update the `ClientService` instantiation to pass the agent manager as the router:

```go
clientService := client.NewClientServiceWithRouter(
    sqliteStore, sqliteStore, dedupeCache, agentMgr, agentMgr,
)
```

## Implementation Details

### processClientMessage Implementation

```go
func (s *ClientService) processClientMessage(ctx context.Context, req *pb.ClientSendMessageRequest) (string, error) {
    messageID := uuid.New().String()
    conversationKey := req.ConversationKey

    // 1. Store inbound message event
    inboundEvent := &store.LedgerEvent{
        ID:              messageID,
        ConversationKey: conversationKey,
        Direction:       store.EventDirectionInbound,
        Author:          "client", // Could enhance with principal info
        Timestamp:       time.Now(),
        Type:            store.EventTypeMessage,
        Text:            &req.Content,
    }

    if err := s.store.SaveEvent(ctx, inboundEvent); err != nil {
        return "", status.Error(codes.Internal, "failed to store message")
    }

    // 2. Route to agent (conversation_key is the agent ID for direct messages)
    if s.router == nil {
        return messageID, nil // No router configured, just store
    }

    respChan, err := s.router.SendMessage(ctx, &agent.SendRequest{
        AgentID:  conversationKey,
        ThreadID: messageID, // Use message ID as thread ID
        Content:  req.Content,
        Sender:   "client",
    })
    if err != nil {
        if errors.Is(err, agent.ErrAgentNotFound) {
            return "", status.Error(codes.NotFound, "agent not found")
        }
        return "", status.Error(codes.Unavailable, "agent unavailable")
    }

    // 3. Spawn goroutine to consume responses
    go s.consumeAgentResponses(context.Background(), conversationKey, messageID, respChan)

    return messageID, nil
}
```

### consumeAgentResponses Implementation

```go
func (s *ClientService) consumeAgentResponses(
    ctx context.Context,
    conversationKey string,
    threadID string,
    respChan <-chan *agent.Response,
) {
    for resp := range respChan {
        event := s.responseToLedgerEvent(conversationKey, threadID, resp)
        if event != nil {
            if err := s.store.SaveEvent(ctx, event); err != nil {
                // Log error but continue processing
                slog.Error("failed to store agent response",
                    "error", err,
                    "conversation_key", conversationKey,
                    "event_type", resp.Event,
                )
            }
        }
    }
}
```

### responseToLedgerEvent Implementation

```go
func (s *ClientService) responseToLedgerEvent(convKey, threadID string, resp *agent.Response) *store.LedgerEvent {
    event := &store.LedgerEvent{
        ID:              uuid.New().String(),
        ConversationKey: convKey,
        Direction:       store.EventDirectionOutbound,
        Author:          "agent",
        Timestamp:       time.Now(),
    }

    switch resp.Event {
    case agent.EventText:
        event.Type = store.EventTypeMessage
        event.Text = &resp.Text
    case agent.EventThinking:
        // Could store as system event or skip
        return nil
    case agent.EventToolUse:
        event.Type = store.EventTypeToolCall
        // Encode tool use details as text or raw payload
    case agent.EventToolResult:
        event.Type = store.EventTypeToolResult
    case agent.EventError:
        event.Type = store.EventTypeError
        event.Text = &resp.Error
    case agent.EventDone:
        // Final response - store if it has content
        if resp.Text != "" {
            event.Type = store.EventTypeMessage
            event.Text = &resp.Text
        } else {
            return nil
        }
    default:
        return nil // Skip other event types for now
    }

    return event
}
```

## Test Plan

### Unit Tests (send_message_test.go)

1. `TestProcessClientMessage_StoresInboundEvent` - Verify inbound message is stored
2. `TestProcessClientMessage_RoutesToAgent` - Verify message is sent to correct agent
3. `TestProcessClientMessage_AgentNotFound` - Verify NotFound error returned
4. `TestProcessClientMessage_StoresAgentResponses` - Verify responses are stored as events
5. `TestProcessClientMessage_AgentError` - Verify error responses are stored

### Integration Tests

1. Full flow test with mock agent that sends responses
2. Verify events can be retrieved via StreamEvents after SendMessage

## Test Approach

Following TDD principles, I'll write tests first that define the expected behavior, then implement the code to make them pass.

## Execution Order

1. Write failing tests for the routing behavior
2. Extend EventStore interface with SaveEvent
3. Add MessageRouter interface and router field to ClientService
4. Implement processClientMessage
5. Implement consumeAgentResponses
6. Implement responseToLedgerEvent
7. Update gateway.go to wire dependencies
8. Run all tests and ensure they pass

## Risk Assessment

- **Low risk**: All pieces exist and work independently
- **Integration point**: Ensuring the async response consumption doesn't leak goroutines
- **Context handling**: Need to use a background context for the response goroutine since the RPC context may be cancelled before responses complete
