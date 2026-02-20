import type { Meta, StoryObj } from '@storybook/svelte';
import SidebarNav from './SidebarNav.svelte';

const meta = {
  title: 'Navigation/SidebarNav',
  component: SidebarNav,
  tags: ['autodocs'],
  argTypes: {
    items: { control: 'object' },
    groups: { control: 'object' },
  },
} satisfies Meta<SidebarNav>;

export default meta;
type Story = StoryObj<typeof meta>;

export const FlatItems: Story = {
  args: {
    items: [
      { id: 'dashboard', label: 'Dashboard', active: true },
      { id: 'agents', label: 'Agents' },
      { id: 'threads', label: 'Threads' },
      { id: 'settings', label: 'Settings' },
    ],
  },
};

export const WithGroups: Story = {
  args: {
    groups: [
      {
        label: 'Overview',
        items: [
          { id: 'dashboard', label: 'Dashboard', active: true },
          { id: 'usage', label: 'Usage' },
        ],
      },
      {
        label: 'Management',
        items: [
          { id: 'agents', label: 'Agents' },
          { id: 'threads', label: 'Threads' },
          { id: 'secrets', label: 'Secrets' },
        ],
      },
      {
        label: 'System',
        items: [
          { id: 'settings', label: 'Settings' },
          { id: 'logs', label: 'Logs' },
        ],
      },
    ],
  },
};

export const WithLinks: Story = {
  args: {
    items: [
      { id: 'dashboard', label: 'Dashboard', href: '/dashboard', active: true },
      { id: 'agents', label: 'Agents', href: '/agents' },
      { id: 'docs', label: 'Documentation', href: 'https://docs.example.com' },
    ],
  },
};

export const LongLabels: Story = {
  args: {
    items: [
      { id: '1', label: 'This is a very long navigation label that should truncate', active: true },
      { id: '2', label: 'Another lengthy sidebar item name' },
      { id: '3', label: 'Short' },
    ],
  },
};
