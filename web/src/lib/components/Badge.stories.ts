import type { Meta, StoryObj } from '@storybook/svelte';
import Badge from './Badge.svelte';
import { textSnippet } from './_storyHelpers';

const meta = {
  title: 'Data Display/Badge',
  component: Badge,
  tags: ['autodocs'],
  argTypes: {
    variant: {
      control: 'select',
      options: ['default', 'accent', 'success', 'warning', 'danger'],
    },
    size: {
      control: 'select',
      options: ['sm', 'md'],
    },
    fill: {
      control: 'select',
      options: ['solid', 'outline'],
    },
  },
} satisfies Meta<Badge>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: { variant: 'default', children: textSnippet('Default') },
};

export const Accent: Story = {
  args: { variant: 'accent', children: textSnippet('Active') },
};

export const Success: Story = {
  args: { variant: 'success', children: textSnippet('Online') },
};

export const Warning: Story = {
  args: { variant: 'warning', children: textSnippet('Busy') },
};

export const Danger: Story = {
  args: { variant: 'danger', children: textSnippet('Error') },
};

export const Small: Story = {
  args: { variant: 'accent', size: 'sm', children: textSnippet('v1.2.3') },
};

export const LongText: Story = {
  args: { variant: 'default', children: textSnippet('Long badge text content here') },
};

export const OutlineDefault: Story = {
  args: { variant: 'default', fill: 'outline', children: textSnippet('Default') },
};

export const OutlineAccent: Story = {
  args: { variant: 'accent', fill: 'outline', children: textSnippet('Type: Agent') },
};

export const OutlineSuccess: Story = {
  args: { variant: 'success', fill: 'outline', children: textSnippet('Scope: Read') },
};

export const OutlineWarning: Story = {
  args: { variant: 'warning', fill: 'outline', children: textSnippet('Expiring') },
};

export const OutlineDanger: Story = {
  args: { variant: 'danger', fill: 'outline', children: textSnippet('Revoked') },
};

export const OutlineSmall: Story = {
  args: { variant: 'accent', fill: 'outline', size: 'sm', children: textSnippet('v2.0') },
};
