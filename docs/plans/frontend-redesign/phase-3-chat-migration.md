# Phase 3: Chat Migration — Highest User Value

**Weeks:** 5–7
**Depends on:** [Phase 2 gate](phase-2-design-system.md#exit-gate)
**Goal:** Replace the monolithic `chat_app.html` (currently ~the entire frontend experience) with a Go template shell hosting Svelte chat islands. This is the highest-value migration — it's the page users spend 90%+ of their time on.

## Deliverables

| # | Task | Detail |
|---|------|--------|
| 1 | `ChatThread` component | Scrollable message list. Auto-scroll on new messages with "scroll to bottom" affordance. Date separators. Virtual scrolling if thread exceeds 500 messages. |
| 2 | `ChatMessage` component | Renders markdown (via marked.js, bundled not CDN), DOMPurify sanitization. Handles: text, thinking, tool calls. Supports user and agent message styles. |
| 3 | `ChatInput` component | Multi-line textarea with auto-resize. Cmd/Ctrl+Enter to send. Character count. Disabled state during send. |
| 4 | `ToolCallView` component | Expandable/collapsible tool call display. Name, arguments (JSON), result. |
| 5 | `ThinkingIndicator` component | Animated dots with optional thinking text from agent. |
| 6 | `createSSEStore` integration | Wire chat to `createSSEStore`. Handle event types: `text`, `thinking`, `tool_use`, `tool_result`, `done`, `error`. Configurable backoff (1s → 30s exponential, max 5 retries). |
| 7 | SSE cleanup enforcement | Every component consuming an SSE store must call `close()` on unmount. Add an `autoCleanupSSE()` helper that registers cleanup with Svelte's `$effect` teardown. |
| 8 | `chat.ts` island entry point | Named Vite entry. Eagerly loaded (not lazy via auto-loader). Mounts full chat experience into `<div data-island="chat-app">`. |
| 9 | Go template shell | New `chat_app_v2.html` — minimal HTML that loads the chat island. Feature-flagged: `COVEN_NEW_CHAT=1` env var switches between old and new chat. |
| 10 | Agent list sidebar | Migrate sidebar agent list to Svelte (`AgentList` component). Live-updating via SSE. |
| 11 | Settings modal | Migrate search (Cmd+K) and settings modal to Svelte `Dialog` components. |
| 12 | Marked.js + DOMPurify bundling | Remove CDN scripts. Bundle via npm as Vite dependencies. Tree-shake unused marked extensions. |
| 13 | E2E: Chat smoke test | Playwright: login → select agent → send message → verify SSE response streams → verify markdown renders. |

## Feature Flag Strategy

The chat migration runs behind a feature flag to enable safe rollback:

```go
// internal/webadmin/chat_app.go
func (a *Admin) handleChatApp(w http.ResponseWriter, r *http.Request) {
    if os.Getenv("COVEN_NEW_CHAT") == "1" {
        a.renderTemplate(w, "chat_app_v2.html", data) // Svelte island
    } else {
        a.renderTemplate(w, "chat_app.html", data)    // Legacy HTMX
    }
}
```

Both old and new chat must be functional simultaneously until this phase's gate passes.

## Exit Gate

| Criterion | How to verify |
|-----------|---------------|
| Feature parity | New chat supports: send message, receive streaming response, markdown rendering, tool call display, thinking indicator, agent switching, settings modal, search modal |
| SSE reliability | E2E test: disconnect network for 5s → reconnect → stream resumes. Run 100 send/receive cycles without leak. |
| SSE cleanup verified | Vitest: mount chat → unmount → verify EventSource.close() called. No open connections after unmount. |
| Performance | Time-to-interactive ≤ 2s on localhost. Bundle: chat island JS < 100KB gzipped (including marked + DOMPurify). |
| Scroll behavior | Auto-scroll on new messages. Manual scroll-up pauses auto-scroll. "New messages" indicator when paused. |
| Feature flag works | Toggle `COVEN_NEW_CHAT` — both versions render correctly. Old chat unaffected by new Svelte assets. |
| Playwright E2E green | Full chat flow: login → agent select → send → streaming response → markdown → tool call expand/collapse |
| No regressions in old UI | With flag off, all existing HTMX flows still work. No console errors from new JS loading. |

## Drift Adaptation

| If this happens... | Then adjust... |
|--------------------|---------------|
| Phase 2 components don't fit chat needs exactly | Extend components inline in chat code first. Extract improvements back to library after chat stabilizes. Don't block chat on library perfection. |
| SSE store pattern doesn't work for chat's multi-event-type streams | Replace generic `createSSEStore` with a chat-specific `createChatStream` that handles typed events (text/thinking/tool). Keep generic store for simpler use cases (ConnectionBadge). |
| marked.js bundle is too large | Switch to `markdown-it` (smaller, modular) or a minimal custom parser for the limited markdown subset agents actually produce. |
| Chat island needs more state than expected (agent list, settings, chat thread all interconnected) | Promote the chat page to a single "app island" rather than multiple small islands. One mount point, internal component routing. This is fine — it's still an island, just a bigger one. |
| Go template shell is harder than expected to keep minimal | Accept a thicker shell. The shell can include sidebar chrome and header in Go HTML; only the chat content area is a Svelte island. Migrate sidebar to Svelte in Phase 4 if needed. |
| Old chat and new chat diverge in capabilities | Freeze old chat. No new features on old chat_app.html. All new work goes to v2. Set a deadline (end of Phase 3) for removing the flag and committing to new chat. |

## Bundle Budget

| Asset | Limit (gzipped) |
|-------|-----------------|
| JS (chat + marked + DOMPurify) | 100KB |
| CSS | 25KB |
