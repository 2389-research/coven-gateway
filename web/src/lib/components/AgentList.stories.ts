import type { Meta, StoryObj } from '@storybook/svelte';
import AgentList from './AgentList.svelte';

const defaultAgents = [
  { id: 'agent-1', name: 'Claude Agent', connected: true },
  { id: 'agent-2', name: 'Code Assistant', connected: true },
  { id: 'agent-3', name: 'Research Bot', connected: false },
];

/**
 * Mock fetch('/api/agents') for the lifetime of a story.
 * Returns a cleanup function that restores the original fetch.
 */
function mockAgentsFetch(agents: typeof defaultAgents) {
  return () => {
    const originalFetch = globalThis.fetch;
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      const url =
        typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url;
      if (url === '/api/agents') {
        return new Response(JSON.stringify(agents), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      return originalFetch(input, init);
    }) as typeof fetch;
    return () => {
      globalThis.fetch = originalFetch;
    };
  };
}

const meta: Meta<AgentList> = {
  title: 'Chat/AgentList',
  component: AgentList,
  parameters: { layout: 'padded' },
  beforeEach: mockAgentsFetch(defaultAgents),
  tags: ['autodocs'],
};

export default meta;
type Story = StoryObj<AgentList>;

export const Default: Story = {
  args: {
    activeAgentId: '',
    pollInterval: 600000,
  },
};

export const WithActiveAgent: Story = {
  args: {
    activeAgentId: 'agent-2',
    pollInterval: 600000,
  },
};

export const Empty: Story = {
  args: {
    pollInterval: 600000,
  },
  beforeEach: mockAgentsFetch([]),
};
