import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi } from 'vitest';
import ChatInput from './ChatInput.svelte';

describe('ChatInput', () => {
  it('renders textarea and send button', () => {
    render(ChatInput, { props: { onSend: vi.fn() } });
    expect(screen.getByTestId('chat-input-textarea')).toBeTruthy();
    expect(screen.getByTestId('chat-input-send')).toBeTruthy();
  });

  it('send button is disabled when textarea is empty', () => {
    render(ChatInput, { props: { onSend: vi.fn() } });
    const btn = screen.getByTestId('chat-input-send') as HTMLButtonElement;
    expect(btn.disabled).toBe(true);
  });

  it('send button enables when text is entered', async () => {
    render(ChatInput, { props: { onSend: vi.fn() } });
    const textarea = screen.getByTestId('chat-input-textarea') as HTMLTextAreaElement;
    await fireEvent.input(textarea, { target: { value: 'Hello' } });
    const btn = screen.getByTestId('chat-input-send') as HTMLButtonElement;
    expect(btn.disabled).toBe(false);
  });

  it('calls onSend and clears textarea on send button click', async () => {
    const onSend = vi.fn();
    render(ChatInput, { props: { onSend } });
    const textarea = screen.getByTestId('chat-input-textarea') as HTMLTextAreaElement;
    await fireEvent.input(textarea, { target: { value: 'Hello' } });
    const btn = screen.getByTestId('chat-input-send');
    await fireEvent.click(btn);
    expect(onSend).toHaveBeenCalledWith('Hello');
    expect(textarea.value).toBe('');
  });

  it('shows disabled state with "Sending..." text', () => {
    render(ChatInput, { props: { onSend: vi.fn(), disabled: true } });
    const textarea = screen.getByTestId('chat-input-textarea') as HTMLTextAreaElement;
    expect(textarea.disabled).toBe(true);
    expect(screen.getByTestId('chat-input').textContent).toContain('Sending...');
  });

  it('shows character count', async () => {
    render(ChatInput, { props: { onSend: vi.fn(), maxLength: 100 } });
    const textarea = screen.getByTestId('chat-input-textarea') as HTMLTextAreaElement;
    await fireEvent.input(textarea, { target: { value: 'Hello' } });
    const counter = screen.getByTestId('chat-input-charcount');
    expect(counter.textContent).toContain('5');
    expect(counter.textContent).toContain('100');
  });
});
