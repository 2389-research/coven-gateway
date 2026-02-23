import type { Meta, StoryObj } from '@storybook/svelte';
import ChatMessage from './ChatMessage.svelte';
import type { ChatMessage as ChatMessageType } from '../types/chat';

function msg(overrides: Partial<ChatMessageType> & Pick<ChatMessageType, 'type'>): ChatMessageType {
  return {
    id: Math.random().toString(36).slice(2),
    content: '',
    timestamp: new Date('2026-02-18T14:30:00'),
    ...overrides,
  };
}

const meta = {
  title: 'Chat/ChatMessage',
  component: ChatMessage,
  tags: ['autodocs'],
  parameters: { layout: 'padded' },
} satisfies Meta<ChatMessage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const UserMessage: Story = {
  args: {
    message: msg({
      type: 'user',
      content: 'Can you help me refactor the authentication module?',
    }),
  },
};

export const AgentText: Story = {
  args: {
    message: msg({
      type: 'text',
      content:
        "Sure! I'll look at the authentication module. Here's what I suggest:\n\n1. Extract the JWT validation into a separate middleware\n2. Use `interface`-driven design for the token store\n3. Add proper error wrapping with `fmt.Errorf`\n\n```go\nfunc ValidateToken(token string) (*Claims, error) {\n    // ...\n}\n```\n\nThis keeps things **clean** and *testable*.",
    }),
  },
};

export const Thinking: Story = {
  args: {
    message: msg({
      type: 'thinking',
      content: 'Let me analyze the current authentication flow and identify the key components that need refactoring...',
    }),
  },
};

export const ToolUse: Story = {
  args: {
    message: msg({
      type: 'tool_use',
      toolName: 'read_file',
      toolId: 'tool_123',
      inputJson: JSON.stringify({ path: 'internal/auth/jwt.go', lines: '1-50' }),
    }),
  },
};

export const ToolResult: Story = {
  args: {
    message: msg({
      type: 'tool_result',
      toolId: 'tool_123',
      content: 'package auth\n\nimport (\n\t"crypto/rsa"\n\t"fmt"\n\t"time"\n)\n\ntype Claims struct {\n\tSubject   string\n\tExpiresAt time.Time\n}',
    }),
  },
};

export const ErrorMessage: Story = {
  args: {
    message: msg({
      type: 'error',
      content: 'Connection to agent lost. Retrying in 5 seconds...',
    }),
  },
};

export const CanceledMessage: Story = {
  args: {
    message: msg({
      type: 'canceled',
      reason: 'User interrupted',
    }),
  },
};

export const CanceledNoReason: Story = {
  args: {
    message: msg({
      type: 'canceled',
    }),
  },
};

export const AgentWithLinks: Story = {
  args: {
    message: msg({
      type: 'text',
      content:
        'Check the [Go documentation](https://go.dev/doc/) for more details on error handling patterns.\n\n> Always wrap errors with context using `fmt.Errorf("operation: %w", err)`',
    }),
  },
};

export const LongCodeBlock: Story = {
  args: {
    message: msg({
      type: 'text',
      content:
        'Here\'s the full implementation:\n\n```go\npackage auth\n\nimport (\n\t"crypto/rsa"\n\t"fmt"\n\t"time"\n\n\t"github.com/golang-jwt/jwt/v5"\n)\n\ntype TokenValidator struct {\n\tpublicKey *rsa.PublicKey\n}\n\nfunc NewTokenValidator(key *rsa.PublicKey) *TokenValidator {\n\treturn &TokenValidator{publicKey: key}\n}\n\nfunc (v *TokenValidator) Validate(tokenStr string) (*Claims, error) {\n\ttoken, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {\n\t\tif _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {\n\t\t\treturn nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])\n\t\t}\n\t\treturn v.publicKey, nil\n\t})\n\tif err != nil {\n\t\treturn nil, fmt.Errorf("parse token: %w", err)\n\t}\n\t// ...\n\treturn &Claims{}, nil\n}\n```',
    }),
  },
};
