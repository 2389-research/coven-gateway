import type { Meta, StoryObj } from '@storybook/svelte';
import Button from './Button.svelte';

const meta = {
  title: 'Inputs/Button',
  component: Button,
  tags: ['autodocs'],
  argTypes: {
    variant: {
      control: 'select',
      options: ['primary', 'secondary', 'ghost', 'danger'],
    },
    size: {
      control: 'select',
      options: ['sm', 'md', 'lg'],
    },
    loading: { control: 'boolean' },
    disabled: { control: 'boolean' },
  },
} satisfies Meta<Button>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Primary: Story = {
  args: { variant: 'primary', children: 'Primary Button' },
};

export const Secondary: Story = {
  args: { variant: 'secondary', children: 'Secondary Button' },
};

export const Ghost: Story = {
  args: { variant: 'ghost', children: 'Ghost Button' },
};

export const Danger: Story = {
  args: { variant: 'danger', children: 'Delete' },
};

export const Small: Story = {
  args: { variant: 'primary', size: 'sm', children: 'Small' },
};

export const Large: Story = {
  args: { variant: 'primary', size: 'lg', children: 'Large Button' },
};

export const Loading: Story = {
  args: { variant: 'primary', loading: true, children: 'Saving...' },
};

export const Disabled: Story = {
  args: { variant: 'primary', disabled: true, children: 'Disabled' },
};

export const LongText: Story = {
  args: { variant: 'primary', children: 'This is a button with very long text content' },
};
