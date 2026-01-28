# Web Chat Redesign - Team 2 Implementation Plan

## Philosophy

This implementation focuses on **polish and user delight**. The design doc calls for ChatGPT/Claude.ai-like UX, which means:

1. **Immediate responsiveness** - Thread switching feels instant via HTMX swaps
2. **Keyboard-first workflows** - Power users never touch the mouse
3. **Thoughtful empty states** - Every null state guides the user forward
4. **Subtle animations** - Transitions communicate state without distracting

## Architecture Approach

### Template Structure

```
templates/
  base.html                    # (existing) HTML shell, Tailwind, HTMX
  chat_app.html                # NEW: Full-page chat shell with CSS Grid layout
  partials/
    sidebar.html               # NEW: Thread list with date grouping + admin nav
    thread_item.html           # NEW: Single thread row for HTMX updates
    agent_picker.html          # NEW: Modal overlay for agent selection
    chat_view.html             # NEW: Message area + input (HTMX swappable)
    empty_state.html           # NEW: Welcome screen when no thread selected
    message.html               # NEW: Single message bubble for HTMX append
```

### Layout Strategy

Using CSS Grid for the main layout:
- Fixed sidebar width (280px) that collapses on mobile
- Flexible main content area
- Fixed-position input at bottom of chat view

```css
.chat-app {
  display: grid;
  grid-template-columns: 280px 1fr;
  grid-template-rows: auto 1fr;
  height: 100vh;
}
```

### HTMX Strategy

1. **Thread switching**: `hx-get="/admin/chat/thread/{id}"` swaps `#chat-content`
2. **Thread list updates**: `hx-get="/admin/threads/list"` updates sidebar
3. **New thread creation**: Modal submits, returns new thread, swaps into sidebar
4. **Message sending**: Form posts, triggers SSE subscription for response

### Keyboard Shortcuts

| Shortcut | Action | Implementation |
|----------|--------|----------------|
| `Cmd+K` | Open search overlay | JS: focus search input, show results |
| `Cmd+N` | New chat | JS: open agent picker modal |
| `Cmd+Shift+S` | Toggle sidebar | JS: add/remove `sidebar-collapsed` class |
| `Escape` | Close modals | JS: close any open overlays |
| `Up/Down` | Navigate threads | JS: when sidebar focused |

## Implementation Sequence

### Phase 1: Template Shell (chat_app.html)

Create the main layout with:
- Header with logo, agent dropdown (disabled initially), settings
- Sidebar placeholder
- Main content placeholder
- CSS Grid layout
- Responsive breakpoints

### Phase 2: Sidebar (partials/sidebar.html)

- "+ New Chat" button at top
- Thread list with date grouping (Today, Yesterday, Previous 7 days, Older)
- Each thread shows: title preview, agent badge, active indicator
- Bottom nav: Agents, Tools, Principals links
- Mobile: hamburger trigger

### Phase 3: Agent Picker Modal (partials/agent_picker.html)

- Overlay that dims background
- List of connected agents with status indicators
- Search/filter input
- Click agent -> create thread -> navigate to chat

### Phase 4: Chat View (partials/chat_view.html)

Port existing chat.html functionality:
- Thread header with agent name and status
- Messages area (scrollable)
- Input area (fixed at bottom)
- SSE connection for streaming

### Phase 5: Go Handler Updates

New endpoints:
```go
// Main app shell
GET /admin/           -> renderChatApp (with thread list)

// Thread management (HTMX partials)
GET /admin/threads/list         -> sidebar thread list partial
POST /admin/threads             -> create thread, return thread item
PATCH /admin/threads/{id}       -> rename thread
DELETE /admin/threads/{id}      -> delete thread, return empty

// Chat content (HTMX swaps)
GET /admin/chat/thread/{id}     -> chat view partial for thread
GET /admin/chat/empty           -> empty state partial

// Existing (keep)
POST /admin/chat/{id}/send      -> send message
GET /admin/chat/{id}/stream     -> SSE stream
```

### Phase 6: Polish

- Add transitions/animations
- Implement keyboard shortcuts
- Thread search functionality
- Mobile responsive testing
- Error states

## Data Model Notes

The existing `Thread` model has:
- `ID`, `FrontendName`, `ExternalID`, `AgentID`, `CreatedAt`, `UpdatedAt`

For display, we need a title. Options:
1. Use first message content as title preview
2. Add `Title` field to Thread (requires migration)

For v1, I will use option 1: show first message preview or "New conversation".

## Key Design Decisions

1. **No database migrations** - Work with existing schema
2. **HTMX over client-side state** - Server renders all HTML
3. **Progressive enhancement** - Works without JS, better with it
4. **Existing SSE infrastructure** - Don't reinvent, reuse chat.go
5. **Preserve admin features** - Keep Agents/Tools/Principals accessible

## Success Metrics

- [ ] Chat is default view at `/admin/`
- [ ] Can create new chat in <3 clicks
- [ ] Thread switching feels instant
- [ ] Keyboard shortcuts work
- [ ] Mobile sidebar collapses properly
- [ ] Existing chat streaming works unchanged
