import type { Meta, StoryObj } from '@storybook/svelte';
import AgentList from './AgentList.svelte';

const meta: Meta<AgentList> = {
  title: 'Chat/AgentList',
  component: AgentList,
  parameters: {
    layout: 'padded',
    // Mock fetch to avoid real API calls in stories
    mockData: [
      {
        url: '/api/agents',
        method: 'GET',
        status: 200,
        response: [
          { id: 'agent-1', name: 'Claude Agent', connected: true },
          { id: 'agent-2', name: 'Code Assistant', connected: true },
          { id: 'agent-3', name: 'Research Bot', connected: true },
        ],
      },
    ],
  },
  tags: ['autodocs'],
};

export default meta;
type Story = StoryObj<AgentList>;

export const Default: Story = {
  args: {
    activeAgentId: '',
    pollInterval: 600000, // Very long poll in stories to avoid fetching
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
  parameters: {
    mockData: [
      {
        url: '/api/agents',
        method: 'GET',
        status: 200,
        response: [],
      },
    ],
  },
};
