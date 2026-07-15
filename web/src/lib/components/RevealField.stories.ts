// ABOUTME: Demonstrates the RevealField component in Storybook.
// ABOUTME: Defines reveal-field variants and controls for concealed values and labels.
import type { Meta, StoryObj } from '@storybook/svelte';
import RevealField from './RevealField.svelte';

const meta = {
  title: 'Data Display/RevealField',
  component: RevealField,
  tags: ['autodocs'],
  argTypes: {
    value: { control: 'text' },
    mask: { control: 'text' },
  },
} satisfies Meta<typeof RevealField>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: { value: 'super-secret-api-key-12345' },
};

export const CustomMask: Story = {
  args: { value: 'password123', mask: '********' },
};

export const ShortValue: Story = {
  args: { value: 'abc' },
};

export const LongValue: Story = {
  args: { value: 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0' },
};
