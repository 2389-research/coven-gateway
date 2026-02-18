import type { Meta, StoryObj } from '@storybook/svelte';
import TextField from './TextField.svelte';

const meta = {
  title: 'Inputs/TextField',
  component: TextField,
  tags: ['autodocs'],
  argTypes: {
    label: { control: 'text' },
    error: { control: 'text' },
    placeholder: { control: 'text' },
    disabled: { control: 'boolean' },
  },
} satisfies Meta<TextField>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: { label: 'Email', placeholder: 'you@example.com' },
};

export const WithValue: Story = {
  args: { label: 'Name', value: 'Jane Doe' },
};

export const WithError: Story = {
  args: { label: 'Email', value: 'not-an-email', error: 'Please enter a valid email address' },
};

export const Disabled: Story = {
  args: { label: 'Locked Field', value: 'Cannot edit', disabled: true },
};

export const NoLabel: Story = {
  args: { placeholder: 'Search...' },
};

export const LongError: Story = {
  args: {
    label: 'Username',
    value: 'x',
    error: 'Username must be between 3 and 20 characters and contain only letters, numbers, and underscores',
  },
};
