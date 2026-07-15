// ABOUTME: Demonstrates the ChatInput component in Storybook.
// ABOUTME: Defines chat-input variants and controls for messaging interaction states.
import type { Meta, StoryObj } from '@storybook/svelte';
import ChatInput from './ChatInput.svelte';

const meta = {
  title: 'Chat/ChatInput',
  component: ChatInput,
  tags: ['autodocs'],
  args: {
    onSend: (text: string) => console.log('[ChatInput] send:', text),
  },
  argTypes: {
    disabled: { control: 'boolean' },
    maxLength: { control: 'number' },
    placeholder: { control: 'text' },
  },
  parameters: { layout: 'padded' },
} satisfies Meta<typeof ChatInput>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {},
};

export const Disabled: Story = {
  args: {
    disabled: true,
  },
};

export const CustomPlaceholder: Story = {
  args: {
    placeholder: 'Ask the agent anything...',
  },
};

export const ShortMaxLength: Story = {
  args: {
    maxLength: 100,
  },
};
