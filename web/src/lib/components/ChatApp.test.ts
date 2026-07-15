// ABOUTME: Verifies ChatApp renders connection state from the real chat SSE store.
// ABOUTME: Drives the browser EventSource boundary while asserting rendered user behavior.

import { render, screen, waitFor } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import { MockEventSource } from '../../../test/setup';
import ChatApp from './ChatApp.svelte';

describe('ChatApp', () => {
  it('shows the agent online when its SSE stream opens', async () => {
    render(ChatApp, {
      props: {
        agentId: 'agent-1',
        agentName: 'Agent One',
        csrfToken: 'csrf-token',
      },
    });

    expect(screen.getByTestId('status-dot').getAttribute('aria-label')).toBe('Offline');

    const source = MockEventSource._last();
    expect(source).toBeDefined();
    source!.simulateOpen();

    await waitFor(() => {
      expect(screen.getByTestId('status-dot').getAttribute('aria-label')).toBe('Online');
    });
  });
});
