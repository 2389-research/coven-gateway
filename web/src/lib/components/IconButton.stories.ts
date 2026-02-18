import type { Meta, StoryObj } from '@storybook/svelte';
import IconButton from './IconButton.svelte';

const meta = {
  title: 'Inputs/IconButton',
  component: IconButton,
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
    disabled: { control: 'boolean' },
  },
} satisfies Meta<IconButton>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Ghost: Story = {
  args: { variant: 'ghost', 'aria-label': 'Settings' },
};

export const Primary: Story = {
  args: { variant: 'primary', 'aria-label': 'Add item' },
};

export const Secondary: Story = {
  args: { variant: 'secondary', 'aria-label': 'Edit' },
};

export const Danger: Story = {
  args: { variant: 'danger', 'aria-label': 'Delete' },
};

export const Small: Story = {
  args: { variant: 'ghost', size: 'sm', 'aria-label': 'Close' },
};

export const Large: Story = {
  args: { variant: 'ghost', size: 'lg', 'aria-label': 'Menu' },
};

export const Disabled: Story = {
  args: { variant: 'ghost', disabled: true, 'aria-label': 'Locked' },
};
