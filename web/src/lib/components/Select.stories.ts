import type { Meta, StoryObj } from '@storybook/svelte';
import Select from './Select.svelte';

const sampleOptions = [
  { value: 'agent', label: 'Agent' },
  { value: 'human', label: 'Human' },
  { value: 'system', label: 'System' },
];

const meta = {
  title: 'Inputs/Select',
  component: Select,
  tags: ['autodocs'],
  argTypes: {
    label: { control: 'text' },
    error: { control: 'text' },
    placeholder: { control: 'text' },
    disabled: { control: 'boolean' },
  },
} satisfies Meta<Select>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: { options: sampleOptions },
};

export const WithLabel: Story = {
  args: { label: 'Principal Type', options: sampleOptions },
};

export const WithError: Story = {
  args: {
    label: 'Scope',
    options: sampleOptions,
    error: 'Please select a valid scope',
  },
};

export const WithPlaceholder: Story = {
  args: {
    label: 'Filter by type',
    options: sampleOptions,
    placeholder: 'All Types',
  },
};

export const Disabled: Story = {
  args: {
    label: 'Locked',
    options: sampleOptions,
    disabled: true,
  },
};
