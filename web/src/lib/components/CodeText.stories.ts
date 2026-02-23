import type { Meta, StoryObj } from '@storybook/svelte';
import CodeText from './CodeText.svelte';
import { textSnippet } from './_storyHelpers';

const meta = {
  title: 'Data Display/CodeText',
  component: CodeText,
  tags: ['autodocs'],
} satisfies Meta<CodeText>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: { children: textSnippet('agent-abc123') },
};

export const FilePath: Story = {
  args: { children: textSnippet('/var/lib/coven/workspace') },
};

export const UUID: Story = {
  args: { children: textSnippet('550e8400-e29b-41d4-a716-446655440000') },
};

export const Short: Story = {
  args: { children: textSnippet('v2.1.0') },
};
