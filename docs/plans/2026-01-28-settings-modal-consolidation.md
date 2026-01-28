# Settings Modal Consolidation

Date: 2026-01-28

## Overview

Consolidate the dashboard and separate admin pages into a settings modal accessible from the chat interface. The chat app becomes the single primary interface.

## Current State

- Chat app at `/admin/` is the primary interface
- Separate dashboard at `/admin/dashboard` with full-page navigation
- Standalone pages for Agents, Principals, Tools, Threads, Link Codes
- Duplicated navigation and layout code

## Target State

- Chat app remains the only interface at `/admin/`
- Gear icon in header opens a settings modal
- Settings modal has three tabs: Agents, Tools, Security
- Dashboard redirects to chat app

## Modal Structure

```
┌─────────────────────────────────────────────────────────────────┐
│  Settings                                              [✕]     │
├──────────────┬──────────────────────────────────────────────────┤
│              │                                                  │
│  ○ Agents    │  [Tab content loads here via HTMX]              │
│              │                                                  │
│  ○ Tools     │                                                  │
│              │                                                  │
│  ○ Security  │                                                  │
│              │                                                  │
├──────────────┴──────────────────────────────────────────────────┤
│  Signed in as {user}            [Sign Out]                      │
└─────────────────────────────────────────────────────────────────┘
```

- Escape key closes modal
- Tabs load content via HTMX
- User info and logout in modal footer
- Maintains warm/executive aesthetic

## Tab Contents

### Agents
- List of agents with connection status (green dot = online)
- Click row to expand details inline (name, version, capabilities, recent threads)
- Reuses existing agent list/detail partials

### Tools
- Grouped by agent
- Collapsible sections per agent
- Shows tool name, description, parameter count
- Reuses existing tools list partial

### Security
Two sections:
- **Principals** — API keys table with name, created date, last used, revoke button
- **Invite Links** — Link codes table with status, created by, create new button
- Reuses existing principals and link code partials

## Implementation

### Keep
- Existing HTMX partials for agents, tools, principals, link codes
- All existing API endpoints
- Chat app shell template

### Add
- Settings modal markup in `chat_app.html`
- Gear icon click handler
- Tab switching (JS + HTMX)
- `GET /admin/settings/{tab}` endpoints for tab content

### Remove/Deprecate
- `/admin/dashboard` — redirect to `/admin/`
- Full-page templates: `dashboard.html`, `agents.html`, `principals.html`, `tools.html`, `threads.html`
- Dashboard sidebar navigation

### Routes After

| Route | Purpose |
|-------|---------|
| `GET /admin/` | Chat app (primary) |
| `GET /admin/settings/agents` | Agents tab partial |
| `GET /admin/settings/tools` | Tools tab partial |
| `GET /admin/settings/security` | Security tab partial |
| `GET /admin/dashboard` | Redirect to `/admin/` |

## Success Criteria

- Single entry point at `/admin/`
- Settings accessible in <2 clicks from any chat state
- All admin functionality preserved
- No full-page navigations for settings
- Clean removal of redundant templates
