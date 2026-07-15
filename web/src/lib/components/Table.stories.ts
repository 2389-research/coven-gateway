// ABOUTME: Demonstrates the Table demo component in Storybook.
// ABOUTME: Defines table variants and controls for representative data states.
import type { Meta, StoryObj } from '@storybook/svelte';
import TableDemo from './_TableDemo.svelte';

const meta = {
  title: 'Data Display/Table',
  component: TableDemo,
  tags: ['autodocs'],
  argTypes: {
    variant: {
      control: 'select',
      options: ['basic', 'actions', 'empty', 'aligned'],
    },
  },
} satisfies Meta<typeof TableDemo>;

export default meta;
type Story = StoryObj<typeof meta>;

export const BasicTable: Story = {
  args: { variant: 'basic' },
};

export const WithActions: Story = {
  args: { variant: 'actions' },
};

export const EmptyTable: Story = {
  args: { variant: 'empty' },
};

export const AlignedColumns: Story = {
  args: { variant: 'aligned' },
};
