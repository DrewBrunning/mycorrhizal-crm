// Test helpers for the WebAuthn ceremony (issue #594). jsdom has no real
// WebAuthn, so tests stub navigator.credentials + window.PublicKeyCredential.
import { vi } from 'vitest';
import {
  bufferToBase64url,
  type WireCreationOptions,
  type WireRequestOptions,
} from './webauthnCeremony';

export const bytes = (...n: number[]) => new Uint8Array(n).buffer;

export const creationOptionsWire: WireCreationOptions = {
  publicKey: {
    challenge: bufferToBase64url(bytes(1, 2, 3)),
    rp: { id: 'localhost', name: 'Mycorrhizal' },
    user: { id: bufferToBase64url(bytes(9)), name: 'alice', displayName: 'alice' },
    pubKeyCredParams: [{ type: 'public-key', alg: -7 }],
    excludeCredentials: [{ id: bufferToBase64url(bytes(7, 7)), type: 'public-key' }],
  },
};

export const requestOptionsWire: WireRequestOptions = {
  publicKey: {
    challenge: bufferToBase64url(bytes(4, 5, 6)),
    rpId: 'localhost',
    allowCredentials: [{ id: bufferToBase64url(bytes(7, 7)), type: 'public-key' }],
  },
};

export function fakeAttestation() {
  return {
    id: 'cred-id',
    rawId: bytes(7, 7),
    type: 'public-key',
    authenticatorAttachment: 'platform',
    response: {
      attestationObject: bytes(1),
      clientDataJSON: bytes(2),
      getTransports: () => ['internal'],
    },
    getClientExtensionResults: () => ({}),
  };
}

export function fakeAssertion() {
  return {
    id: 'cred-id',
    rawId: bytes(7, 7),
    type: 'public-key',
    authenticatorAttachment: null,
    response: {
      authenticatorData: bytes(3),
      clientDataJSON: bytes(4),
      signature: bytes(5),
      userHandle: bytes(9),
    },
    getClientExtensionResults: () => ({}),
  };
}

// Installs a fake WebAuthn implementation; returns the create/get spies.
export function stubWebAuthn(overrides: { create?: () => unknown; get?: () => unknown } = {}) {
  const create = vi.fn(async () => (overrides.create ? overrides.create() : fakeAttestation()));
  const get = vi.fn(async () => (overrides.get ? overrides.get() : fakeAssertion()));
  vi.stubGlobal('PublicKeyCredential', class {});
  Object.defineProperty(navigator, 'credentials', {
    configurable: true,
    value: { create, get },
  });
  return { create, get };
}

export function unstubWebAuthn() {
  vi.unstubAllGlobals();
  Reflect.deleteProperty(navigator, 'credentials');
}

export const cancelledError = () =>
  new DOMException('The operation was cancelled', 'NotAllowedError');
