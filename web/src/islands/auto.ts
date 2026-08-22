/**
 * Island auto-loader: mounts Svelte components into [data-island] containers.
 * Pages are full navigations, so mounting happens once per page load;
 * teardown is the browser discarding the document.
 */
import '../app.css';
import { mount } from 'svelte';

// Registry: maps data-island names to lazy component imports.
// Each entry returns the default export of a Svelte component module.
const registry: Record<string, () => Promise<{ default: any }>> = {
  'agent-detail-page': () => import('../lib/components/AgentDetailPage.svelte'),
  'agents-page': () => import('../lib/components/AgentsPage.svelte'),
  'board-page': () => import('../lib/components/BoardPage.svelte'),
  'chat-app': () => import('../lib/components/ChatApp.svelte'),
  'connection-badge': () => import('../lib/components/ConnectionBadge.svelte'),
  'dashboard-page': () => import('../lib/components/DashboardPage.svelte'),
  'invite-form': () => import('../lib/components/InviteForm.svelte'),
  'link-page': () => import('../lib/components/LinkPage.svelte'),
  'login-form': () => import('../lib/components/LoginForm.svelte'),
  'logs-page': () => import('../lib/components/LogsPage.svelte'),
  'principals-page': () => import('../lib/components/PrincipalsPage.svelte'),
  'secrets-page': () => import('../lib/components/SecretsPage.svelte'),
  'setup-complete': () => import('../lib/components/SetupComplete.svelte'),
  'setup-form': () => import('../lib/components/SetupForm.svelte'),
  'thread-detail-page': () => import('../lib/components/ThreadDetailPage.svelte'),
  'threads-page': () => import('../lib/components/ThreadsPage.svelte'),
  'todos-page': () => import('../lib/components/TodosPage.svelte'),
  'tools-page': () => import('../lib/components/ToolsPage.svelte'),
  'usage-page': () => import('../lib/components/UsagePage.svelte'),
};

// Track mounted instances to prevent double-mounting. With scanAndMount
// currently called only on initial page load, the `mounting` WeakSet alone
// covers the live races; this map keeps the guard correct if scanAndMount
// ever gains additional callers.
const instances = new WeakMap<Element, ReturnType<typeof mount>>();
// Guard against concurrent mount calls from overlapping scanAndMount calls.
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

/** Find and mount all islands within a root element. */
function scanAndMount(root: Element | Document): void {
  const islands = root.querySelectorAll('[data-island]');
  islands.forEach((el) => mountIsland(el));
}

// --- Initial mount on page load ---
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', () => scanAndMount(document.body));
} else {
  scanAndMount(document.body);
}
