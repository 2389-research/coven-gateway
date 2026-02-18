import type { Meta, StoryObj } from '@storybook/svelte';
import Button from './Button.svelte';
import { textSnippet } from './_storyHelpers';

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
  args: { variant: 'primary', children: textSnippet('Primary Button') },
};

export const Secondary: Story = {
  args: { variant: 'secondary', children: textSnippet('Secondary Button') },
};

export const Ghost: Story = {
  args: { variant: 'ghost', children: textSnippet('Ghost Button') },
};

export const Danger: Story = {
  args: { variant: 'danger', children: textSnippet('Delete') },
};

export const Small: Story = {
  args: { variant: 'primary', size: 'sm', children: textSnippet('Small') },
};

export const Large: Story = {
  args: { variant: 'primary', size: 'lg', children: textSnippet('Large Button') },
};

export const Loading: Story = {
  args: { variant: 'primary', loading: true, children: textSnippet('Saving...') },
};

export const Disabled: Story = {
  args: { variant: 'primary', disabled: true, children: textSnippet('Disabled') },
};

export const LongText: Story = {
  args: { variant: 'primary', children: textSnippet('This is a button with very long text content') },
};
