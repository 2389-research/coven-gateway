import { render, screen } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import ChatThread from './ChatThread.svelte';
import type { ChatMessage } from '../types/chat';

function msg(overrides: Partial<ChatMessage> & Pick<ChatMessage, 'type'>): ChatMessage {
  return {
    id: `msg-${Math.random()}`,
    content: '',
    timestamp: new Date('2026-02-18T14:30:00'),
    ...overrides,
  };
}

describe('ChatThread', () => {
  it('renders empty state when no messages', () => {
    render(ChatThread, { props: { messages: [], class: 'h-96' } });
    expect(screen.getByTestId('chat-thread').textContent).toContain('No messages yet');
  });

  it('renders messages', () => {
    const messages = [
      msg({ type: 'user', content: 'Hello', id: 'm1' }),
      msg({ type: 'text', content: 'Hi there', id: 'm2' }),
    ];
    render(ChatThread, { props: { messages, class: 'h-96' } });
    const thread = screen.getByTestId('chat-thread');
    expect(thread.querySelectorAll('[data-testid="chat-message"]').length).toBe(2);
  });

  it('filters out non-displayable messages (done, usage)', () => {
    const messages = [
      msg({ type: 'user', content: 'Hello', id: 'm1' }),
      msg({ type: 'done', id: 'm2' }),
      msg({ type: 'usage', id: 'm3' }),
      msg({ type: 'text', content: 'Reply', id: 'm4' }),
    ];
    render(ChatThread, { props: { messages, class: 'h-96' } });
    const thread = screen.getByTestId('chat-thread');
    expect(thread.querySelectorAll('[data-testid="chat-message"]').length).toBe(2);
  });

  it('shows date separator between messages on different days', () => {
    const messages = [
      msg({ type: 'user', content: 'Day 1', id: 'm1', timestamp: new Date('2026-02-17T09:00:00') }),
      msg({ type: 'text', content: 'Day 2', id: 'm2', timestamp: new Date('2026-02-18T10:00:00') }),
    ];
    render(ChatThread, { props: { messages, class: 'h-96' } });
    const separators = screen.getAllByTestId('date-separator');
    expect(separators.length).toBe(2); // One for each day
  });

  it('does not show date separator for same-day messages', () => {
    const messages = [
      msg({ type: 'user', content: 'First', id: 'm1', timestamp: new Date('2026-02-18T09:00:00') }),
      msg({ type: 'text', content: 'Second', id: 'm2', timestamp: new Date('2026-02-18T10:00:00') }),
    ];
    render(ChatThread, { props: { messages, class: 'h-96' } });
    const separators = screen.getAllByTestId('date-separator');
    expect(separators.length).toBe(1); // Only one separator for the first message's day
  });
});
