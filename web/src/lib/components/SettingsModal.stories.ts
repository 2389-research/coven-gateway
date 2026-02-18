import type { Meta, StoryObj } from '@storybook/svelte';
import SettingsModal from './SettingsModal.svelte';

const meta: Meta<SettingsModal> = {
  title: 'Chat/SettingsModal',
  component: SettingsModal,
  parameters: { layout: 'centered' },
  tags: ['autodocs'],
};

export default meta;
type Story = StoryObj<SettingsModal>;

export const Open: Story = {
  args: {
    open: true,
    csrfToken: 'demo-csrf-token',
  },
};

export const Closed: Story = {
  args: {
    open: false,
    csrfToken: 'demo-csrf-token',
  },
};
