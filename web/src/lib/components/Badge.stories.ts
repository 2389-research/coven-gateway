import type { Meta, StoryObj } from '@storybook/svelte';
import Badge from './Badge.svelte';

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
  },
} satisfies Meta<Badge>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: { variant: 'default', children: 'Default' },
};

export const Accent: Story = {
  args: { variant: 'accent', children: 'Active' },
};

export const Success: Story = {
  args: { variant: 'success', children: 'Online' },
};

export const Warning: Story = {
  args: { variant: 'warning', children: 'Busy' },
};

export const Danger: Story = {
  args: { variant: 'danger', children: 'Error' },
};

export const Small: Story = {
  args: { variant: 'accent', size: 'sm', children: 'v1.2.3' },
};

export const LongText: Story = {
  args: { variant: 'default', children: 'Long badge text content here' },
};
