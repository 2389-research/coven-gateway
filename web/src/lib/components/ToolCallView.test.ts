import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import ToolCallView from './ToolCallView.svelte';

describe('ToolCallView', () => {
  it('renders call variant with tool name', () => {
    render(ToolCallView, { props: { variant: 'call', toolName: 'read_file', content: '{}' } });
    const el = screen.getByTestId('tool-call-view');
    expect(el.getAttribute('data-variant')).toBe('call');
    expect(el.textContent).toContain('read_file');
  });

  it('renders result variant with success styling', () => {
    render(ToolCallView, { props: { variant: 'result', content: 'output' } });
    const el = screen.getByTestId('tool-call-view');
    expect(el.getAttribute('data-variant')).toBe('result');
    expect(el.textContent).toContain('Tool Result');
  });

  it('defaults to "Tool Call" when no toolName provided', () => {
    render(ToolCallView, { props: { variant: 'call', content: '{}' } });
    expect(screen.getByTestId('tool-call-view').textContent).toContain('Tool Call');
  });

  it('expands call content on click', async () => {
    render(ToolCallView, {
      props: { variant: 'call', toolName: 'search', content: '{"query":"test"}' },
    });
    const el = screen.getByTestId('tool-call-view');
    // Content hidden initially
    expect(el.querySelector('pre')).toBeNull();
    // Click to expand
    await fireEvent.click(el.querySelector('button')!);
    const pre = el.querySelector('pre');
    expect(pre).toBeTruthy();
    expect(pre!.textContent).toContain('"query"');
  });

  it('starts expanded when expanded prop is true', () => {
    render(ToolCallView, {
      props: { variant: 'result', content: 'some output', expanded: true },
    });
    const pre = screen.getByTestId('tool-call-view').querySelector('pre');
    expect(pre).toBeTruthy();
    expect(pre!.textContent).toContain('some output');
  });

  it('formats JSON content for call variant', async () => {
    render(ToolCallView, {
      props: { variant: 'call', toolName: 'test', content: '{"a":1,"b":2}', expanded: true },
    });
    const pre = screen.getByTestId('tool-call-view').querySelector('pre');
    // Should be pretty-printed
    expect(pre!.textContent).toContain('  "a": 1');
  });
});
