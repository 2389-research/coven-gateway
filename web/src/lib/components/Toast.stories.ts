import type { Meta, StoryObj } from '@storybook/svelte';
import Toast from './Toast.svelte';
import { addToast } from '../stores/toast.svelte';

const meta = {
  title: 'Feedback/Toast',
  component: Toast,
  tags: ['autodocs'],
  parameters: {
    layout: 'fullscreen',
  },
} satisfies Meta<Toast>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  play: () => {
    addToast('This is an info toast', 'info');
  },
};

export const AllVariants: Story = {
  play: () => {
    addToast('Info notification', 'info', 30000);
    addToast('Success — agent connected', 'success', 30000);
    addToast('Warning — rate limit approaching', 'warning', 30000);
    addToast('Error — connection failed', 'danger', 30000);
  },
};
