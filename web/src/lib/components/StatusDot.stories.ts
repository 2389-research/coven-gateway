import type { Meta, StoryObj } from '@storybook/svelte';
import StatusDot from './StatusDot.svelte';

const meta = {
  title: 'Data Display/StatusDot',
  component: StatusDot,
  tags: ['autodocs'],
  argTypes: {
    status: {
      control: 'select',
      options: ['online', 'offline', 'error', 'busy'],
    },
    pulse: { control: 'boolean' },
    label: { control: 'text' },
    showLabel: { control: 'boolean' },
  },
} satisfies Meta<StatusDot>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Online: Story = {
  args: { status: 'online', showLabel: true },
};

export const Offline: Story = {
  args: { status: 'offline', showLabel: true },
};

export const Error: Story = {
  args: { status: 'error', showLabel: true },
};

export const Busy: Story = {
  args: { status: 'busy', showLabel: true },
};

export const WithPulse: Story = {
  args: { status: 'online', pulse: true, showLabel: true },
};

export const CustomLabel: Story = {
  args: { status: 'online', label: 'Agent #42', showLabel: true },
};

export const DotOnly: Story = {
  args: { status: 'online' },
};
