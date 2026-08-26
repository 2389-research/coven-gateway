// ABOUTME: Renders LinkPage with fixture link codes and asserts expiry timestamp formatting.
// ABOUTME: Locks in the shared formatTime migration, and QR pair-token minting section.
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, vi, afterEach } from 'vitest';
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

afterEach(() => {
  vi.unstubAllGlobals();
});

const mintOK = {
  ok: true,
  json: async () => ({
    url: 'coven://pair?v=1&host=gw.example.ts.net&token=abc123',
    qr: 'data:image/png;base64,AAAA',
    expires_at: new Date(Date.now() + 5 * 60 * 1000).toISOString(),
  }),
};

describe('LinkPage QR pairing', () => {
  it('renders the mint button', () => {
    render(LinkPage, { props: { codes: [], csrfToken: 't' } });
    expect(screen.getByText('Generate QR code')).toBeTruthy();
  });

  it('shows the QR image and pair link after minting', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mintOK));
    render(LinkPage, { props: { codes: [], csrfToken: 't' } });

    await fireEvent.click(screen.getByText('Generate QR code'));

    await waitFor(() => {
      expect(screen.getByAltText('Pairing QR code')).toBeTruthy();
      expect(screen.getByText('coven://pair?v=1&host=gw.example.ts.net&token=abc123')).toBeTruthy();
      expect(screen.getByText(/Expires in/)).toBeTruthy();
    });
  });

  it('sends the CSRF token header when minting', async () => {
    const fetchMock = vi.fn().mockResolvedValue(mintOK);
    vi.stubGlobal('fetch', fetchMock);
    render(LinkPage, { props: { codes: [], csrfToken: 'csrf-abc' } });

    await fireEvent.click(screen.getByText('Generate QR code'));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/admin/link/pair-token',
        expect.objectContaining({
          method: 'POST',
          headers: { 'X-CSRF-Token': 'csrf-abc' },
        })
      );
    });
  });

  it('shows an error when minting fails', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: false, status: 500, text: async () => 'boom' })
    );
    render(LinkPage, { props: { codes: [], csrfToken: 't' } });

    await fireEvent.click(screen.getByText('Generate QR code'));

    await waitFor(() => {
      expect(screen.getByText('Failed to create pairing code')).toBeTruthy();
    });
  });

  it('re-mints a fresh code from the New code button while one is active', async () => {
    const secondMint = {
      ok: true,
      json: async () => ({
        url: 'coven://pair?v=1&host=gw.example.ts.net&token=second',
        qr: 'data:image/png;base64,BBBB',
        expires_at: new Date(Date.now() + 5 * 60 * 1000).toISOString(),
      }),
    };
    const fetchMock = vi.fn().mockResolvedValueOnce(mintOK).mockResolvedValueOnce(secondMint);
    vi.stubGlobal('fetch', fetchMock);
    render(LinkPage, { props: { codes: [], csrfToken: 't' } });

    await fireEvent.click(screen.getByText('Generate QR code'));
    await waitFor(() => {
      expect(screen.getByAltText('Pairing QR code')).toBeTruthy();
    });

    await fireEvent.click(screen.getByText('New code'));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(2);
      expect(
        screen.getByText('coven://pair?v=1&host=gw.example.ts.net&token=second')
      ).toBeTruthy();
    });
  });

  it('surfaces the 409 remediation message from the server', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 409,
        text: async () => 'gateway base URL is an IP literal; set webadmin.base_url',
      })
    );
    render(LinkPage, { props: { codes: [], csrfToken: 't' } });

    await fireEvent.click(screen.getByText('Generate QR code'));

    await waitFor(() => {
      expect(
        screen.getByText('gateway base URL is an IP literal; set webadmin.base_url')
      ).toBeTruthy();
    });
  });
});
