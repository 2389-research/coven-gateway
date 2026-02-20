import type { Meta, StoryObj } from '@storybook/svelte';
import CopyButton from './CopyButton.svelte';

const meta = {
  title: 'Actions/CopyButton',
  component: CopyButton,
  tags: ['autodocs'],
  argTypes: {
    value: { control: 'text' },
    label: { control: 'text' },
    copiedLabel: { control: 'text' },
    disabled: { control: 'boolean' },
  },
} satisfies Meta<CopyButton>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: { value: 'sk-1234-abcd-5678' },
};

export const CustomLabel: Story = {
  args: { value: 'https://gateway.example.com/invite/abc123', label: 'Copy link', copiedLabel: 'Link copied!' },
};

export const Disabled: Story = {
  args: { value: 'nothing-to-copy', disabled: true },
};

export const LongValue: Story = {
  args: { value: 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U' },
};
