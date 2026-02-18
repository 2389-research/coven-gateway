import type { Meta, StoryObj } from '@storybook/svelte';
import Alert from './Alert.svelte';

const meta = {
  title: 'Feedback/Alert',
  component: Alert,
  tags: ['autodocs'],
  argTypes: {
    variant: {
      control: 'select',
      options: ['info', 'success', 'warning', 'danger'],
    },
    title: { control: 'text' },
    dismissible: { control: 'boolean' },
  },
} satisfies Meta<Alert>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Info: Story = {
  args: { variant: 'info', children: 'This is an informational message.' },
};

export const Success: Story = {
  args: { variant: 'success', children: 'Agent connected successfully.' },
};

export const Warning: Story = {
  args: { variant: 'warning', children: 'Connection may be unstable.' },
};

export const Danger: Story = {
  args: { variant: 'danger', children: 'Failed to connect to agent.' },
};

export const WithTitle: Story = {
  args: {
    variant: 'danger',
    title: 'Connection Error',
    children: 'Could not reach the gateway server. Please check your network connection and try again.',
  },
};

export const Dismissible: Story = {
  args: {
    variant: 'info',
    dismissible: true,
    children: 'You can dismiss this alert by clicking the X button.',
  },
};

export const LongContent: Story = {
  args: {
    variant: 'warning',
    title: 'Rate Limit Warning',
    children: 'You are approaching your API rate limit. Current usage: 950/1000 requests per minute. Consider reducing request frequency or upgrading your plan.',
  },
};
