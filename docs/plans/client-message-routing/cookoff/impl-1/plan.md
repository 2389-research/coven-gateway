# Client Message Routing - Implementation Plan (Team 1)

## Architectural Approach

My approach: **Direct injection via interface extension**

Rather than creating a new dependency, I will extend the existing `ClientService` to accept a `MessageSender` interface that mirrors what's used in the conversation service. This keeps the changes minimal and uses the patterns already established in the codebase.

## Key Design Decisions

### 1. Interface for Message Sending

Add a `MessageSender` interface to `ClientService` that the `agent.Manager` already satisfies:

```go
type MessageSender interface {
    SendMessage(ctx context.Context, req *agent.SendRequest) (<-chan *agent.Response, error)
}
```

### 2. conversation_key = agent_id

As specified in the design doc, for direct client messages, `conversation_key` IS the agent ID. The client specifies which agent should receive the message.

### 3. Async Event Storage

The implementation will:
1. Send message to agent via Manager
2. Spawn a goroutine to consume the response channel
3. Store events in EventStore as they arrive
4. Return immediately with message ID (non-blocking)

### 4. Event Mapping

Map agent response events to LedgerEvents:
- `EventText` -> accumulate, save on `EventDone`
- `EventToolUse` -> `EventType: tool_call`
- `EventToolResult` -> `EventType: tool_result`
- `EventDone` -> `EventType: message` (with full response)
- `EventError` -> `EventType: error`

## Files to Modify

### 1. `internal/client/history.go`
- Add `MessageSender` interface
- Add `sender` field to `ClientService`
- Update constructors to accept sender

### 2. `internal/client/send_message.go`
- Implement `processClientMessage` to:
  1. Validate agent exists
  2. Create request and send to agent
  3. Store inbound event (client message)
  4. Spawn goroutine to store outbound events (agent responses)
  5. Return message ID

### 3. `internal/gateway/gateway.go`
- Pass `agentManager` to `ClientService` constructor

### 4. `internal/client/send_message_test.go`
- Add tests for:
  - Agent not found error
  - Successful message routing
  - Event storage verification

## Implementation Steps

### Step 1: Extend ClientService with MessageSender (history.go)

Add interface and field:
```go
type MessageSender interface {
    SendMessage(ctx context.Context, req *agent.SendRequest) (<-chan *agent.Response, error)
}
```

Update `ClientService`:
```go
type ClientService struct {
    // existing fields...
    sender MessageSender
}
```

### Step 2: Add EventSaver interface (send_message.go)

Need ability to save events:
```go
type EventSaver interface {
    SaveEvent(ctx context.Context, event *store.LedgerEvent) error
}
```

### Step 3: Implement processClientMessage (send_message.go)

```go
func (s *ClientService) processClientMessage(ctx context.Context, req *pb.ClientSendMessageRequest) (string, error) {
    messageID := uuid.New().String()

    // conversation_key is the agent ID for direct client messages
    agentID := req.ConversationKey

    // Build send request
    sendReq := &agent.SendRequest{
        AgentID:  agentID,
        Content:  req.Content,
        Sender:   "client",
        ThreadID: messageID, // Use message ID as thread for now
    }

    // Send to agent
    respChan, err := s.sender.SendMessage(ctx, sendReq)
    if err != nil {
        // Return appropriate gRPC error
        if errors.Is(err, agent.ErrAgentNotFound) {
            return "", status.Error(codes.NotFound, "agent not found")
        }
        return "", status.Error(codes.Internal, "failed to send message to agent")
    }

    // Store inbound event (client message)
    s.storeClientMessage(ctx, messageID, agentID, req.Content)

    // Spawn goroutine to consume and store responses
    go s.consumeAndStoreResponses(ctx, agentID, respChan)

    return messageID, nil
}
```

### Step 4: Implement response consumption and storage

```go
func (s *ClientService) consumeAndStoreResponses(ctx context.Context, agentID string, respChan <-chan *agent.Response) {
    var textBuffer string

    for resp := range respChan {
        switch resp.Event {
        case agent.EventText:
            textBuffer += resp.Text

        case agent.EventToolUse:
            s.storeEvent(ctx, agentID, store.EventTypeToolCall, resp.ToolUse.InputJSON)

        case agent.EventToolResult:
            s.storeEvent(ctx, agentID, store.EventTypeToolResult, resp.ToolResult.Output)

        case agent.EventDone:
            content := textBuffer
            if content == "" && resp.Text != "" {
                content = resp.Text
            }
            if content != "" {
                s.storeEvent(ctx, agentID, store.EventTypeMessage, content)
            }

        case agent.EventError:
            s.storeEvent(ctx, agentID, store.EventTypeError, resp.Error)
        }
    }
}
```

### Step 5: Update gateway.go

Pass the agent manager to ClientService:
```go
clientService := client.NewClientServiceWithSender(
    sqliteStore,
    sqliteStore,
    dedupeCache,
    agentMgr,  // NEW: agent manager as sender
    agentMgr,  // agent lister
)
```

## Testing Strategy (TDD)

### Test 1: Agent Not Found
- Send message with conversation_key that doesn't match any agent
- Expect `codes.NotFound` error

### Test 2: Successful Message Routing
- Register mock agent
- Send message
- Verify agent received the message
- Verify response is "accepted" with message ID

### Test 3: Event Storage
- Register mock agent that returns responses
- Send message
- Verify events are stored in EventStore

### Test 4: Error Event Storage
- Register mock agent that returns error
- Send message
- Verify error event is stored

## Complexity Notes

This implementation:
- Uses existing interfaces (`agent.Manager` implements `MessageSender`)
- Minimal changes to existing code
- Async response handling (non-blocking)
- Proper gRPC error codes
- Uses existing EventStore patterns

Estimated: ~100 lines of new code, 3 files modified
