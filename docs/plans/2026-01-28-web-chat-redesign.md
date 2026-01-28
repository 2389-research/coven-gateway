# Coven Gateway Web Chat Redesign

Date: 2026-01-28

## Overview

Redesign the admin web interface to be **chat-centric** like ChatGPT/Claude.ai, while keeping admin features accessible. The chat becomes the primary experience, not a sub-feature.

## Current State

- Admin dashboard with chat as one page among many (`/admin/chat/{id}`)
- Must navigate: Dashboard → Agents → Select Agent → Chat
- Threads exist but aren't prominently surfaced
- Good SSE streaming infrastructure already in place
- Warm/executive aesthetic (forest green, cream) - **keeping this**

## Target State

```
┌─────────────────────────────────────────────────────────────────┐
│  🌲 Coven                                    [Agent ▼]  [⚙️]   │
├────────────────────┬────────────────────────────────────────────┤
│                    │                                            │
│  [+ New Chat]      │                                            │
│                    │     Welcome to Coven                       │
│  ─────────────     │                                            │
│  Today             │     Select an agent and start chatting.    │
│   📎 claude-dev    │                                            │
│   Research task    │     [Start New Chat]                       │
│                    │                                            │
│  Yesterday         │                                            │
│   📎 code-agent    │                                            │
│   Fix auth bug     │                                            │
│                    │                                            │
│  Previous 7 days   │                                            │
│   📎 claude-dev    │                                            │
│   API design       │                                            │
│                    │                                            │
│                    │                                            │
│                    │                                            │
│                    │                                            │
├────────────────────┼────────────────────────────────────────────┤
│  [Agents] [Tools]  │                                            │
│  [Principals]      │                                            │
└────────────────────┴────────────────────────────────────────────┘
```

**Active Chat View:**
```
┌─────────────────────────────────────────────────────────────────┐
│  🌲 Coven                                    [Agent ▼]  [⚙️]   │
├────────────────────┬────────────────────────────────────────────┤
│                    │  Research task              📎 claude-dev  │
│  [+ New Chat]      ├────────────────────────────────────────────┤
│                    │                                            │
│  ─────────────     │  You                                       │
│  Today             │  What's the best approach for implementing │
│  ▶ Research task   │  a rate limiter in Go?                     │
│    claude-dev      │                                            │
│                    │  ─────────────────────────────────────────  │
│  Yesterday         │                                            │
│    Fix auth bug    │  claude-dev                                │
│    code-agent      │  For rate limiting in Go, I'd recommend    │
│                    │  the token bucket algorithm...             │
│                    │                                            │
│                    │  ```go                                     │
│                    │  type RateLimiter struct {                 │
│                    │      tokens    int                         │
│                    │      ...                                   │
│                    │  ```                               [Copy]  │
│                    │                                            │
│                    ├────────────────────────────────────────────┤
│                    │  [                                    ] ⏎  │
├────────────────────┼────────────────────────────────────────────┤
│  [Agents] [Tools]  │  ⌘K Search  ⌘N New Chat                    │
└────────────────────┴────────────────────────────────────────────┘
```

## Key Features

### 1. Conversation Sidebar
- List of all threads, grouped by date (Today, Yesterday, Previous 7 days, Older)
- Each thread shows: title (first message or custom), agent icon/name
- Click to switch conversations
- Right-click/long-press for: Rename, Delete
- Search threads (Cmd+K)
- Collapsible on mobile

### 2. Agent Selection
- **New Chat flow**: Click "+ New Chat" → Agent picker modal → Start typing
- **Header dropdown**: Shows current agent, can switch (creates new thread)
- **Per-thread binding**: Each thread permanently bound to one agent
- Agent shown in thread list and chat header

### 3. Chat Experience
- Full-height message area with auto-scroll
- Message bubbles with sender identification
- Streaming responses with typing indicator
- Rich markdown rendering (already have marked.js)
- Code blocks with syntax highlighting + copy button
- Timestamp on hover
- Token usage collapsed by default, expandable

### 4. Message Input
- Fixed at bottom
- Auto-growing textarea
- Enter to send, Shift+Enter for newline
- Character/token estimate (optional)
- Attachment support (future: files, images)

### 5. Admin Features (Accessible but not primary)
- Bottom-left nav: Agents, Tools, Principals
- Settings gear in header
- Agent status visible (online/offline indicators)
- Quick jump to agent details from chat header

### 6. Keyboard Shortcuts
| Shortcut | Action |
|----------|--------|
| `Cmd+K` | Search conversations |
| `Cmd+N` | New chat |
| `Cmd+Shift+S` | Toggle sidebar |
| `Escape` | Close modals |
| `Up/Down` | Navigate thread list (when sidebar focused) |

## Technical Approach

### Keep
- Go HTML templates (no React/Vue rebuild)
- SSE for streaming (works great)
- HTMX for partial updates
- Existing auth/session system
- Current CSS variables and color palette
- marked.js + DOMPurify for markdown

### Change
- **Layout**: Sidebar + main content instead of full-page navigation
- **Routing**: `/admin/` becomes the chat interface, other pages are modals or sub-routes
- **Thread management**: Surface threads prominently, add CRUD operations
- **Agent picker**: New modal component for selecting agents

### New Templates
```
templates/
├── chat_app.html        # Main shell (sidebar + content area)
├── partials/
│   ├── sidebar.html           # Thread list + nav
│   ├── thread_item.html       # Single thread in sidebar
│   ├── chat_view.html         # Message list + input
│   ├── message.html           # Single message bubble
│   ├── agent_picker.html      # Modal for selecting agent
│   └── empty_state.html       # Welcome screen
```

### New Endpoints
| Endpoint | Method | Purpose |
|----------|--------|---------|
| `GET /admin/` | - | Main chat app shell |
| `GET /admin/threads` | HTMX | Thread list partial |
| `POST /admin/threads` | HTMX | Create new thread |
| `PATCH /admin/threads/{id}` | HTMX | Rename thread |
| `DELETE /admin/threads/{id}` | HTMX | Delete thread |
| `GET /admin/threads/{id}/messages` | HTMX | Load thread messages |

### Data Model
Existing `Thread` and `Message` models should work. May need:
- `Thread.title` field (nullable, defaults to first message preview)
- `Thread.agent_id` field (binding to agent)

## Implementation Phases

### Phase 1: Layout Shell
- New `chat_app.html` template with sidebar + content
- Responsive CSS (sidebar collapses on mobile)
- Basic thread list (from existing threads)
- Route `/admin/` to new template

### Phase 2: Thread Management
- Create new thread with agent selection
- Thread CRUD (rename, delete)
- Thread switching without page reload (HTMX)
- Date grouping in sidebar

### Phase 3: Chat Experience Polish
- Message bubbles with proper styling
- Code block copy buttons
- Keyboard shortcuts
- Search (Cmd+K)
- Empty states and loading states

### Phase 4: Admin Integration
- Bottom nav to Agents/Tools/Principals
- Agent status indicators
- Settings panel

## Non-Goals (v1)
- Multi-agent threads
- Message editing/regeneration
- File attachments
- Mobile app
- Public (non-admin) access

## Success Criteria
- Chat is the default view when visiting `/admin/`
- Can create new conversations and pick agents in <3 clicks
- Switching between threads feels instant (HTMX partial loads)
- Existing functionality (SSE streaming, markdown, auth) unchanged
- Looks cohesive with current design system
