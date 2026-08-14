import { describe, test, expect, vi, afterEach } from 'vitest';
import { updateSelfContact } from './users';

afterEach(() => {
  vi.unstubAllGlobals();
});

// T90: PATCH /users/me/self-contact wrapper. handleResponse (errorHandling.ts)
// reads response.text(), so the mocks provide both text and json.
function responseBody(body: unknown) {
  const text = JSON.stringify(body);
  return { ok: true, json: async () => body, text: async () => text };
}

describe('updateSelfContact', () => {
  test('PATCHes the chosen uid to /users/me/self-contact', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(
      responseBody({ message: 'Self contact updated', self_contact_vcard_uid: 'uid-1' })
    );
    vi.stubGlobal('fetch', fetchMock);

    await updateSelfContact('uid-1');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/users/me/self-contact');
    expect(init.method).toBe('PATCH');
    expect(JSON.parse(init.body)).toEqual({ vcard_uid: 'uid-1' });
  });

  test('sends a null vcard_uid to clear the link', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(responseBody({ message: 'Self contact cleared' }));
    vi.stubGlobal('fetch', fetchMock);

    await updateSelfContact(null);

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/users/me/self-contact');
    expect(JSON.parse(init.body)).toEqual({ vcard_uid: null });
  });

  test('throws the backend reason on a failed request', async () => {
    const body = { error: { code: 'NOT_FOUND', message: 'Contact not found', details: { reason: 'not yours' } } };
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: false,
      status: 404,
      json: async () => body,
      text: async () => JSON.stringify(body),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(updateSelfContact('uid-x')).rejects.toThrow('not yours');
  });
});
