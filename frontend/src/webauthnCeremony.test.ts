import { afterEach, describe, expect, test } from 'vitest';
import {
  assertionToJSON,
  attestationToJSON,
  base64urlToBuffer,
  bufferToBase64url,
  createCredential,
  decodeCreationOptions,
  decodeRequestOptions,
  getAssertion,
  isCeremonyCancelled,
  isWebAuthnSupported,
} from './webauthnCeremony';
import {
  bytes,
  cancelledError,
  creationOptionsWire,
  fakeAssertion,
  fakeAttestation,
  requestOptionsWire,
  stubWebAuthn,
  unstubWebAuthn,
} from './webauthnTestUtils';

afterEach(unstubWebAuthn);

describe('base64url', () => {
  test('round-trips arbitrary bytes, including ones that need +/ substitution', () => {
    const raw = new Uint8Array([251, 255, 254, 0, 1, 62, 63]).buffer;
    const encoded = bufferToBase64url(raw);
    expect(encoded).not.toMatch(/[+/=]/);
    expect(new Uint8Array(base64urlToBuffer(encoded))).toEqual(new Uint8Array(raw));
  });

  test('encodes to the unpadded url-safe alphabet', () => {
    expect(bufferToBase64url(bytes(0xfb, 0xff))).toBe('-_8');
  });

  test('decodes unpadded input of every length remainder', () => {
    for (const len of [1, 2, 3, 4, 5]) {
      const raw = new Uint8Array(len).map((_, i) => i + 200).buffer;
      expect(new Uint8Array(base64urlToBuffer(bufferToBase64url(raw)))).toEqual(
        new Uint8Array(raw),
      );
    }
  });

  test('encodes a missing buffer as empty', () => {
    expect(bufferToBase64url(null)).toBe('');
    expect(bufferToBase64url(undefined)).toBe('');
  });
});

describe('isWebAuthnSupported', () => {
  test('is false without PublicKeyCredential/navigator.credentials', () => {
    expect(isWebAuthnSupported()).toBe(false);
  });
  test('is true once the browser API exists', () => {
    stubWebAuthn();
    expect(isWebAuthnSupported()).toBe(true);
  });
});

describe('option decoding', () => {
  test('creation options: challenge, user id and excluded ids become buffers', () => {
    const opts = decodeCreationOptions(creationOptionsWire);
    expect(new Uint8Array(opts.challenge as ArrayBuffer)).toEqual(new Uint8Array([1, 2, 3]));
    expect(new Uint8Array(opts.user.id as ArrayBuffer)).toEqual(new Uint8Array([9]));
    expect(new Uint8Array(opts.excludeCredentials?.[0].id as ArrayBuffer)).toEqual(
      new Uint8Array([7, 7]),
    );
    expect(opts.rp.id).toBe('localhost');
  });

  test('creation options tolerate an absent exclude list', () => {
    const { excludeCredentials: _omit, ...rest } = creationOptionsWire.publicKey;
    expect(decodeCreationOptions({ publicKey: rest }).excludeCredentials).toBeUndefined();
  });

  test('request options: challenge and allowed ids become buffers', () => {
    const opts = decodeRequestOptions(requestOptionsWire);
    expect(new Uint8Array(opts.challenge as ArrayBuffer)).toEqual(new Uint8Array([4, 5, 6]));
    expect(new Uint8Array(opts.allowCredentials?.[0].id as ArrayBuffer)).toEqual(
      new Uint8Array([7, 7]),
    );
  });

  test('request options tolerate an absent allow list', () => {
    const { allowCredentials: _omit, ...rest } = requestOptionsWire.publicKey;
    expect(decodeRequestOptions({ publicKey: rest }).allowCredentials).toBeUndefined();
  });
});

describe('credential serialization', () => {
  test('attestation → the JSON go-webauthn parses', () => {
    const json = attestationToJSON(fakeAttestation() as unknown as PublicKeyCredential);
    expect(json).toMatchObject({
      id: 'cred-id',
      rawId: bufferToBase64url(bytes(7, 7)),
      type: 'public-key',
      response: {
        attestationObject: bufferToBase64url(bytes(1)),
        clientDataJSON: bufferToBase64url(bytes(2)),
        transports: ['internal'],
      },
    });
  });

  test('attestation without getTransports/extension results falls back to empties', () => {
    const cred = fakeAttestation() as Record<string, unknown>;
    (cred.response as Record<string, unknown>).getTransports = undefined;
    cred.getClientExtensionResults = undefined;
    const json = attestationToJSON(cred as unknown as PublicKeyCredential) as {
      response: { transports: string[] };
      clientExtensionResults: unknown;
    };
    expect(json.response.transports).toEqual([]);
    expect(json.clientExtensionResults).toEqual({});
  });

  test('assertion → the JSON go-webauthn parses, userHandle included', () => {
    const json = assertionToJSON(fakeAssertion() as unknown as PublicKeyCredential);
    expect(json).toMatchObject({
      rawId: bufferToBase64url(bytes(7, 7)),
      response: {
        authenticatorData: bufferToBase64url(bytes(3)),
        clientDataJSON: bufferToBase64url(bytes(4)),
        signature: bufferToBase64url(bytes(5)),
        userHandle: bufferToBase64url(bytes(9)),
      },
    });
  });

  test('assertion omits a null userHandle and tolerates missing extension results', () => {
    const cred = fakeAssertion() as Record<string, unknown>;
    (cred.response as Record<string, unknown>).userHandle = null;
    cred.getClientExtensionResults = undefined;
    const json = assertionToJSON(cred as unknown as PublicKeyCredential) as {
      response: { userHandle?: string };
      clientExtensionResults: unknown;
    };
    expect(json.response.userHandle).toBeUndefined();
    expect(json.clientExtensionResults).toEqual({});
  });
});

describe('isCeremonyCancelled', () => {
  test('recognises a dismissed or aborted prompt', () => {
    expect(isCeremonyCancelled(cancelledError())).toBe(true);
    expect(isCeremonyCancelled(new DOMException('x', 'AbortError'))).toBe(true);
  });
  test('rejects everything else', () => {
    expect(isCeremonyCancelled(new Error('boom'))).toBe(false);
    expect(isCeremonyCancelled(null)).toBe(false);
    expect(isCeremonyCancelled('NotAllowedError')).toBe(false);
  });
});

describe('createCredential / getAssertion', () => {
  test('create passes decoded options to the browser and serializes the result', async () => {
    const { create } = stubWebAuthn();
    const json = await createCredential(creationOptionsWire);
    const arg = (create.mock.calls[0] as unknown[])[0] as {
      publicKey: PublicKeyCredentialCreationOptions;
    };
    expect(arg.publicKey.challenge).toBeInstanceOf(ArrayBuffer);
    expect(json.id).toBe('cred-id');
  });

  test('a null credential from the browser is treated as a cancelled ceremony', async () => {
    stubWebAuthn({ create: () => null, get: () => null });
    await expect(createCredential(creationOptionsWire)).rejects.toMatchObject({
      name: 'NotAllowedError',
    });
    await expect(getAssertion(requestOptionsWire)).rejects.toMatchObject({
      name: 'NotAllowedError',
    });
  });

  test('get passes decoded options and serializes the assertion', async () => {
    const { get } = stubWebAuthn();
    const json = await getAssertion(requestOptionsWire);
    const arg = (get.mock.calls[0] as unknown[])[0] as {
      publicKey: PublicKeyCredentialRequestOptions;
    };
    expect(arg.publicKey.challenge).toBeInstanceOf(ArrayBuffer);
    expect(json.id).toBe('cred-id');
  });
});
