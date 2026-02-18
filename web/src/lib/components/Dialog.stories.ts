import type { Meta, StoryObj } from '@storybook/svelte';
import Dialog from './Dialog.svelte';
import { textSnippet, htmlSnippet } from './_storyHelpers';

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
    header: textSnippet('Confirm Action'),
    children: textSnippet('This is dialog content. Press Escape or click the backdrop to close.'),
  },
};

export const WithFooter: Story = {
  args: {
    open: true,
    header: textSnippet('Delete Agent'),
    children: textSnippet('Are you sure you want to delete this agent? This action cannot be undone.'),
    footer: htmlSnippet('<div style="display:flex;gap:8px;justify-content:flex-end"><button style="padding:6px 16px;border-radius:6px;border:1px solid #e2e8f0">Cancel</button><button style="padding:6px 16px;border-radius:6px;background:#dc2626;color:white;border:none">Delete</button></div>'),
  },
};

export const Closed: Story = {
  args: {
    open: false,
    children: textSnippet('This dialog is closed. Toggle the open control to show it.'),
  },
};
