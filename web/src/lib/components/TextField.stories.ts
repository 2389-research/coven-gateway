import type { Meta, StoryObj } from '@storybook/svelte';
import TextField from './TextField.svelte';
import { htmlSnippet } from './_storyHelpers';

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

export const WithLeadingIcon: Story = {
  args: {
    label: 'Search',
    placeholder: 'Search agents...',
    leading: htmlSnippet(
      '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>',
    ),
  },
};

export const WithTrailingIcon: Story = {
  args: {
    label: 'Password',
    type: 'password',
    placeholder: 'Enter password',
    trailing: htmlSnippet(
      '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z"/><circle cx="12" cy="12" r="3"/></svg>',
    ),
  },
};
