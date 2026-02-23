import type { Meta, StoryObj } from '@storybook/svelte';
import Tabs from './Tabs.svelte';

const meta = {
  title: 'Navigation/Tabs',
  component: Tabs,
  tags: ['autodocs'],
  argTypes: {
    activeTab: { control: 'text' },
    tabs: { control: 'object' },
  },
} satisfies Meta<Tabs>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    tabs: [
      { id: 'general', label: 'General' },
      { id: 'security', label: 'Security' },
      { id: 'agents', label: 'Agents' },
    ],
  },
};

export const WithActiveTab: Story = {
  args: {
    tabs: [
      { id: 'general', label: 'General' },
      { id: 'security', label: 'Security' },
      { id: 'agents', label: 'Agents' },
    ],
    activeTab: 'security',
  },
};

export const WithDisabledTab: Story = {
  args: {
    tabs: [
      { id: 'general', label: 'General' },
      { id: 'security', label: 'Security' },
      { id: 'advanced', label: 'Advanced', disabled: true },
    ],
  },
};

export const ManyTabs: Story = {
  args: {
    tabs: [
      { id: 'overview', label: 'Overview' },
      { id: 'threads', label: 'Threads' },
      { id: 'messages', label: 'Messages' },
      { id: 'settings', label: 'Settings' },
      { id: 'logs', label: 'Logs' },
      { id: 'usage', label: 'Usage' },
    ],
  },
};
