import type { Meta, StoryObj } from '@storybook/svelte';
import ConnectionBadge from './ConnectionBadge.svelte';

const meta = {
  title: 'Real-time/ConnectionBadge',
  component: ConnectionBadge,
  tags: ['autodocs'],
  argTypes: {
    status: {
      control: 'select',
      options: ['connecting', 'open', 'closed', 'error'],
    },
    label: { control: 'text' },
  },
} satisfies Meta<ConnectionBadge>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Connecting: Story = {
  args: { status: 'connecting' },
};

export const Connected: Story = {
  args: { status: 'open' },
};

export const Disconnected: Story = {
  args: { status: 'closed' },
};

export const Error: Story = {
  args: { status: 'error' },
};

export const CustomLabel: Story = {
  args: { status: 'open', label: 'Gateway' },
};
