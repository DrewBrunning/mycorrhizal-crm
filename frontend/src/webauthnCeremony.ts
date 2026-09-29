// Browser-native WebAuthn plumbing (issue #594). No dependency: the backend
// (go-webauthn) speaks base64url strings, the browser API speaks ArrayBuffers,
// so this file is only the encode/decode boilerplate between them.
//
// Deliberately import-free: both auth.ts (passkey login) and api/webauthn.ts
// (enrollment/management) use it, and auth.ts is also loaded directly by Node
// in the Playwright harness.

export function base64urlToBuffer(value: string): ArrayBuffer {
  const padded = value
    .replace(/-/g, '+')
    .replace(/_/g, '/')
    .padEnd(Math.ceil(value.length / 4) * 4, '=');
  const binary = atob(padded);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  return bytes.buffer;
}

export function bufferToBase64url(buffer: ArrayBuffer | null | undefined): string {
  if (!buffer) return '';
  const bytes = new Uint8Array(buffer);
  let binary = '';
  for (const b of bytes) binary += String.fromCharCode(b);
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

export function isWebAuthnSupported(): boolean {
  return (
    typeof window !== 'undefined' &&
    typeof window.PublicKeyCredential !== 'undefined' &&
    typeof navigator !== 'undefined' &&
    !!navigator.credentials
  );
}

// The go-webauthn options envelope: `{ publicKey: { ...base64url fields } }`.
interface WireDescriptor {
  id: string;
  type: PublicKeyCredentialType;
  transports?: AuthenticatorTransport[];
}

export interface WireCreationOptions {
  publicKey: Omit<
    PublicKeyCredentialCreationOptions,
    'challenge' | 'user' | 'excludeCredentials'
  > & {
    challenge: string;
    user: Omit<PublicKeyCredentialUserEntity, 'id'> & { id: string };
    excludeCredentials?: WireDescriptor[];
  };
}

export interface WireRequestOptions {
  publicKey: Omit<PublicKeyCredentialRequestOptions, 'challenge' | 'allowCredentials'> & {
    challenge: string;
    allowCredentials?: WireDescriptor[];
  };
}

const decodeDescriptor = (d: WireDescriptor): PublicKeyCredentialDescriptor => ({
  ...d,
  id: base64urlToBuffer(d.id),
});

export function decodeCreationOptions(
  wire: WireCreationOptions,
): PublicKeyCredentialCreationOptions {
  const pk = wire.publicKey;
  return {
    ...pk,
    challenge: base64urlToBuffer(pk.challenge),
    user: { ...pk.user, id: base64urlToBuffer(pk.user.id) },
    excludeCredentials: pk.excludeCredentials?.map(decodeDescriptor),
  };
}

export function decodeRequestOptions(wire: WireRequestOptions): PublicKeyCredentialRequestOptions {
  const pk = wire.publicKey;
  return {
    ...pk,
    challenge: base64urlToBuffer(pk.challenge),
    allowCredentials: pk.allowCredentials?.map(decodeDescriptor),
  };
}

// Serialize the browser's credential into the JSON go-webauthn parses.
export function attestationToJSON(cred: PublicKeyCredential): Record<string, unknown> {
  const response = cred.response as AuthenticatorAttestationResponse;
  return {
    id: cred.id,
    rawId: bufferToBase64url(cred.rawId),
    type: cred.type,
    authenticatorAttachment: cred.authenticatorAttachment ?? undefined,
    response: {
      attestationObject: bufferToBase64url(response.attestationObject),
      clientDataJSON: bufferToBase64url(response.clientDataJSON),
      transports: response.getTransports?.() ?? [],
    },
    clientExtensionResults: cred.getClientExtensionResults?.() ?? {},
  };
}

export function assertionToJSON(cred: PublicKeyCredential): Record<string, unknown> {
  const response = cred.response as AuthenticatorAssertionResponse;
  return {
    id: cred.id,
    rawId: bufferToBase64url(cred.rawId),
    type: cred.type,
    authenticatorAttachment: cred.authenticatorAttachment ?? undefined,
    response: {
      authenticatorData: bufferToBase64url(response.authenticatorData),
      clientDataJSON: bufferToBase64url(response.clientDataJSON),
      signature: bufferToBase64url(response.signature),
      userHandle: response.userHandle ? bufferToBase64url(response.userHandle) : undefined,
    },
    clientExtensionResults: cred.getClientExtensionResults?.() ?? {},
  };
}

// True when the browser rejected the ceremony because the user dismissed the
// prompt (or it timed out) — NotAllowedError is the spec's catch-all for both.
export function isCeremonyCancelled(err: unknown): boolean {
  return (
    typeof err === 'object' &&
    err !== null &&
    'name' in err &&
    ((err as { name: string }).name === 'NotAllowedError' ||
      (err as { name: string }).name === 'AbortError')
  );
}

export async function createCredential(
  wire: WireCreationOptions,
): Promise<Record<string, unknown>> {
  const cred = (await navigator.credentials.create({
    publicKey: decodeCreationOptions(wire),
  })) as PublicKeyCredential | null;
  if (!cred) throw new DOMException('No credential returned', 'NotAllowedError');
  return attestationToJSON(cred);
}

export async function getAssertion(wire: WireRequestOptions): Promise<Record<string, unknown>> {
  const cred = (await navigator.credentials.get({
    publicKey: decodeRequestOptions(wire),
  })) as PublicKeyCredential | null;
  if (!cred) throw new DOMException('No credential returned', 'NotAllowedError');
  return assertionToJSON(cred);
}
