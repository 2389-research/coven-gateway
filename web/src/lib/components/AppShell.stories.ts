import type { Meta, StoryObj } from '@storybook/svelte';
import AppShell from './AppShell.svelte';
import { htmlSnippet } from './_storyHelpers';

const meta = {
  title: 'Layout/AppShell',
  component: AppShell,
  tags: ['autodocs'],
  parameters: {
    layout: 'fullscreen',
  },
} satisfies Meta<AppShell>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    children: htmlSnippet('<div style="padding:24px"><h1 style="font-size:1.5rem;font-weight:600;margin-bottom:8px">Main Content</h1><p style="color:#64748b">This is the main content area of the AppShell.</p></div>'),
  },
};

export const WithSidebar: Story = {
  args: {
    sidebar: htmlSnippet('<div style="padding:16px;border-right:1px solid #e2e8f0;height:100%"><p style="font-weight:600;margin-bottom:8px">Sidebar</p><ul style="list-style:none;padding:0"><li style="padding:4px 0">Nav Item 1</li><li style="padding:4px 0">Nav Item 2</li><li style="padding:4px 0">Nav Item 3</li></ul></div>'),
    children: htmlSnippet('<div style="padding:24px"><h1 style="font-size:1.5rem;font-weight:600;margin-bottom:8px">Dashboard</h1><p style="color:#64748b">Content with sidebar navigation.</p></div>'),
  },
};

export const WithHeader: Story = {
  args: {
    header: htmlSnippet('<div style="display:flex;align-items:center;gap:12px"><span style="font-weight:600">Coven Gateway</span><span style="margin-left:auto;color:#64748b;font-size:0.875rem">v1.0.0</span></div>'),
    children: htmlSnippet('<div style="padding:24px"><h1 style="font-size:1.5rem;font-weight:600;margin-bottom:8px">Main Content</h1><p style="color:#64748b">Content area below the header bar.</p></div>'),
  },
};

export const FullLayout: Story = {
  args: {
    header: htmlSnippet('<div style="display:flex;align-items:center;gap:12px"><span style="font-weight:600">Coven Gateway</span><span style="margin-left:auto;color:#64748b;font-size:0.875rem">admin@coven</span></div>'),
    sidebar: htmlSnippet('<div style="padding:16px"><p style="font-weight:600;margin-bottom:12px">Navigation</p><ul style="list-style:none;padding:0"><li style="padding:6px 0">Dashboard</li><li style="padding:6px 0">Agents</li><li style="padding:6px 0">Threads</li><li style="padding:6px 0">Settings</li></ul></div>'),
    children: htmlSnippet('<div style="padding:24px"><h1 style="font-size:1.5rem;font-weight:600;margin-bottom:8px">Dashboard</h1><p style="color:#64748b">Full layout with header, sidebar, and main content area.</p></div>'),
  },
};
