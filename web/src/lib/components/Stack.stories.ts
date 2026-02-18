import type { Meta, StoryObj } from '@storybook/svelte';
import Stack from './Stack.svelte';

const meta = {
  title: 'Layout/Stack',
  component: Stack,
  tags: ['autodocs'],
  argTypes: {
    direction: {
      control: 'select',
      options: ['vertical', 'horizontal'],
    },
    gap: {
      control: 'select',
      options: ['0', '1', '2', '3', '4', '6', '8', '12', '16'],
    },
    align: {
      control: 'select',
      options: ['start', 'center', 'end', 'stretch'],
    },
    justify: {
      control: 'select',
      options: ['start', 'center', 'end', 'between'],
    },
    wrap: { control: 'boolean' },
  },
} satisfies Meta<Stack>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Vertical: Story = {
  args: {
    direction: 'vertical',
    gap: '4',
    children: 'Stack items go here',
  },
};

export const Horizontal: Story = {
  args: {
    direction: 'horizontal',
    gap: '4',
    children: 'Horizontal stack items',
  },
};

export const Centered: Story = {
  args: {
    direction: 'horizontal',
    gap: '4',
    align: 'center',
    justify: 'center',
    children: 'Centered content',
  },
};

export const SpaceBetween: Story = {
  args: {
    direction: 'horizontal',
    justify: 'between',
    children: 'Space between items',
  },
};
