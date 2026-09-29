import { afterEach, describe, expect, test, vi } from 'vitest';
import {
  cancelledError,
  creationOptionsWire,
  requestOptionsWire,
  stubWebAuthn,
  unstubWebAuthn,
} from '../webauthnTestUtils';
import { listPasskeys, proveWithOtherPasskey, registerPasskey, removePasskey } from './webauthn';

afterEach(unstubWebAuthn);

function ok(body: unknown) {
  return { ok: true, status: 200, json: async () => body, text: async () => JSON.stringify(body) };
}
function fail(status: number, message: string) {
  const body = { error: { message } };
  return { ok: false, status, json: async () => body, text: async () => JSON.stringify(body) };
}

describe('listPasskeys', () => {
  test('GETs the credentials and unwraps the list', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        ok({ credentials: [{ id: 'a', name: 'Laptop', created_at: '2026-01-01T00:00:00Z' }] }),
      );
    vi.stubGlobal('fetch', fetchMock);
    const list = await listPasskeys();
    expect(list).toHaveLength(1);
    expect(fetchMock.mock.calls[0][0]).toContain('/webauthn/credentials');
  });

  test('tolerates a response without the key', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(ok({})));
    expect(await listPasskeys()).toEqual([]);
  });
});

describe('registerPasskey', () => {
  test('begin → create() → finish, sending the trimmed name and serialized credential', async () => {
    const { create } = stubWebAuthn();
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(ok(creationOptionsWire))
      .mockResolvedValueOnce(
        ok({ id: 'p1', name: 'YubiKey', created_at: 'x', recovery_codes: ['A', 'B'] }),
      );
    vi.stubGlobal('fetch', fetchMock);

    const result = await registerPasskey('  YubiKey ');

    expect(fetchMock.mock.calls[0][0]).toContain('/webauthn/register/begin');
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ name: 'YubiKey' });
    expect(create).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[1][0]).toContain('/webauthn/register/finish');
    expect(JSON.parse(fetchMock.mock.calls[1][1].body)).toMatchObject({
      id: 'cred-id',
      type: 'public-key',
    });
    expect(result.recovery_codes).toEqual(['A', 'B']);
  });

  test('sends an empty body when no name is given and defaults recovery codes to []', async () => {
    stubWebAuthn();
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(ok(creationOptionsWire))
      .mockResolvedValueOnce(ok({ id: 'p1', name: 'x', created_at: 'x' }));
    vi.stubGlobal('fetch', fetchMock);
    const result = await registerPasskey();
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({});
    expect(result.recovery_codes).toEqual([]);
  });

  test('a dismissed prompt rejects with the browser error and never calls finish', async () => {
    stubWebAuthn({
      create: () => {
        throw cancelledError();
      },
    });
    const fetchMock = vi.fn().mockResolvedValueOnce(ok(creationOptionsWire));
    vi.stubGlobal('fetch', fetchMock);
    await expect(registerPasskey()).rejects.toMatchObject({ name: 'NotAllowedError' });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  test('surfaces a backend rejection of begin (e.g. OIDC account)', async () => {
    stubWebAuthn();
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(fail(403, 'OIDC accounts cannot use this')));
    await expect(registerPasskey()).rejects.toThrow(/OIDC accounts/);
  });
});

describe('proveWithOtherPasskey / removePasskey', () => {
  test('proof begins at /webauthn/assert/begin and returns the serialized assertion', async () => {
    stubWebAuthn();
    const fetchMock = vi.fn().mockResolvedValue(ok(requestOptionsWire));
    vi.stubGlobal('fetch', fetchMock);
    const proof = await proveWithOtherPasskey();
    expect(fetchMock.mock.calls[0][0]).toContain('/webauthn/assert/begin');
    expect(proof.assertion.id).toBe('cred-id');
  });

  test('DELETEs the encoded id with the proof body', async () => {
    const fetchMock = vi.fn().mockResolvedValue(ok({ message: 'Passkey removed' }));
    vi.stubGlobal('fetch', fetchMock);
    await removePasskey('a/b', { code: '123456' });
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/webauthn/credentials/a%2Fb');
    expect(init.method).toBe('DELETE');
    expect(JSON.parse(init.body)).toEqual({ code: '123456' });
  });

  test('a rejected proof throws the backend message', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(fail(400, 'Invalid code. Please try again.')));
    await expect(removePasskey('a', { code: '0' })).rejects.toThrow('Invalid code');
  });
});
