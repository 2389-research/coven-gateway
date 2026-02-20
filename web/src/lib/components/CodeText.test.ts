import { render, screen } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import { createRawSnippet } from 'svelte';
import CodeText from './CodeText.svelte';

describe('CodeText', () => {
  it('renders as code element', () => {
    const children = createRawSnippet(() => ({
      render: () => '<span>agent-123</span>',
    }));
    render(CodeText, { props: { children } });
    const el = screen.getByTestId('code-text');
    expect(el.tagName).toBe('CODE');
  });

  it('has monospace font', () => {
    const children = createRawSnippet(() => ({
      render: () => '<span>value</span>',
    }));
    render(CodeText, { props: { children } });
    expect(screen.getByTestId('code-text').className).toContain('font-mono');
  });

  it('has select-all for easy copying', () => {
    const children = createRawSnippet(() => ({
      render: () => '<span>550e8400</span>',
    }));
    render(CodeText, { props: { children } });
    expect(screen.getByTestId('code-text').className).toContain('select-all');
  });

  it('applies custom class', () => {
    const children = createRawSnippet(() => ({
      render: () => '<span>test</span>',
    }));
    render(CodeText, { props: { children, class: 'text-accent' } });
    expect(screen.getByTestId('code-text').className).toContain('text-accent');
  });
});
