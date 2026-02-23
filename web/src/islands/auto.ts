/**
 * Island auto-loader: mounts Svelte components into [data-island] containers.
 * Handles HTMX lifecycle (beforeSwap, beforeCleanupElement, afterSwap, load)
 * for clean mount/unmount on page transitions.
 */
import '../app.css';
import { mount, unmount } from 'svelte';

// Registry: maps data-island names to lazy component imports.
// Each entry returns the default export of a Svelte component module.
const registry: Record<string, () => Promise<{ default: any }>> = {
  'agent-detail-page': () => import('../lib/components/AgentDetailPage.svelte'),
  'agents-page': () => import('../lib/components/AgentsPage.svelte'),
  'board-page': () => import('../lib/components/BoardPage.svelte'),
  'chat-app': () => import('../lib/components/ChatApp.svelte'),
  'connection-badge': () => import('../lib/components/ConnectionBadge.svelte'),
  'dashboard-page': () => import('../lib/components/DashboardPage.svelte'),
  'link-page': () => import('../lib/components/LinkPage.svelte'),
  'login-form': () => import('../lib/components/LoginForm.svelte'),
  'logs-page': () => import('../lib/components/LogsPage.svelte'),
  'principals-page': () => import('../lib/components/PrincipalsPage.svelte'),
  'secrets-page': () => import('../lib/components/SecretsPage.svelte'),
  'thread-detail-page': () => import('../lib/components/ThreadDetailPage.svelte'),
  'threads-page': () => import('../lib/components/ThreadsPage.svelte'),
  'todos-page': () => import('../lib/components/TodosPage.svelte'),
  'tools-page': () => import('../lib/components/ToolsPage.svelte'),
  'usage-page': () => import('../lib/components/UsagePage.svelte'),
};

// Track mounted instances for clean unmounting.
const instances = new WeakMap<Element, ReturnType<typeof mount>>();
// Guard against concurrent mount calls from overlapping HTMX events.
const mounting = new WeakSet<Element>();

/**
 * Read props from a child <script type="application/json"> element.
 * Returns empty object if no script element is found or JSON is invalid.
 */
function readProps(el: Element): Record<string, any> {
  const script = el.querySelector('script[type="application/json"]');
  if (!script?.textContent) return {};
  try {
    return JSON.parse(script.textContent);
  } catch (e) {
    console.warn('[islands] invalid JSON props in', el, e);
    return {};
  }
}

/** Mount a Svelte component into a [data-island] container. */
async function mountIsland(el: Element): Promise<void> {
  if (instances.has(el) || mounting.has(el)) return; // already mounted or in-flight
  mounting.add(el);

  const name = el.getAttribute('data-island');
  if (!name) {
    mounting.delete(el);
    return;
  }

  const loader = registry[name];
  if (!loader) {
    console.warn(`[islands] unknown island: "${name}"`);
    mounting.delete(el);
    return;
  }

  try {
    const mod = await loader();
    const props = readProps(el);
    const instance = mount(mod.default, { target: el, props });
    instances.set(el, instance);
  } catch (e) {
    console.error(`[islands] failed to mount "${name}"`, e);
  } finally {
    mounting.delete(el);
  }
}

/** Unmount a Svelte component from a [data-island] container. */
function unmountIsland(el: Element): void {
  const instance = instances.get(el);
  if (!instance) return;
  try {
    unmount(instance);
  } catch (e) {
    console.error('[islands] failed to unmount', el, e);
  }
  instances.delete(el);
}

/** Find and mount all islands within a root element. */
function scanAndMount(root: Element | Document): void {
  const islands = root.querySelectorAll('[data-island]');
  islands.forEach((el) => mountIsland(el));
}

/** Unmount all islands within a root element. */
function scanAndUnmount(root: Element | Document): void {
  const islands = root.querySelectorAll('[data-island]');
  islands.forEach((el) => unmountIsland(el));
}

// --- HTMX lifecycle integration ---

// Before swap: unmount islands in the element being replaced.
document.addEventListener('htmx:beforeSwap', ((e: CustomEvent) => {
  const target = e.detail?.target;
  if (target instanceof Element) {
    scanAndUnmount(target);
  }
}) as EventListener);

// Before cleanup: unmount islands in elements being removed during settling.
// Catches removals that beforeSwap may miss (e.g. individual element cleanup).
document.addEventListener('htmx:beforeCleanupElement', ((e: CustomEvent) => {
  const elt = e.detail?.elt;
  if (elt instanceof Element) {
    if (elt.hasAttribute('data-island')) {
      unmountIsland(elt);
    }
    scanAndUnmount(elt);
  }
}) as EventListener);

// After swap: mount islands in the new content.
document.addEventListener('htmx:afterSwap', ((e: CustomEvent) => {
  const target = e.detail?.target;
  if (target instanceof Element) {
    scanAndMount(target);
  }
}) as EventListener);

// htmx:load fires for OOB swaps and fragment loads that afterSwap may miss.
document.addEventListener('htmx:load', ((e: CustomEvent) => {
  const elt = e.detail?.elt;
  if (elt instanceof Element) {
    // Mount islands within the loaded element, or the element itself if it's an island
    if (elt.hasAttribute('data-island')) {
      mountIsland(elt);
    }
    scanAndMount(elt);
  }
}) as EventListener);

// --- Initial mount on page load ---
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', () => scanAndMount(document.body));
} else {
  scanAndMount(document.body);
}
