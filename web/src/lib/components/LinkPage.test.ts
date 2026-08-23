// ABOUTME: Renders LinkPage with fixture link codes and asserts expiry timestamp formatting.
// ABOUTME: Locks in the shared formatTime migration: date+time+seconds, em-dash when empty.
import { render, screen } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import LinkPage from './LinkPage.svelte';

const baseCode = {
  ID: 'lc1',
  Code: 'ABC123',
  Fingerprint: 'SHA256:abcdef1234567890abcd',
  DeviceName: 'Test Device',
  Status: 'pending',
  CreatedAt: '2026-08-23T10:00:00Z',
};

describe('LinkPage', () => {
  it('renders expiry as date + time with seconds', () => {
    render(LinkPage, {
      props: { codes: [{ ...baseCode, ExpiresAt: '2026-08-23T14:30:45Z' }], csrfToken: 't' },
    });
    expect(screen.getByText(/^[A-Z][a-z]{2} \d{2} \d{2}:\d{2}:\d{2}$/)).toBeTruthy();
  });

  it('renders em-dash for empty expiry', () => {
    render(LinkPage, {
      props: { codes: [{ ...baseCode, ID: 'lc2', ExpiresAt: '' }], csrfToken: 't' },
    });
    expect(screen.getAllByText('—').length).toBeGreaterThanOrEqual(1);
  });
});
