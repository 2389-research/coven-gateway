import type { Meta, StoryObj } from '@storybook/svelte';
import TextArea from './TextArea.svelte';

const meta = {
  title: 'Inputs/TextArea',
  component: TextArea,
  tags: ['autodocs'],
  argTypes: {
    label: { control: 'text' },
    error: { control: 'text' },
    placeholder: { control: 'text' },
    autoResize: { control: 'boolean' },
    disabled: { control: 'boolean' },
    rows: { control: 'number' },
  },
} satisfies Meta<TextArea>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: { label: 'Message', placeholder: 'Type your message...' },
};

export const WithValue: Story = {
  args: { label: 'Bio', value: 'Software engineer focused on distributed systems.' },
};

export const AutoResize: Story = {
  args: { label: 'Notes', placeholder: 'Start typing to auto-expand...', autoResize: true, rows: 1 },
};

export const WithError: Story = {
  args: { label: 'Message', value: '', error: 'Message is required' },
};

export const Disabled: Story = {
  args: { label: 'Read Only', value: 'This field is disabled', disabled: true },
};

export const LongContent: Story = {
  args: {
    label: 'Description',
    value: 'Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris.',
    rows: 5,
  },
};
