# Frontend Redesign: Islands-to-SPA Migration

**Date:** 2026-02-17
**Status:** Draft
**Approach:** Svelte 5 islands architecture with planned migration to full SvelteKit SPA

## Summary

Replace the existing Go template + inline Tailwind frontend with a modern, themable design system built on Svelte 5, Vite, and CSS custom properties. The migration uses an "islands" pattern — interactive Svelte components mounting into Go-template-rendered HTML shells — as a stepping stone toward a full SvelteKit SPA.

### Why This Approach

- **Incremental migration**: No big-bang rewrite. Pages migrate one at a time.
- **Single binary preserved**: Compiled Svelte output embeds via `go:embed`, same as today's templates.
- **Design system from day one**: Tokens, components, and patterns are built SPA-ready, even during the islands phase.
- **Real-time first**: SSE streaming, live agent status, and chat are core concerns, not afterthoughts.

---

## 1. Design Tokens

### Source of Truth

All visual values live in `web/tokens/tokens.json`. A build script generates CSS custom properties and a Tailwind theme extension. Nothing is hardcoded in components.

### Token Categories

#### 1.1 Color Primitives

HSL channel format (`H S% L%` without the `hsl()` wrapper) so Tailwind can compose alpha values:

```jsonc
{
  "primitives": {
    "neutral": {
      "0":   "0 0% 100%",
      "50":  "60 5% 97%",
      "100": "50 5% 94%",
      "200": "48 5% 88%",
      "300": "44 4% 78%",
      "400": "40 4% 65%",
      "500": "38 4% 53%",
      "600": "36 5% 40%",
      "700": "34 6% 28%",
      "800": "30 8% 18%",
      "900": "26 10% 12%",
      "950": "24 12% 7%"
    },
    "indigo": {
      "50":  "226 100% 97%",
      "100": "226 100% 94%",
      "500": "239 84% 67%",
      "600": "243 75% 59%",
      "700": "245 58% 51%"
    },
    "green": {
      "50":  "138 76% 97%",
      "100": "141 84% 93%",
      "500": "152 69% 31%",
      "600": "155 65% 25%",
      "700": "157 62% 20%"
    },
    "red": {
      "50":  "0 86% 97%",
      "100": "0 93% 94%",
      "500": "0 72% 51%",
      "600": "0 74% 42%",
      "700": "0 70% 35%"
    },
    "amber": {
      "50":  "48 100% 96%",
      "100": "48 96% 89%",
      "500": "38 92% 50%",
      "600": "32 95% 44%"
    },
    "blue": {
      "50":  "214 100% 97%",
      "100": "214 95% 93%",
      "500": "217 91% 60%",
      "600": "221 83% 53%"
    }
  }
}
```

#### 1.2 Semantic Tokens (Light Theme)

```jsonc
{
  "semantic": {
    "light": {
      "bg":           "{primitives.neutral.0}",
      "bgMuted":      "{primitives.neutral.50}",
      "surface":      "{primitives.neutral.0}",
      "surfaceAlt":   "{primitives.neutral.50}",
      "surfaceHover": "{primitives.neutral.100}",
      "fg":           "{primitives.neutral.900}",
      "fgMuted":      "{primitives.neutral.500}",
      "fgOnAccent":   "{primitives.neutral.0}",
      "border":       "{primitives.neutral.200}",
      "borderHover":  "{primitives.neutral.300}",
      "ring":         "{primitives.indigo.500}",
      "accent":       "{primitives.indigo.600}",
      "accentHover":  "{primitives.indigo.700}",
      "accentMuted":  "{primitives.indigo.50}",
      "success":      { "solidBg": "{primitives.green.600}", "subtleBg": "{primitives.green.50}", "subtleFg": "{primitives.green.700}", "subtleBorder": "{primitives.green.100}" },
      "danger":       { "solidBg": "{primitives.red.600}",   "subtleBg": "{primitives.red.50}",   "subtleFg": "{primitives.red.700}",   "subtleBorder": "{primitives.red.100}" },
      "warning":      { "solidBg": "{primitives.amber.600}", "subtleBg": "{primitives.amber.50}", "subtleFg": "{primitives.amber.600}", "subtleBorder": "{primitives.amber.100}" },
      "info":         { "solidBg": "{primitives.blue.600}",  "subtleBg": "{primitives.blue.50}",  "subtleFg": "{primitives.blue.600}",  "subtleBorder": "{primitives.blue.100}" }
    },
    "dark": {
      "bg":           "{primitives.neutral.950}",
      "bgMuted":      "{primitives.neutral.900}",
      "surface":      "{primitives.neutral.900}",
      "surfaceAlt":   "{primitives.neutral.800}",
      "surfaceHover": "{primitives.neutral.700}",
      "fg":           "{primitives.neutral.50}",
      "fgMuted":      "{primitives.neutral.400}",
      "fgOnAccent":   "{primitives.neutral.0}",
      "border":       "{primitives.neutral.700}",
      "borderHover":  "{primitives.neutral.600}",
      "ring":         "{primitives.indigo.500}",
      "accent":       "{primitives.indigo.500}",
      "accentHover":  "{primitives.indigo.600}",
      "accentMuted":  "{primitives.indigo.700}"
    }
  }
}
```

#### 1.3 Typography

| Token | Value | Notes |
|-------|-------|-------|
| `fontFamily.sans` | `Inter, system-ui, sans-serif` | Primary UI font |
| `fontFamily.mono` | `JetBrains Mono, IBM Plex Mono, monospace` | Code, terminal output, agent IDs |
| `fontSize.xs` | `0.75rem` (12px) | Labels, badges |
| `fontSize.sm` | `0.875rem` (14px) | Secondary text, table cells |
| `fontSize.base` | `1rem` (16px) | Body text baseline |
| `fontSize.lg` | `1.125rem` (18px) | Subheadings |
| `fontSize.xl` | `1.25rem` (20px) | Section titles |
| `fontSize.2xl` | `1.5rem` (24px) | Page titles |
| `fontSize.3xl` | `1.875rem` (30px) | Hero/display text |
| `lineHeight.tight` | `1.25` | Headings |
| `lineHeight.snug` | `1.375` | UI text |
| `lineHeight.normal` | `1.5` | Body |
| `lineHeight.relaxed` | `1.625` | Long-form prose |
| `fontWeight.regular` | `400` | |
| `fontWeight.medium` | `500` | |
| `fontWeight.semibold` | `600` | |
| `fontWeight.bold` | `700` | |

**Decision: Inter over DM Sans.** Inter is purpose-built for screens at small sizes, has variable font support (one file, all weights), and better glyph coverage. Instrument Serif is dropped — this is a tool UI, not a magazine.

#### 1.4 Spacing & Sizing

4px grid, aligned with Tailwind defaults:

| Token | Value |
|-------|-------|
| `space.0` | `0` |
| `space.1` | `0.25rem` (4px) |
| `space.2` | `0.5rem` (8px) |
| `space.3` | `0.75rem` (12px) |
| `space.4` | `1rem` (16px) |
| `space.6` | `1.5rem` (24px) |
| `space.8` | `2rem` (32px) |
| `space.12` | `3rem` (48px) |
| `space.16` | `4rem` (64px) |

Control sizes (input heights, button heights):

| Token | Value |
|-------|-------|
| `size.control.sm` | `2rem` (32px) |
| `size.control.md` | `2.25rem` (36px) |
| `size.control.lg` | `2.5rem` (40px) |

Container widths:

| Token | Value |
|-------|-------|
| `size.container.narrow` | `32rem` (512px) |
| `size.container.default` | `48rem` (768px) |
| `size.container.wide` | `72rem` (1152px) |
| `size.sidebar.width` | `16rem` (256px) |

#### 1.5 Borders, Shadows, Motion

```jsonc
{
  "radius": {
    "xs":   "0.125rem",
    "sm":   "0.25rem",
    "md":   "0.375rem",
    "lg":   "0.5rem",
    "xl":   "0.75rem",
    "2xl":  "1rem",
    "pill": "9999px"
  },
  "shadow": {
    "xs":   "0 1px 2px hsl(0 0% 0% / 0.05)",
    "sm":   "0 1px 3px hsl(0 0% 0% / 0.1), 0 1px 2px hsl(0 0% 0% / 0.06)",
    "md":   "0 4px 6px hsl(0 0% 0% / 0.07), 0 2px 4px hsl(0 0% 0% / 0.06)",
    "lg":   "0 10px 15px hsl(0 0% 0% / 0.1), 0 4px 6px hsl(0 0% 0% / 0.05)",
    "xl":   "0 20px 25px hsl(0 0% 0% / 0.1), 0 8px 10px hsl(0 0% 0% / 0.04)"
  },
  "motion": {
    "fast":   "100ms",
    "normal": "200ms",
    "slow":   "300ms",
    "easing": "cubic-bezier(0.4, 0, 0.2, 1)"
  },
  "zIndex": {
    "dropdown": "50",
    "sticky":   "100",
    "overlay":  "200",
    "modal":    "300",
    "popover":  "400",
    "toast":    "500"
  },
  "focus": {
    "ring": "0 0 0 2px hsl(var(--color-bg)), 0 0 0 4px hsl(var(--color-ring))"
  }
}
```

### Generated Output

The token build script (`web/scripts/build-tokens.ts`) generates:

1. **`web/src/styles/generated/variables.css`** — `:root` and `[data-theme="dark"]` blocks
2. **`web/src/styles/generated/tailwind.ts`** — Tailwind theme extension mapping tokens to `hsl(var(--color-*) / <alpha-value>)`

Theme switching is a `data-theme` attribute on `<html>`. No class toggling, no JS runtime cost.

---

## 2. Component Library

### Architecture Principles

- **Svelte 5 runes** (`$state`, `$derived`, `$effect`) for all reactivity
- **Headless + styled separation** where complexity warrants it (Table, Combobox)
- **Snippet-based composition** using Svelte 5's `{#snippet}` for render delegation
- **CSS custom properties** consumed directly — no component-level theme overrides
- **Accessible by default** — ARIA roles, keyboard navigation, focus management

### Component Inventory

#### Layout
| Component | Purpose | Priority |
|-----------|---------|----------|
| `AppShell` | Top-level layout: sidebar + header + main content | P0 |
| `SplitPane` | Resizable two-panel layout (sidebar + thread) | P0 |
| `Stack` | Vertical/horizontal flex layout with gap control | P0 |
| `Card` | Bordered surface container with optional header/footer | P0 |
| `PageHeader` | Page title + breadcrumbs + action buttons | P1 |
| `Section` | Collapsible content group with heading | P1 |
| `Panel` | Side panel (settings, details) with slide animation | P1 |

#### Navigation
| Component | Purpose | Priority |
|-----------|---------|----------|
| `SidebarNav` | Vertical nav with icons, groups, active state, collapse | P0 |
| `Tabs` | Horizontal tab bar with panel switching | P0 |
| `Breadcrumbs` | Path navigation with links | P1 |
| `CommandPalette` | Cmd+K search modal with fuzzy matching | P1 |
| `Pagination` | Page navigation for lists | P2 |

#### Data Display
| Component | Purpose | Priority |
|-----------|---------|----------|
| `Table` | Headless data table with sorting, selection, virtual scroll | P0 |
| `Badge` | Status labels (agent status, role, policy) | P0 |
| `StatusDot` | Colored dot indicator (online/offline/pending) | P0 |
| `CodeBlock` | Syntax-highlighted code with copy button | P0 |
| `MarkdownRenderer` | Rendered markdown (chat messages, descriptions) | P0 |
| `MetricCard` | Single stat with label, value, trend | P1 |
| `JSONViewer` | Collapsible JSON tree | P1 |
| `KeyValue` | Horizontal key-value pair list | P1 |
| `Avatar` | User/agent avatar with fallback initials | P2 |

#### Inputs
| Component | Purpose | Priority |
|-----------|---------|----------|
| `Button` | Primary/secondary/ghost/danger, sm/md/lg sizes | P0 |
| `IconButton` | Square button with icon only | P0 |
| `TextField` | Text input with label, error, prefix/suffix slots | P0 |
| `TextArea` | Multi-line text input, auto-resize | P0 |
| `Select` | Native or custom dropdown | P0 |
| `Checkbox` | Single checkbox with label | P1 |
| `Switch` | Toggle switch | P1 |
| `FormField` | Wrapper: label + input + description + error message | P1 |
| `Combobox` | Searchable select with custom options | P2 |
| `TagInput` | Multi-value input with tag chips | P2 |

#### Overlays
| Component | Purpose | Priority |
|-----------|---------|----------|
| `Dialog` | Modal dialog with focus trap, escape close | P0 |
| `Drawer` | Slide-in panel from edge | P1 |
| `Tooltip` | Hover/focus tooltip with positioning | P1 |
| `Popover` | Click-triggered floating content | P1 |
| `ContextMenu` | Right-click menu | P2 |

#### Feedback
| Component | Purpose | Priority |
|-----------|---------|----------|
| `Toast` | Temporary notification with auto-dismiss | P0 |
| `Alert` | Inline banner (success/warning/danger/info) | P0 |
| `Spinner` | Loading indicator | P0 |
| `Skeleton` | Content placeholder during loading | P1 |
| `EmptyState` | Placeholder for empty lists/searches | P1 |
| `ConfirmDialog` | Destructive action confirmation | P1 |
| `ProgressBar` | Determinate/indeterminate progress | P2 |

#### Real-time & Chat
| Component | Purpose | Priority |
|-----------|---------|----------|
| `ChatThread` | Scrollable message list with auto-scroll, date separators | P0 |
| `ChatMessage` | Single message bubble (markdown, tool calls, thinking) | P0 |
| `ChatInput` | Multi-line input with send button, Cmd+Enter | P0 |
| `ToolCallView` | Expandable tool call with name, args, result | P0 |
| `ThinkingIndicator` | Animated indicator with optional text | P0 |
| `ConnectionBadge` | SSE connection status indicator | P0 |
| `SSEStream` | Headless component wrapping EventSource with reconnection | P0 |
| `LiveValue` | Auto-updating value from SSE stream | P1 |
| `TokenCounter` | Token usage display with progress bar | P2 |

#### Domain-Specific
| Component | Purpose | Priority |
|-----------|---------|----------|
| `AgentList` | Filterable agent list with status, name, model | P0 |
| `AgentDetailsHeader` | Agent name, status, model, uptime, actions | P1 |
| `PolicyBadge` | Agent policy display (auto-approve, manual, etc.) | P1 |
| `AuditLogList` | Filterable audit log table | P2 |

### Component API Pattern

All components follow a consistent API using Svelte 5 runes:

```svelte
<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    variant?: 'primary' | 'secondary' | 'ghost' | 'danger';
    size?: 'sm' | 'md' | 'lg';
    disabled?: boolean;
    loading?: boolean;
    onclick?: (e: MouseEvent) => void;
    children: Snippet;
  }

  let {
    variant = 'primary',
    size = 'md',
    disabled = false,
    loading = false,
    onclick,
    children
  }: Props = $props();
</script>

<button
  class="btn btn-{variant} btn-{size}"
  {disabled}
  aria-busy={loading}
  {onclick}
>
  {#if loading}<Spinner size="sm" />{/if}
  {@render children()}
</button>
```

### SSE Streaming Pattern

```typescript
// web/src/lib/stores/sse.ts
export function createSSEStore<T>(url: string, parse: (data: string) => T) {
  let data = $state<T | null>(null);
  let status = $state<'connecting' | 'open' | 'closed' | 'error'>('connecting');
  let source: EventSource | null = null;
  let retryCount = 0;
  const maxRetry = 5;

  function connect() {
    source = new EventSource(url);
    source.onopen = () => { status = 'open'; retryCount = 0; };
    source.onmessage = (e) => { data = parse(e.data); };
    source.onerror = () => {
      status = 'error';
      source?.close();
      if (retryCount < maxRetry) {
        setTimeout(connect, Math.min(1000 * 2 ** retryCount, 30000));
        retryCount++;
      } else {
        status = 'closed';
      }
    };
  }

  function close() { source?.close(); status = 'closed'; }
  connect();
  return { get data() { return data; }, get status() { return status; }, close };
}
```

---

## 3. Build Pipeline

### Directory Structure

```
web/
├── tokens/
│   └── tokens.json              <- Design token source of truth
├── scripts/
│   └── build-tokens.ts          <- Token -> CSS/Tailwind generator
├── src/
│   ├── styles/
│   │   ├── generated/
│   │   │   ├── variables.css    <- Generated CSS custom properties
│   │   │   └── tailwind.ts      <- Generated Tailwind theme
│   │   ├── base.css             <- Reset, typography, global styles
│   │   └── utilities.css        <- Shared utility classes
│   ├── lib/
│   │   ├── components/          <- Svelte component library
│   │   │   ├── layout/
│   │   │   ├── navigation/
│   │   │   ├── data-display/
│   │   │   ├── inputs/
│   │   │   ├── overlays/
│   │   │   ├── feedback/
│   │   │   ├── realtime/
│   │   │   └── domain/
│   │   ├── stores/              <- Shared reactive state
│   │   └── utils/               <- Shared utilities
│   ├── islands/                 <- Island entry points
│   │   ├── auto.ts              <- Auto-loader (scans data-island attrs)
│   │   ├── chat.ts              <- Chat island entry
│   │   ├── agent-list.ts        <- Agent list island entry
│   │   └── dashboard-stats.ts   <- Dashboard stats island entry
│   └── app/                     <- Future SPA pages (phase 2)
├── e2e/                         <- Playwright E2E tests
├── package.json
├── vite.config.ts
├── svelte.config.js
├── tailwind.config.ts
├── vitest.config.ts
├── playwright.config.ts
└── .storybook/
    ├── main.ts
    └── preview.ts
```

### Vite Configuration

```typescript
// web/vite.config.ts
import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

export default defineConfig({
  plugins: [svelte()],
  base: '/static/',
  build: {
    outDir: 'dist',
    manifest: true,
    cssCodeSplit: true,
    rollupOptions: {
      input: {
        auto: 'src/islands/auto.ts',
        chat: 'src/islands/chat.ts',
        'agent-list': 'src/islands/agent-list.ts',
        'dashboard-stats': 'src/islands/dashboard-stats.ts',
      },
      output: {
        entryFileNames: 'js/[name].[hash].js',
        chunkFileNames: 'js/chunks/[name].[hash].js',
        assetFileNames: 'assets/[name].[hash][extname]',
      },
    },
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
      '/health': 'http://localhost:8080',
    },
  },
});
```

**Decision: Named entries per island, not a single bundle.** Each heavy island (chat) loads eagerly. Light islands lazy-load via `auto.ts`. Vite deduplicates shared chunks automatically.

### Island Auto-Loader

The auto-loader scans for `data-island` attributes, dynamically imports the matching component, and mounts it. It integrates with HTMX's lifecycle to clean up components before DOM swaps and mount new ones after.

```typescript
// web/src/islands/auto.ts
import { mount, unmount } from 'svelte';

const registry: Record<string, () => Promise<any>> = {
  'chat-thread':       () => import('../lib/components/realtime/ChatThread.svelte'),
  'agent-list':        () => import('../lib/components/domain/AgentList.svelte'),
  'metric-card':       () => import('../lib/components/data-display/MetricCard.svelte'),
  'connection-badge':  () => import('../lib/components/realtime/ConnectionBadge.svelte'),
};

const instances = new WeakMap<Element, any>();

export async function mountAll(root: Element = document.body) {
  const targets = root.querySelectorAll<HTMLElement>('[data-island]');
  for (const el of targets) {
    if (instances.has(el)) continue;
    const name = el.dataset.island!;
    const loader = registry[name];
    if (!loader) { console.warn('Unknown island:', name); continue; }
    const mod = await loader();
    const props = el.dataset.props ? JSON.parse(el.dataset.props) : {};
    const instance = mount(mod.default, { target: el, props });
    instances.set(el, instance);
  }
}

export function unmountAll(root: Element = document.body) {
  const targets = root.querySelectorAll<HTMLElement>('[data-island]');
  for (const el of targets) {
    const instance = instances.get(el);
    if (instance) { unmount(instance); instances.delete(el); }
  }
}

// Initial mount
mountAll();

// HTMX integration: clean up before swap, remount after
document.body.addEventListener('htmx:beforeSwap', (e: any) => {
  unmountAll(e.detail.target);
});
document.body.addEventListener('htmx:afterSwap', (e: any) => {
  mountAll(e.detail.target);
});
```

### Go Asset Embedding

```go
// internal/assets/embed.go
package assets

import (
    "embed"
    "encoding/json"
    "io/fs"
    "net/http"
    "path"
    "strings"
)

//go:embed dist
var distFS embed.FS

type ManifestEntry struct {
    File    string   `json:"file"`
    CSS     []string `json:"css,omitempty"`
    Imports []string `json:"imports,omitempty"`
}

var Manifest map[string]ManifestEntry

func init() {
    data, err := fs.ReadFile(distFS, "dist/.vite/manifest.json")
    if err != nil {
        return // Dev mode: manifest absent
    }
    Manifest = make(map[string]ManifestEntry)
    json.Unmarshal(data, &Manifest)
}

// FileServer returns an http.Handler serving static assets with cache headers.
func FileServer() http.Handler {
    sub, _ := fs.Sub(distFS, "dist")
    fileServer := http.FileServer(http.FS(sub))
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if containsHash(r.URL.Path) {
            w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
        } else {
            w.Header().Set("Cache-Control", "no-cache")
        }
        if ct := mimeFromExt(path.Ext(r.URL.Path)); ct != "" {
            w.Header().Set("Content-Type", ct)
        }
        fileServer.ServeHTTP(w, r)
    })
}

// ScriptTags returns HTML tags for a Vite entry point.
func ScriptTags(entry string) string {
    e, ok := Manifest[entry]
    if !ok {
        return ""
    }
    var b strings.Builder
    for _, css := range e.CSS {
        b.WriteString("<link rel=\"stylesheet\" href=\"/static/")
        b.WriteString(css)
        b.WriteString("\">")
    }
    b.WriteString("<script type=\"module\" src=\"/static/")
    b.WriteString(e.File)
    b.WriteString("\"></script>")
    return b.String()
}
```

### Makefile Targets

```makefile
.PHONY: web web-dev web-tokens web-clean

web-tokens:                          ## Generate CSS/Tailwind from tokens.json
	cd web && npx tsx scripts/build-tokens.ts

web: web-tokens                      ## Build frontend for production
	cd web && npm run build

web-dev:                             ## Start Vite dev server with HMR
	cd web && npm run dev

web-clean:                           ## Remove built assets
	rm -rf web/dist

build: proto web                     ## Build everything
	go build -o bin/coven-gateway ./cmd/coven-gateway
```

---

## 4. Testing Strategy

### Unit Tests (Vitest)

```typescript
// web/vitest.config.ts
import { defineConfig } from 'vitest/config';
import { svelte } from '@sveltejs/vite-plugin-svelte';

export default defineConfig({
  plugins: [svelte({ hot: false })],
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.ts'],
    setupFiles: ['src/test/setup.ts'],
    coverage: {
      provider: 'v8',
      include: ['src/lib/**'],
      thresholds: { statements: 80, branches: 75, functions: 80, lines: 80 },
    },
  },
});
```

**What to unit test:**
- Component rendering with different props/variants
- Store logic (SSE reconnection, state transitions)
- Utility functions (token parsing, formatting)
- Keyboard navigation and focus management

**EventSource mock** for SSE tests:

```typescript
// web/src/test/setup.ts
class MockEventSource {
  static instances: MockEventSource[] = [];
  onopen: ((e: Event) => void) | null = null;
  onmessage: ((e: MessageEvent) => void) | null = null;
  onerror: ((e: Event) => void) | null = null;
  readyState = 0;
  constructor(public url: string) {
    MockEventSource.instances.push(this);
    setTimeout(() => { this.readyState = 1; this.onopen?.(new Event('open')); }, 0);
  }
  simulateMessage(data: string, type = 'message') {
    this.onmessage?.(new MessageEvent(type, { data }));
  }
  simulateError() { this.readyState = 2; this.onerror?.(new Event('error')); }
  close() { this.readyState = 2; }
  static reset() { MockEventSource.instances = []; }
  static latest() { return MockEventSource.instances.at(-1); }
}
Object.defineProperty(globalThis, 'EventSource', { value: MockEventSource, writable: true });
```

### Component Tests (Storybook 9)

Every component gets a `.stories.ts` file covering:
- All variants and sizes
- Interactive states (hover, focus, disabled, loading)
- Light and dark theme
- Edge cases (long text, empty state, error state)

**Visual regression** via Chromatic (runs on every PR).

### E2E Tests (Playwright)

```typescript
// web/playwright.config.ts
import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: 'e2e',
  webServer: {
    command: 'cd .. && GO_TEST_MODE=1 go run ./cmd/coven-gateway serve',
    url: 'http://localhost:8080/health/ready',
    reuseExistingServer: !process.env.CI,
    timeout: 30_000,
  },
  use: { baseURL: 'http://localhost:8080' },
  projects: [{ name: 'chromium', use: { browserName: 'chromium' } }],
});
```

**E2E coverage:**
- Login flow (username/password + WebAuthn via CDP Virtual Authenticator)
- Chat send + SSE response streaming
- Agent list updates
- Navigation between pages
- Island mount/unmount lifecycle (HTMX swap scenarios)

### CI Pipeline

**On every PR:**
1. Lint (ESLint + Svelte check)
2. Vitest unit tests with coverage gate
3. Storybook build verification
4. Chromatic visual snapshots
5. Playwright smoke suite (login, chat send, navigation)

**Nightly:**
- Full Playwright E2E across all pages
- Memory leak detection (mount/unmount cycles, long-running SSE)
- Bundle size tracking (fail if JS/CSS exceeds budget)

---

## 5. Migration Plan

### Phase 1: Foundation (Weeks 1-2)

1. Initialize `web/` with Vite, Svelte 5, Tailwind, TypeScript
2. Create `tokens.json` and token build script
3. Set up `internal/assets/embed.go` with manifest-based asset serving
4. Wire `/static/` route in gateway HTTP server
5. Create `auto.ts` island loader with HTMX lifecycle hooks
6. Set up Vitest, Storybook, Playwright configs
7. **First island: `ConnectionBadge`** — proves the full pipeline

### Phase 2: Design System Core (Weeks 3-4)

1. Build P0 components: Button, TextField, Card, Stack, Badge, Alert, Spinner, Dialog, Toast
2. Storybook stories for all P0 components
3. Update `base.html` to load Vite assets alongside existing templates
4. Replace inline Tailwind config in `base.html` with generated tokens

### Phase 3: Chat Migration (Weeks 5-7)

1. Build chat components: ChatThread, ChatMessage, ChatInput, ToolCallView, ThinkingIndicator
2. Create `chat.ts` island entry point
3. Replace monolithic `chat_app.html` with Go shell + Svelte chat island
4. SSE streaming wired through `createSSEStore`

### Phase 4: Dashboard & Admin (Weeks 8-10)

1. Build data display components: Table, MetricCard, CodeBlock, JSONViewer
2. Migrate dashboard stats, agent list, remaining admin pages

### Phase 5: Login & Auth (Week 11)

1. Unify login page with the new design system
2. WebAuthn flows migrated to Svelte components

### Phase 6: SPA Transition (Weeks 12+)

1. Introduce SvelteKit or client-side router
2. Convert Go template shells to SvelteKit pages
3. Go server becomes pure JSON API + static asset server
4. Remove Go templates, HTMX, and island auto-loader

**Phase 6 gate:** All pages are 90%+ Svelte. Go templates serve only thin HTML shells. JSON API covers all data needs.

---

## 6. Key Decisions Log

| # | Decision | Rationale | Alternatives |
|---|----------|-----------|-------------|
| 1 | **Svelte 5** over React/Vue | Smallest runtime (~3KB), runes are clean, compiles to vanilla JS | React (heavy), Vue (larger), Lit (too low-level) |
| 2 | **Islands then SPA** | Avoids big-bang rewrite. Design system built SPA-ready from day one | Pure islands (state sharing gets messy), immediate SPA (too risky) |
| 3 | **Inter** font family | Purpose-built for screens, variable font, excellent at 12-16px | DM Sans (current), system UI stack (no consistency) |
| 4 | **HSL channel format** | Enables Tailwind `bg-accent/50` alpha composition | Hex (no alpha), full HSL (no Tailwind alpha support) |
| 5 | **Named island entries** | Heavy islands eager, light ones lazy. Vite deduplicates chunks | Single bundle (penalizes light pages), per-page (fragile) |
| 6 | **`/static/` prefix** | Clear separation from API routes. Standard convention | `/assets/` (potential API conflict) |
| 7 | **npm** | Zero config, widest CI support. Not a monorepo | pnpm (adds complexity), yarn (no advantage) |
| 8 | **Custom token script** | Simple: one input, two outputs. Style Dictionary is overkill | Style Dictionary (heavyweight), manual (drift risk) |
| 9 | **Storybook 9** | Visual testing, theme switching, a11y, Chromatic integration | HTML harness (no visual regression), Histoire (smaller ecosystem) |
| 10 | **Playwright** | Cross-browser, native WebAuthn via CDP | Cypress (no WebAuthn), Testing Library only (no E2E) |
| 11 | **`data-theme` attribute** | CSS-only, no JS runtime. Server-settable via cookie | Class toggle, media query only (no override), JS-managed vars |
| 12 | **WeakMap** for instances | Auto-GC on DOM removal. Clean HTMX integration | Map (leak risk), globals (same), data-attr (no object ref) |
| 13 | **16px base font** | Web standard, zoomable via browser. Addresses "too small" complaint | 14px (too small), 18px (wastes space) |
| 14 | **4px spacing grid** | Matches Tailwind, consistent rhythm | 8px (too coarse), arbitrary (inconsistent) |

---

## 7. Open Questions

To be resolved during Phase 1:

1. **Dev server strategy**: Vite proxy to Go, or Go reverse-proxy to Vite? (Leaning: Vite proxy)
2. **Pre-compression**: Build-time gzip/brotli or runtime? (Leaning: Build-time for prod)
3. **Font hosting**: Self-host Inter or CDN? (Leaning: Self-host for single-binary)
4. **Bundle budget**: Max JS/CSS per page? (Suggestion: 150KB JS gzipped, 30KB CSS gzipped)
5. **Dark theme timing**: Ship light-only in Phase 2, add dark in Phase 4? (Leaning: Yes)
