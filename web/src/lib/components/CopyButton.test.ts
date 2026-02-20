import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import CopyButton from './CopyButton.svelte';

describe('CopyButton', () => {
  beforeEach(() => {
    Object.assign(navigator, {
      clipboard: { writeText: vi.fn().mockResolvedValue(undefined) },
    });
  });

  it('renders with default label', () => {
    render(CopyButton, { props: { value: 'test-value' } });
    expect(screen.getByText('Copy')).toBeTruthy();
  });

  it('renders with custom label', () => {
    render(CopyButton, { props: { value: 'test', label: 'Copy link' } });
    expect(screen.getByText('Copy link')).toBeTruthy();
  });

  it('copies value to clipboard on click', async () => {
    render(CopyButton, { props: { value: 'secret-key-123' } });
    await fireEvent.click(screen.getByTestId('copy-button'));
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('secret-key-123');
  });

  it('shows copied feedback after click', async () => {
    render(CopyButton, { props: { value: 'test' } });
    await fireEvent.click(screen.getByTestId('copy-button'));
    expect(screen.getByText('Copied!')).toBeTruthy();
  });

  it('shows custom copied label', async () => {
    render(CopyButton, { props: { value: 'test', copiedLabel: 'Done!' } });
    await fireEvent.click(screen.getByTestId('copy-button'));
    expect(screen.getByText('Done!')).toBeTruthy();
  });

  it('is disabled when disabled prop is set', () => {
    render(CopyButton, { props: { value: 'test', disabled: true } });
    expect(screen.getByTestId('copy-button').hasAttribute('disabled')).toBe(true);
  });

  it('sets aria-label based on state', async () => {
    render(CopyButton, { props: { value: 'test' } });
    const btn = screen.getByTestId('copy-button');
    expect(btn.getAttribute('aria-label')).toBe('Copy');
    await fireEvent.click(btn);
    expect(btn.getAttribute('aria-label')).toBe('Copied!');
  });
});
