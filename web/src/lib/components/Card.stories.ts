import type { Meta, StoryObj } from '@storybook/svelte';
import Card from './Card.svelte';
import { textSnippet } from './_storyHelpers';

const meta = {
  title: 'Layout/Card',
  component: Card,
  tags: ['autodocs'],
  argTypes: {
    padding: {
      control: 'select',
      options: ['none', 'sm', 'md', 'lg'],
    },
  },
} satisfies Meta<Card>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    children: textSnippet('Card content goes here. This is a basic surface container.'),
  },
};

export const SmallPadding: Story = {
  args: {
    padding: 'sm',
    children: textSnippet('Compact card with small padding.'),
  },
};

export const LargePadding: Story = {
  args: {
    padding: 'lg',
    children: textSnippet('Spacious card with large padding.'),
  },
};

export const NoPadding: Story = {
  args: {
    padding: 'none',
    children: textSnippet('Card with no padding — useful for full-bleed content.'),
  },
};
