import type { Meta, StoryObj } from '@storybook/svelte';
import Stack from './Stack.svelte';
import { htmlSnippet } from './_storyHelpers';

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

const stackItems = htmlSnippet(
  '<div style="background:#e2e8f0;padding:8px 16px;border-radius:6px">Item 1</div>' +
  '<div style="background:#e2e8f0;padding:8px 16px;border-radius:6px">Item 2</div>' +
  '<div style="background:#e2e8f0;padding:8px 16px;border-radius:6px">Item 3</div>',
);

export const Vertical: Story = {
  args: {
    direction: 'vertical',
    gap: '4',
    children: stackItems,
  },
};

export const Horizontal: Story = {
  args: {
    direction: 'horizontal',
    gap: '4',
    children: stackItems,
  },
};

export const Centered: Story = {
  args: {
    direction: 'horizontal',
    gap: '4',
    align: 'center',
    justify: 'center',
    children: stackItems,
  },
};

export const SpaceBetween: Story = {
  args: {
    direction: 'horizontal',
    justify: 'between',
    children: stackItems,
  },
};
