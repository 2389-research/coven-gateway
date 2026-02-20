import type { Meta, StoryObj } from '@storybook/svelte';
import ChatThread from './ChatThread.svelte';
import type { ChatMessage } from '../types/chat';

let idCounter = 0;
function msg(overrides: Partial<ChatMessage> & Pick<ChatMessage, 'type'>): ChatMessage {
  return {
    id: `msg-${++idCounter}`,
    content: '',
    timestamp: new Date('2026-02-18T14:30:00'),
    ...overrides,
  };
}

const sampleConversation: ChatMessage[] = [
  msg({ type: 'user', content: 'Can you help me refactor the authentication module?', timestamp: new Date('2026-02-18T14:30:00') }),
  msg({ type: 'thinking', content: 'Let me analyze the current auth flow...', timestamp: new Date('2026-02-18T14:30:02') }),
  msg({
    type: 'tool_use',
    toolName: 'read_file',
    toolId: 'tool_1',
    inputJson: JSON.stringify({ path: 'internal/auth/jwt.go' }),
    timestamp: new Date('2026-02-18T14:30:05'),
  }),
  msg({
    type: 'tool_result',
    toolId: 'tool_1',
    content: 'package auth\n\nimport "time"\n\ntype Claims struct {\n\tSubject string\n\tExpiry  time.Time\n}',
    timestamp: new Date('2026-02-18T14:30:06'),
  }),
  msg({
    type: 'text',
    content: "I've reviewed the auth module. Here's what I recommend:\n\n1. **Extract JWT validation** into middleware\n2. Use an `interface` for the token store\n3. Add `fmt.Errorf` wrapping\n\n```go\nfunc ValidateToken(t string) (*Claims, error) {\n    // validation logic\n}\n```",
    timestamp: new Date('2026-02-18T14:30:10'),
  }),
  msg({ type: 'user', content: 'That looks great, please proceed!', timestamp: new Date('2026-02-18T14:31:00') }),
];

const multiDayConversation: ChatMessage[] = [
  msg({ type: 'user', content: 'Starting the refactor today.', timestamp: new Date('2026-02-17T09:00:00') }),
  msg({ type: 'text', content: 'Sounds good! Let me know when you need help.', timestamp: new Date('2026-02-17T09:01:00') }),
  msg({ type: 'user', content: "I'm back. The tests are failing after the change.", timestamp: new Date('2026-02-18T10:00:00') }),
  msg({ type: 'text', content: "Let me take a look at the test output. Can you share the error?", timestamp: new Date('2026-02-18T10:00:30') }),
  msg({ type: 'error', content: 'Connection to agent lost. Retrying...', timestamp: new Date('2026-02-18T10:01:00') }),
  msg({ type: 'text', content: "I'm back online. Let's continue debugging.", timestamp: new Date('2026-02-18T10:01:30') }),
];

const meta = {
  title: 'Chat/ChatThread',
  component: ChatThread,
  tags: ['autodocs'],
  parameters: { layout: 'padded' },
} satisfies Meta<ChatThread>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    messages: sampleConversation,
    class: 'h-full',
  },
};

export const MultiDay: Story = {
  args: {
    messages: multiDayConversation,
    class: 'h-full',
  },
};

export const Empty: Story = {
  args: {
    messages: [],
    class: 'h-full',
  },
};

/** Generate many messages to test scroll behavior */
function generateLongThread(count: number): ChatMessage[] {
  const msgs: ChatMessage[] = [];
  for (let i = 0; i < count; i++) {
    const isUser = i % 3 === 0;
    msgs.push(
      msg({
        type: isUser ? 'user' : 'text',
        content: isUser
          ? `User message #${i + 1}`
          : `Agent response to message #${i}. This is a longer message with some detail to make the thread more realistic.`,
        timestamp: new Date(2026, 1, 18, 10, Math.floor(i / 2), i % 60),
      }),
    );
  }
  return msgs;
}

export const LongThread: Story = {
  args: {
    messages: generateLongThread(50),
    class: 'h-full',
  },
};

export const WithToolCalls: Story = {
  args: {
    messages: [
      msg({ type: 'user', content: 'Read the config file', timestamp: new Date('2026-02-18T14:00:00') }),
      msg({ type: 'tool_use', toolName: 'read_file', toolId: 't1', inputJson: '{"path":"config.yaml"}', timestamp: new Date('2026-02-18T14:00:01') }),
      msg({ type: 'tool_result', toolId: 't1', content: 'server:\n  port: 8080\n  host: localhost', timestamp: new Date('2026-02-18T14:00:02') }),
      msg({ type: 'tool_use', toolName: 'write_file', toolId: 't2', inputJson: '{"path":"config.yaml","content":"server:\\n  port: 9090"}', timestamp: new Date('2026-02-18T14:00:03') }),
      msg({ type: 'tool_result', toolId: 't2', content: 'File written successfully', timestamp: new Date('2026-02-18T14:00:04') }),
      msg({ type: 'text', content: "Done! I've updated the port from `8080` to `9090`.", timestamp: new Date('2026-02-18T14:00:05') }),
    ],
    class: 'h-full',
  },
};
