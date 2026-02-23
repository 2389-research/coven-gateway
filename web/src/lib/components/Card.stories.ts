import type { Meta, StoryObj } from '@storybook/svelte';
import Card from './Card.svelte';
import { textSnippet, htmlSnippet } from './_storyHelpers';

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

export const WithHeader: Story = {
  args: {
    header: textSnippet('Card Title'),
    children: textSnippet('Card body content below the header divider.'),
  },
};

export const WithHeaderAndFooter: Story = {
  args: {
    header: textSnippet('Settings'),
    children: textSnippet('Configure your preferences in this section.'),
    footer: htmlSnippet(
      '<div style="display:flex;justify-content:flex-end;gap:8px"><button type="button" style="padding:4px 12px">Cancel</button><button type="button" style="padding:4px 12px;background:#3b82f6;color:white;border-radius:4px">Save</button></div>',
    ),
  },
};
