import type { Meta, StoryObj } from '@storybook/svelte';
import LoginForm from './LoginForm.svelte';

/**
 * Mock fetch for WebAuthn endpoints.
 * Returns a cleanup function that restores the original fetch.
 */
function mockWebAuthnFetch() {
  return () => {
    const originalFetch = globalThis.fetch;
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      const url =
        typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url;

      if (url === '/webauthn/login/begin') {
        return new Response(
          JSON.stringify({
            sessionToken: 'mock-session',
            options: {
              publicKey: {
                challenge: 'dGVzdC1jaGFsbGVuZ2U', // base64url "test-challenge"
                rpId: 'localhost',
                timeout: 60000,
                allowCredentials: [],
              },
            },
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        );
      }

      if (url === '/webauthn/login/finish') {
        return new Response(JSON.stringify({ redirect: '/' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }

      return originalFetch(input, init);
    }) as typeof fetch;
    return () => {
      globalThis.fetch = originalFetch;
    };
  };
}

const meta: Meta<LoginForm> = {
  title: 'Pages/LoginForm',
  component: LoginForm,
  parameters: { layout: 'fullscreen' },
  beforeEach: mockWebAuthnFetch(),
  tags: ['autodocs'],
};

export default meta;
type Story = StoryObj<LoginForm>;

export const Default: Story = {
  args: {
    csrfToken: 'abc123def456',
  },
};

export const WithError: Story = {
  args: {
    csrfToken: 'abc123def456',
    error: 'Invalid username or password',
  },
};

export const PasskeyUnsupported: Story = {
  args: {
    csrfToken: 'abc123def456',
  },
  beforeEach: () => {
    const original = window.PublicKeyCredential;
    // @ts-expect-error -- intentionally removing for story
    delete window.PublicKeyCredential;
    return () => {
      if (original) {
        window.PublicKeyCredential = original;
      }
    };
  },
};
