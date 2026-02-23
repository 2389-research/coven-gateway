import type { Meta, StoryObj } from '@storybook/svelte';
import AdminLayout from './AdminLayout.svelte';
import { htmlSnippet } from './_storyHelpers';

const meta = {
  title: 'Layout/AdminLayout',
  component: AdminLayout,
  tags: ['autodocs'],
  parameters: {
    layout: 'fullscreen',
  },
} satisfies Meta<AdminLayout>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Dashboard: Story = {
  args: {
    activePage: 'dashboard',
    userName: 'admin@coven',
    csrfToken: 'demo-token',
    children: htmlSnippet(
      '<div style="padding:24px"><h2 style="font-size:1.25rem;font-weight:600;margin-bottom:8px">Dashboard Content</h2><p style="color:var(--color-fgMuted)">Stats, charts, and admin widgets go here.</p></div>',
    ),
  },
};

export const AgentsPage: Story = {
  args: {
    activePage: 'agents',
    userName: 'admin@coven',
    csrfToken: 'demo-token',
    children: htmlSnippet(
      '<div style="padding:24px"><h2 style="font-size:1.25rem;font-weight:600;margin-bottom:8px">Agents</h2><p style="color:var(--color-fgMuted)">Agent management table goes here.</p></div>',
    ),
  },
};

export const ActivityLogs: Story = {
  args: {
    activePage: 'logs',
    userName: 'operator',
    csrfToken: 'demo-token',
    children: htmlSnippet(
      '<div style="padding:24px"><h2 style="font-size:1.25rem;font-weight:600;margin-bottom:8px">Activity Logs</h2><p style="color:var(--color-fgMuted)">Log entries shown here.</p></div>',
    ),
  },
};
