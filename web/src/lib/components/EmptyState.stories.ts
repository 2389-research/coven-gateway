import type { Meta, StoryObj } from '@storybook/svelte';
import EmptyState from './EmptyState.svelte';
import { htmlSnippet, textSnippet } from './_storyHelpers';

const meta = {
  title: 'Data Display/EmptyState',
  component: EmptyState,
  tags: ['autodocs'],
  argTypes: {
    heading: { control: 'text' },
    description: { control: 'text' },
  },
} satisfies Meta<EmptyState>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    heading: 'No agents found',
    description: 'Connect an agent to get started.',
    icon: htmlSnippet(
      '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><rect width="18" height="18" x="3" y="3" rx="2"/><path d="M3 9h18"/><path d="M9 21V9"/></svg>',
    ),
  },
};

export const WithAction: Story = {
  args: {
    heading: 'No principals',
    description: 'Create a principal to manage access.',
    icon: htmlSnippet(
      '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><line x1="19" x2="19" y1="8" y2="14"/><line x1="22" x2="16" y1="11" y2="11"/></svg>',
    ),
    action: htmlSnippet(
      '<button type="button" class="px-4 py-2 rounded-md bg-accentSolidBg text-white text-sm font-medium">Create Principal</button>',
    ),
  },
};

export const NoDescription: Story = {
  args: {
    heading: 'No results',
    icon: htmlSnippet(
      '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>',
    ),
  },
};

export const NoIcon: Story = {
  args: {
    heading: 'Nothing here yet',
    description: 'This section is empty. Add some data to see it here.',
  },
};
