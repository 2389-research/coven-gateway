// ABOUTME: Render smoke tests — every story-only component renders without throwing.
// ABOUTME: Not behavior tests; those live in per-component .test.ts files.
import { afterEach, describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import { createRawSnippet, type Snippet } from 'svelte';

import AgentList from './AgentList.svelte';
import AppShell from './AppShell.svelte';
import Badge from './Badge.svelte';
import Button from './Button.svelte';
import Card from './Card.svelte';
import ConnectionBadge from './ConnectionBadge.svelte';
import Dialog from './Dialog.svelte';
import IconButton from './IconButton.svelte';
import SidebarNav from './SidebarNav.svelte';
import Spinner from './Spinner.svelte';
import Stack from './Stack.svelte';
import StatusDot from './StatusDot.svelte';
import Tabs from './Tabs.svelte';
import TextArea from './TextArea.svelte';
import TextField from './TextField.svelte';
import Toast from './Toast.svelte';
import { addToast, clearToasts } from '../stores/toast.svelte';

const text = (s: string): Snippet =>
  createRawSnippet(() => ({ render: () => `<span>${s}</span>` }));

// One entry per component: [name, Component, minimal props]
const cases: Array<[string, any, Record<string, unknown>]> = [
  // AgentList — Props: activeAgentId?, onSelect?, pollInterval?, class? (all optional)
  ['AgentList', AgentList, {}],
  // AppShell — Props: sidebar?, header?, children: Snippet, class?
  ['AppShell', AppShell, { children: text('content') }],
  // Badge — Props: variant?, size?, fill?, children: Snippet, class?
  ['Badge', Badge, { children: text('badge') }],
  // Button — Props: variant?, size?, loading?, children: Snippet, class?, ...HTMLButtonAttributes
  ['Button', Button, { children: text('go') }],
  // Card — Props: header?, footer?, children: Snippet, padding?, class?
  ['Card', Card, { children: text('body') }],
  // ConnectionBadge — Props: url?, status?, label? (all optional; EventSource mock in test/setup.ts)
  ['ConnectionBadge', ConnectionBadge, {}],
  // Dialog — Props: open: boolean, onclose?, header?, footer?, children: Snippet, class?
  ['Dialog', Dialog, { open: false, children: text('dialog content') }],
  // IconButton — Props: variant?, size?, icon: Snippet, 'aria-label': string, class?, ...HTMLButtonAttributes
  ['IconButton', IconButton, { icon: text('×'), 'aria-label': 'close' }],
  // SidebarNav — Props: items?, groups?, onselect?, class? (all optional)
  ['SidebarNav', SidebarNav, {}],
  // Spinner — Props: size?, label?, class? (all optional)
  ['Spinner', Spinner, {}],
  // Stack — Props: direction?, gap?, align?, justify?, wrap?, children: Snippet, class?
  ['Stack', Stack, { children: text('item') }],
  // StatusDot — Props: status?, pulse?, label?, showLabel?, class? (all optional)
  ['StatusDot', StatusDot, {}],
  // Tabs — Props: tabs: Tab[], activeTab?, onchange?, panel?, class?
  ['Tabs', Tabs, { tabs: [{ id: 'a', label: 'Alpha' }] }],
  // TextArea — Props: label?, error?, autoResize?, class?, ...HTMLTextareaAttributes (all optional)
  ['TextArea', TextArea, {}],
  // TextField — Props: label?, hint?, error?, leading?, trailing?, class?, ...HTMLInputAttributes (all optional)
  ['TextField', TextField, {}],
  // Toast — Props: class? (all optional); addToast pre-populates store so it renders
  ['Toast', Toast, {}],
];

describe('render smoke', () => {
  afterEach(() => {
    clearToasts();
  });

  for (const [name, Component, props] of cases) {
    it(`${name} renders`, () => {
      if (name === 'Toast') {
        addToast('smoke test', 'info', 0);
      }
      const { container } = render(Component, { props });
      expect(container.firstElementChild).not.toBeNull();
    });
  }
});
