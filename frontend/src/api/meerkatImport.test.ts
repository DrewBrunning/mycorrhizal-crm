import { afterEach, expect, test, vi } from 'vitest';
import {
  cancelMeerkatImport,
  confirmMeerkatImport,
  getMeerkatImportStatus,
  startMeerkatFetch,
  uploadMeerkatDatabase,
} from './meerkatImport';

afterEach(() => {
  vi.unstubAllGlobals();
});

function okResponse(body?: unknown) {
  return { ok: true, json: async () => body };
}

test('uploadMeerkatDatabase POSTs multipart form-data and returns the picker payload', async () => {
  const fetchMock = vi.fn().mockResolvedValueOnce(
    okResponse({
      session_id: 's1',
      source_users: [{ id: 1, username: 'a', email: '', name: '', contacts: 3 }],
      default_source_user_id: 1,
      totals: { contacts: 3, relationships: 1, notes: 2, activities: 0, reminders: 0 },
    }),
  );
  vi.stubGlobal('fetch', fetchMock);

  const file = new File([new Uint8Array([1, 2, 3])], 'meerkat.db');
  const resp = await uploadMeerkatDatabase(file);

  const [url, init] = fetchMock.mock.calls[0];
  expect(url).toContain('/contacts/import/meerkat/upload');
  expect(init.method).toBe('POST');
  expect(init.body).toBeInstanceOf(FormData);
  expect((init.body as FormData).get('file')).toBeInstanceOf(File);
  expect(resp.default_source_user_id).toBe(1);
});

test('uploadMeerkatDatabase surfaces a rejection', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValueOnce({
      ok: false,
      status: 400,
      json: async () => ({
        error: { code: 'INVALID_INPUT', message: 'no', details: { field: 'file' } },
      }),
    }),
  );
  await expect(uploadMeerkatDatabase(new File(['x'], 'x.db'))).rejects.toMatchObject({
    code: 'INVALID_INPUT',
  });
});

test('startMeerkatFetch sends session id and the chosen source user', async () => {
  const fetchMock = vi.fn().mockResolvedValue(okResponse({}));
  vi.stubGlobal('fetch', fetchMock);

  await startMeerkatFetch('s1', 2);
  expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({
    session_id: 's1',
    source_user_id: 2,
  });

  await startMeerkatFetch('s1');
  expect(JSON.parse(fetchMock.mock.calls[1][1].body)).toEqual({
    session_id: 's1',
    source_user_id: null,
  });
});

test('the shared status/confirm/cancel calls are bound to the meerkat base path', async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValueOnce(
      okResponse({ session_id: 's1', phase: 'ready', phase_done: 0, phase_total: 0 }),
    )
    .mockResolvedValueOnce({ ok: true, status: 202, json: async () => ({}) })
    .mockRejectedValueOnce(new Error('net'));
  vi.stubGlobal('fetch', fetchMock);

  await getMeerkatImportStatus('s1');
  const res = await confirmMeerkatImport('s1', [{ row_index: 0, action: 'add' }]);
  expect(res).toBeUndefined();
  await expect(cancelMeerkatImport('s1')).resolves.toBeUndefined();

  expect(fetchMock.mock.calls[0][0]).toContain('/contacts/import/meerkat/status?session_id=s1');
  expect(fetchMock.mock.calls[1][0]).toContain('/contacts/import/meerkat/confirm');
  expect(fetchMock.mock.calls[2][0]).toContain('/contacts/import/meerkat/cancel?session_id=s1');
});
