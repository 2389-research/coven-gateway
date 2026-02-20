/**
 * WebAuthn/Passkey registration utilities.
 * Handles base64url encoding, credential creation, and server communication.
 */

export function base64URLDecode(str: string): Uint8Array {
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

export function base64URLEncode(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer);
  let binary = '';
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i]);
  }
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '');
}

export function isWebAuthnSupported(): boolean {
  return typeof window !== 'undefined' && !!window.PublicKeyCredential;
}

interface RegisterResult {
  ok: boolean;
  error?: string;
}

/**
 * Runs the full WebAuthn passkey registration flow:
 * 1. Calls /admin/webauthn/register/begin to get challenge
 * 2. Creates credential via navigator.credentials.create()
 * 3. Calls /admin/webauthn/register/finish to store credential
 */
export async function registerPasskey(csrfToken: string): Promise<RegisterResult> {
  // Step 1: Begin registration
  const beginResp = await fetch('/admin/webauthn/register/begin', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-CSRF-Token': csrfToken,
    },
  });

  if (!beginResp.ok) {
    throw new Error('Failed to start registration');
  }

  const beginData = await beginResp.json();
  const options = beginData.options;

  // Step 2: Convert base64url fields to ArrayBuffer for the browser API
  options.publicKey.challenge = base64URLDecode(options.publicKey.challenge);
  options.publicKey.user.id = base64URLDecode(options.publicKey.user.id);

  if (options.publicKey.excludeCredentials) {
    options.publicKey.excludeCredentials = options.publicKey.excludeCredentials.map(
      (cred: { id: string; type: string }) => ({
        ...cred,
        id: base64URLDecode(cred.id),
      }),
    );
  }

  // Step 3: Create credential via browser WebAuthn API
  const credential = (await navigator.credentials.create(options)) as PublicKeyCredential;
  if (!credential) {
    throw new Error('Credential creation returned null');
  }

  const attestationResponse = credential.response as AuthenticatorAttestationResponse;

  // Step 4: Encode response for server
  const response: Record<string, unknown> = {
    id: credential.id,
    rawId: base64URLEncode(credential.rawId),
    type: credential.type,
    response: {
      attestationObject: base64URLEncode(attestationResponse.attestationObject),
      clientDataJSON: base64URLEncode(attestationResponse.clientDataJSON),
    },
  };

  // Add transports if available
  if (attestationResponse.getTransports) {
    (response.response as Record<string, unknown>).transports =
      attestationResponse.getTransports();
  }

  // Step 5: Finish registration
  const finishResp = await fetch('/admin/webauthn/register/finish', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-CSRF-Token': csrfToken,
    },
    body: JSON.stringify({
      sessionToken: beginData.sessionToken,
      response,
    }),
  });

  if (!finishResp.ok) {
    const errText = await finishResp.text();
    throw new Error(errText || 'Registration failed');
  }

  return { ok: true };
}
