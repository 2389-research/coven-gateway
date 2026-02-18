import type { Meta, StoryObj } from '@storybook/svelte';
import Dialog from './Dialog.svelte';

const meta = {
  title: 'Overlays/Dialog',
  component: Dialog,
  tags: ['autodocs'],
  argTypes: {
    open: { control: 'boolean' },
  },
} satisfies Meta<Dialog>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    open: true,
    children: 'This is dialog content. Press Escape or click the backdrop to close.',
  },
};

export const Closed: Story = {
  args: {
    open: false,
    children: 'This dialog is closed. Toggle the open control to show it.',
  },
};
