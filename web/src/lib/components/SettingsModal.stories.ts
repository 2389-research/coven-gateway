import type { Meta, StoryObj } from '@storybook/svelte';
import SettingsModal from './SettingsModal.svelte';

const settingsContent: Record<string, string> = {
  agents: `
    <div class="space-y-4">
      <h3 class="text-sm font-medium">Connected Agents</h3>
      <div class="rounded border border-border p-3 text-sm">
        <p class="font-medium">Claude Agent</p>
        <p class="text-fgMuted">Connected · Last heartbeat 2s ago</p>
      </div>
      <div class="rounded border border-border p-3 text-sm">
        <p class="font-medium">Code Assistant</p>
        <p class="text-fgMuted">Connected · Last heartbeat 5s ago</p>
      </div>
    </div>`,
  tools: `
    <div class="space-y-4">
      <h3 class="text-sm font-medium">Available Tools</h3>
      <p class="text-sm text-fgMuted">No tools configured yet.</p>
    </div>`,
  security: `
    <div class="space-y-4">
      <h3 class="text-sm font-medium">Security Settings</h3>
      <div class="text-sm">
        <label class="flex items-center gap-2">
          <input type="checkbox" checked /> Require authentication
        </label>
      </div>
    </div>`,
  help: `
    <div class="space-y-4">
      <h3 class="text-sm font-medium">Help & Documentation</h3>
      <p class="text-sm text-fgMuted">Visit the docs at docs.example.com for more information.</p>
    </div>`,
};

function mockSettingsFetch() {
  return () => {
    const originalFetch = globalThis.fetch;
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      const url =
        typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url;
      const match = url.match(/^\/settings\/(\w+)$/);
      if (match && settingsContent[match[1]]) {
        return new Response(settingsContent[match[1]], {
          status: 200,
          headers: { 'Content-Type': 'text/html' },
        });
      }
      return originalFetch(input, init);
    }) as typeof fetch;
    return () => {
      globalThis.fetch = originalFetch;
    };
  };
}

const meta: Meta<SettingsModal> = {
  title: 'Chat/SettingsModal',
  component: SettingsModal,
  parameters: { layout: 'centered' },
  tags: ['autodocs'],
  beforeEach: mockSettingsFetch(),
};

export default meta;
type Story = StoryObj<SettingsModal>;

export const Open: Story = {
  args: {
    open: true,
    csrfToken: 'demo-csrf-token',
  },
};

export const Closed: Story = {
  args: {
    open: false,
    csrfToken: 'demo-csrf-token',
  },
};
