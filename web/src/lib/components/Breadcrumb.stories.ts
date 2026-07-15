// ABOUTME: Demonstrates the Breadcrumb demo component in Storybook.
// ABOUTME: Defines breadcrumb hierarchy variants and controls for navigation states.
import type { Meta, StoryObj } from '@storybook/svelte';
import _BreadcrumbDemo from './_BreadcrumbDemo.svelte';

const meta = {
  title: 'Navigation/Breadcrumb',
  component: _BreadcrumbDemo,
  tags: ['autodocs'],
  argTypes: {
    variant: {
      control: 'select',
      options: ['two-level', 'three-level', 'current-only'],
    },
  },
} satisfies Meta<typeof _BreadcrumbDemo>;

export default meta;
type Story = StoryObj<typeof meta>;

export const TwoLevel: Story = {
  args: { variant: 'two-level' },
};

export const ThreeLevel: Story = {
  args: { variant: 'three-level' },
};

export const CurrentOnly: Story = {
  args: { variant: 'current-only' },
};
