import type { Meta, StoryObj } from '@storybook/svelte';
import ThinkingIndicator from './ThinkingIndicator.svelte';

const meta = {
  title: 'Chat/ThinkingIndicator',
  component: ThinkingIndicator,
  tags: ['autodocs'],
  parameters: { layout: 'padded' },
  argTypes: {
    text: { control: 'text' },
  },
} satisfies Meta<ThinkingIndicator>;

export default meta;
type Story = StoryObj<typeof meta>;

export const DotsOnly: Story = {
  args: {},
};

export const WithText: Story = {
  args: {
    text: 'Analyzing the current authentication flow and identifying key components...',
  },
};

export const ShortText: Story = {
  args: {
    text: 'Thinking...',
  },
};

export const LongText: Story = {
  args: {
    text: 'Let me carefully review the entire codebase structure, analyze the dependency graph, check for circular imports, and then propose a refactoring strategy that minimizes breaking changes while improving modularity.',
  },
};
