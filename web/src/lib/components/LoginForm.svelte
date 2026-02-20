<!--
  ABOUTME: Full-page login island — password form (progressive enhancement) + passkey (WebAuthn).
  ABOUTME: Mounted via data-island="login-form" in login.html Go template.
-->
<script lang="ts">
  import Card from './Card.svelte';
  import TextField from './TextField.svelte';
  import Button from './Button.svelte';
  import Alert from './Alert.svelte';
  import Stack from './Stack.svelte';

  interface Props {
    csrfToken: string;
    error?: string;
  }

  let { csrfToken, error }: Props = $props();

  // --- Passkey state ---
  let passkeySupported = $state(false);
  let passkeyLoading = $state(false);
  let passkeyError = $state('');

  $effect(() => {
    passkeySupported = typeof window !== 'undefined' && !!window.PublicKeyCredential;
  });

  let displayError = $derived(error || passkeyError);

  // --- Base64url helpers ---
  function base64URLDecode(str: string): Uint8Array {
    const base64 = str.replace(/-/g, '+').replace(/_/g, '/');
    const padLen = (4 - (base64.length % 4)) % 4;
    const padded = base64 + '='.repeat(padLen);
    const binary = atob(padded);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) {
      bytes[i] = binary.charCodeAt(i);
    }
    return bytes;
  }

  function base64URLEncode(buffer: ArrayBuffer): string {
    const bytes = new Uint8Array(buffer);
    let binary = '';
    for (let i = 0; i < bytes.length; i++) {
      binary += String.fromCharCode(bytes[i]);
    }
    return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '');
  }

  // --- Passkey login ---
  async function handlePasskeyLogin() {
    passkeyError = '';
    passkeyLoading = true;

    try {
      const beginResp = await fetch('/webauthn/login/begin', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
      });

      if (!beginResp.ok) {
        throw new Error('Failed to start authentication');
      }

      const beginData = await beginResp.json();
      const options = beginData.options;

      options.publicKey.challenge = base64URLDecode(options.publicKey.challenge);

      if (options.publicKey.allowCredentials) {
        options.publicKey.allowCredentials = options.publicKey.allowCredentials.map(
          (cred: { id: string; type: string }) => ({
            ...cred,
            id: base64URLDecode(cred.id),
          }),
        );
      }

      const credential = (await navigator.credentials.get(options)) as PublicKeyCredential;
      const response = credential.response as AuthenticatorAssertionResponse;

      const finishResp = await fetch('/webauthn/login/finish', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          sessionToken: beginData.sessionToken,
          response: {
            id: credential.id,
            rawId: base64URLEncode(credential.rawId),
            type: credential.type,
            response: {
              authenticatorData: base64URLEncode(response.authenticatorData),
              clientDataJSON: base64URLEncode(response.clientDataJSON),
              signature: base64URLEncode(response.signature),
              userHandle: response.userHandle ? base64URLEncode(response.userHandle) : null,
            },
          },
        }),
      });

      if (!finishResp.ok) {
        const errText = await finishResp.text();
        throw new Error(errText || 'Authentication failed');
      }

      const finishData = await finishResp.json();
      window.location.href = finishData.redirect || '/';
    } catch (err: unknown) {
      if (err instanceof Error && err.name === 'NotAllowedError') {
        passkeyError = 'Authentication cancelled or timed out';
      } else {
        passkeyError = err instanceof Error ? err.message : 'Authentication failed';
      }
    } finally {
      passkeyLoading = false;
    }
  }
</script>

<div class="min-h-screen flex items-center justify-center p-4" data-testid="login-form">
  <div class="w-full max-w-md">
    <!-- Logo/Brand -->
    <div class="text-center mb-8">
      <div class="inline-flex items-center gap-3 mb-4">
        <div
          class="w-10 h-10 border border-border-bright bg-surface-raised flex items-center justify-center"
        >
          <svg
            class="w-5 h-5 text-accent-primary"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
            aria-hidden="true"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="1.5"
              d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z"
            />
          </svg>
        </div>
        <span class="font-mono text-2xl font-bold tracking-tight">COVEN</span>
      </div>
      <p class="text-text-secondary font-mono text-xs tracking-widest uppercase">
        Control Plane Access
      </p>
    </div>

    <!-- Login Card -->
    <Card>
      {#snippet header()}
        <div class="flex items-center gap-2">
          <div class="w-2 h-2 rounded-full bg-accent-primary animate-pulse"></div>
          <span class="font-mono text-xs text-text-secondary uppercase tracking-wider"
            >Authenticate</span
          >
        </div>
      {/snippet}

      <Stack gap="4">
        {#if displayError}
          <Alert variant="danger" role="alert">
            {#snippet children()}
              {displayError}
            {/snippet}
          </Alert>
        {/if}

        <!-- Password form — works without JS via native form POST -->
        <form method="POST" action="/login" data-testid="login-password-form">
          <input type="hidden" name="csrf_token" value={csrfToken} />
          <Stack gap="4">
            <TextField label="Username" name="username" required autocomplete="username" />
            <TextField
              label="Password"
              name="password"
              type="password"
              required
              autocomplete="current-password"
            />
            <Button variant="primary" type="submit">
              {#snippet children()}Authenticate{/snippet}
            </Button>
          </Stack>
        </form>

        <!-- Divider -->
        <div class="flex items-center gap-4">
          <div class="flex-1 h-px bg-border"></div>
          <span class="text-text-secondary font-mono text-xs">OR</span>
          <div class="flex-1 h-px bg-border"></div>
        </div>

        <!-- Passkey button -->
        <Button
          variant="secondary"
          disabled={!passkeySupported || passkeyLoading}
          loading={passkeyLoading}
          onclick={handlePasskeyLogin}
          data-testid="passkey-button"
        >
          {#snippet children()}
            {#if !passkeySupported}
              Passkeys not supported
            {:else}
              <svg
                class="w-4 h-4"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
                aria-hidden="true"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="1.5"
                  d="M12 11c0 3.517-1.009 6.799-2.753 9.571m-3.44-2.04l.054-.09A13.916 13.916 0 008 11a4 4 0 118 0c0 1.017-.07 2.019-.203 3m-2.118 6.844A21.88 21.88 0 0015.171 17m3.839 1.132c.645-2.266.99-4.659.99-7.132A8 8 0 008 4.07M3 15.364c.64-1.319 1-2.8 1-4.364 0-1.457.39-2.823 1.07-4"
                />
              </svg>
              Passkey
            {/if}
          {/snippet}
        </Button>
      </Stack>
    </Card>

    <!-- Footer -->
    <p class="text-center text-text-secondary font-mono text-xs mt-6">
      coven-gateway <span class="opacity-50">|</span> agent control plane
    </p>
  </div>
</div>
