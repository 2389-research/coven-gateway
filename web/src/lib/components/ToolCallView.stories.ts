import type { Meta, StoryObj } from '@storybook/svelte';
import ToolCallView from './ToolCallView.svelte';

const meta = {
  title: 'Chat/ToolCallView',
  component: ToolCallView,
  tags: ['autodocs'],
  parameters: { layout: 'padded' },
  argTypes: {
    variant: { control: 'select', options: ['call', 'result'] },
    expanded: { control: 'boolean' },
  },
} satisfies Meta<ToolCallView>;

export default meta;
type Story = StoryObj<typeof meta>;

export const ToolCall: Story = {
  args: {
    variant: 'call',
    toolName: 'read_file',
    content: JSON.stringify({ path: 'internal/auth/jwt.go', lines: '1-50' }),
  },
};

export const ToolCallExpanded: Story = {
  args: {
    variant: 'call',
    toolName: 'write_file',
    content: JSON.stringify({ path: 'config.yaml', content: 'server:\n  port: 9090\n  host: localhost' }),
    expanded: true,
  },
};

export const ToolResult: Story = {
  args: {
    variant: 'result',
    content: 'package auth\n\nimport (\n\t"crypto/rsa"\n\t"fmt"\n\t"time"\n)\n\ntype Claims struct {\n\tSubject   string\n\tExpiresAt time.Time\n}',
  },
};

export const ToolResultExpanded: Story = {
  args: {
    variant: 'result',
    content: 'File written successfully.',
    expanded: true,
  },
};

export const NoToolName: Story = {
  args: {
    variant: 'call',
    content: '{"query": "SELECT * FROM users"}',
  },
};
