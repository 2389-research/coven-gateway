// ABOUTME: Verifies Alert owns accessible live-region roles for danger and informational variants.
// ABOUTME: Protects form callers from duplicating or overriding Alert semantics.

import { render, screen } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { describe, expect, it } from 'vitest';
import Alert from './Alert.svelte';

const children = createRawSnippet(() => ({
  render: () => '<span>Message</span>',
}));

describe('Alert', () => {
  it('uses alert semantics for danger messages', () => {
    render(Alert, { props: { variant: 'danger', children } });
    expect(screen.getByRole('alert').textContent).toContain('Message');
  });

  it('uses status semantics for informational messages', () => {
    render(Alert, { props: { variant: 'info', children } });
    expect(screen.getByRole('status').textContent).toContain('Message');
  });
});
