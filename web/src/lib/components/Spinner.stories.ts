// ABOUTME: Demonstrates the Spinner component in Storybook.
// ABOUTME: Defines spinner variants and controls for size and accessible labels.
import type { Meta, StoryObj } from '@storybook/svelte';
import Spinner from './Spinner.svelte';

const meta = {
  title: 'Feedback/Spinner',
  component: Spinner,
  tags: ['autodocs'],
  argTypes: {
    size: {
      control: 'select',
      options: ['sm', 'md', 'lg'],
    },
    label: { control: 'text' },
  },
} satisfies Meta<typeof Spinner>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {},
};

export const Small: Story = {
  args: { size: 'sm' },
};

export const Large: Story = {
  args: { size: 'lg' },
};

export const CustomLabel: Story = {
  args: { label: 'Connecting to agent...' },
};
