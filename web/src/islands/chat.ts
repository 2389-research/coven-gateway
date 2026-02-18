/**
 * Chat island entry point: eagerly loaded, mounts full chat experience.
 * Unlike auto.ts (which lazily loads by registry name), this directly
 * imports and mounts ChatApp for the critical chat page.
 */
import '../app.css';
import { mount, unmount } from 'svelte';
import ChatApp from '../lib/components/ChatApp.svelte';

const TARGET_SELECTOR = '[data-island="chat-app"]';

let instance: ReturnType<typeof mount> | null = null;

function readProps(el: Element): Record<string, any> {
  const script = el.querySelector('script[type="application/json"]');
  if (!script?.textContent) return {};
  try {
    return JSON.parse(script.textContent);
  } catch (e) {
    console.warn('[chat] invalid JSON props in', el, e);
    return {};
  }
}

function mountChat() {
  const target = document.querySelector(TARGET_SELECTOR);
  if (!target || instance) return;

  const props = readProps(target);
  if (!props.agentId) {
    console.warn('[chat] missing agentId prop');
    return;
  }

  instance = mount(ChatApp, { target, props });
}

function unmountChat() {
  if (!instance) return;
  try {
    unmount(instance);
  } catch (e) {
    console.error('[chat] failed to unmount', e);
  }
  instance = null;
}

// HTMX lifecycle integration (for when chat page is loaded via navigation)
document.addEventListener('htmx:beforeSwap', ((e: CustomEvent) => {
  const target = e.detail?.target;
  if (target instanceof Element && target.querySelector(TARGET_SELECTOR)) {
    unmountChat();
  }
}) as EventListener);

document.addEventListener('htmx:afterSwap', (() => {
  mountChat();
}) as EventListener);

// Initial mount on page load
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', mountChat);
} else {
  mountChat();
}
