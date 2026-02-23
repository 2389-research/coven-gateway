import { render, screen } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import ThinkingIndicator from './ThinkingIndicator.svelte';

describe('ThinkingIndicator', () => {
  it('renders three animated dots', () => {
    render(ThinkingIndicator);
    const el = screen.getByTestId('thinking-indicator');
    const dots = el.querySelectorAll('.thinking-dot');
    expect(dots.length).toBe(3);
  });

  it('renders without text by default', () => {
    render(ThinkingIndicator);
    const el = screen.getByTestId('thinking-indicator');
    expect(el.querySelectorAll('p').length).toBe(0);
  });

  it('renders text when provided', () => {
    render(ThinkingIndicator, { props: { text: 'Analyzing code...' } });
    const el = screen.getByTestId('thinking-indicator');
    expect(el.textContent).toContain('Analyzing code...');
  });

  it('has staggered animation delays on dots', () => {
    render(ThinkingIndicator);
    const dots = screen.getByTestId('thinking-indicator').querySelectorAll('.thinking-dot');
    // First dot has no inline delay (uses CSS default)
    expect(dots[0].getAttribute('style')).toBeNull();
    expect(dots[1].getAttribute('style')).toContain('150ms');
    expect(dots[2].getAttribute('style')).toContain('300ms');
  });
});
