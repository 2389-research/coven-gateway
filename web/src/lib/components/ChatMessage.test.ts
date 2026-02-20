import { render, screen } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import ChatMessage from './ChatMessage.svelte';
import type { ChatMessage as ChatMessageType } from '../types/chat';

function msg(overrides: Partial<ChatMessageType> & Pick<ChatMessageType, 'type'>): ChatMessageType {
  return {
    id: 'test-1',
    content: '',
    timestamp: new Date('2026-02-18T14:30:00'),
    ...overrides,
  };
}

describe('ChatMessage', () => {
  it('renders user message right-aligned', () => {
    render(ChatMessage, {
      props: { message: msg({ type: 'user', content: 'Hello agent' }) },
    });
    const el = screen.getByTestId('chat-message');
    expect(el.getAttribute('data-message-type')).toBe('user');
    expect(el.className).toContain('justify-end');
    expect(el.textContent).toContain('Hello agent');
    expect(el.textContent).toContain('You');
  });

  it('renders agent text message left-aligned with markdown', () => {
    render(ChatMessage, {
      props: { message: msg({ type: 'text', content: 'Hello **bold**' }) },
    });
    const el = screen.getByTestId('chat-message');
    expect(el.getAttribute('data-message-type')).toBe('text');
    expect(el.className).toContain('justify-start');
    expect(el.textContent).toContain('Agent');
    // Markdown should be rendered (bold tag becomes text)
    expect(el.innerHTML).toContain('<strong>bold</strong>');
  });

  it('sanitizes dangerous HTML in markdown', () => {
    render(ChatMessage, {
      props: {
        message: msg({
          type: 'text',
          content: '<script>alert("xss")</script>Safe text',
        }),
      },
    });
    const el = screen.getByTestId('chat-message');
    expect(el.innerHTML).not.toContain('<script>');
    expect(el.textContent).toContain('Safe text');
  });

  it('renders thinking message with ThinkingIndicator', () => {
    render(ChatMessage, {
      props: { message: msg({ type: 'thinking', content: 'Analyzing...' }) },
    });
    const el = screen.getByTestId('chat-message');
    expect(el.getAttribute('data-message-type')).toBe('thinking');
    expect(screen.getByTestId('thinking-indicator')).toBeTruthy();
    expect(el.textContent).toContain('Analyzing...');
  });

  it('renders tool_use with ToolCallView', () => {
    render(ChatMessage, {
      props: {
        message: msg({
          type: 'tool_use',
          toolName: 'read_file',
          inputJson: '{"path":"test.go"}',
        }),
      },
    });
    const el = screen.getByTestId('chat-message');
    expect(el.getAttribute('data-message-type')).toBe('tool_use');
    const toolView = screen.getByTestId('tool-call-view');
    expect(toolView.getAttribute('data-variant')).toBe('call');
    expect(el.textContent).toContain('read_file');
  });

  it('renders tool_result with ToolCallView', () => {
    render(ChatMessage, {
      props: {
        message: msg({ type: 'tool_result', content: 'file contents here' }),
      },
    });
    const toolView = screen.getByTestId('tool-call-view');
    expect(toolView.getAttribute('data-variant')).toBe('result');
  });

  it('renders error message centered with danger styling', () => {
    render(ChatMessage, {
      props: { message: msg({ type: 'error', content: 'Connection lost' }) },
    });
    const el = screen.getByTestId('chat-message');
    expect(el.getAttribute('data-message-type')).toBe('error');
    expect(el.className).toContain('justify-center');
    expect(el.textContent).toContain('Connection lost');
  });

  it('renders canceled message with reason', () => {
    render(ChatMessage, {
      props: { message: msg({ type: 'canceled', reason: 'User interrupted' }) },
    });
    const el = screen.getByTestId('chat-message');
    expect(el.textContent).toContain('Canceled: User interrupted');
  });

  it('renders canceled message without reason', () => {
    render(ChatMessage, {
      props: { message: msg({ type: 'canceled' }) },
    });
    const el = screen.getByTestId('chat-message');
    expect(el.textContent).toContain('Canceled');
    expect(el.textContent).not.toContain(':');
  });
});
