import { afterEach, expect, test, vi } from 'vitest';
import {
  cancelMonicaImport,
  confirmMonicaImport,
  connectMonica,
  getMonicaImportPreview,
  getMonicaImportStatus,
  startMonicaFetch,
} from './monicaImport';

afterEach(() => {
  vi.unstubAllGlobals();
});

function okResponse(body?: unknown) {
  return { ok: true, json: async () => body };
}

function errorResponse() {
  return {
    ok: false,
    status: 400,
    statusText: 'Bad Request',
    json: async () => ({
      error: { code: 'INVALID_INPUT', message: 'nope', details: { field: 'api_token' } },
    }),
  };
}

test('connectMonica POSTs base_url + api_token and returns the session', async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValueOnce(
      okResponse({ session_id: 's1', totals: { contacts: 12 }, estimated_fetch_seconds: 90 }),
    );
  vi.stubGlobal('fetch', fetchMock);

  const resp = await connectMonica('https://monica.example', 'tok-123');

  const [url, init] = fetchMock.mock.calls[0];
  expect(url).toContain('/contacts/import/monica/connect');
  expect(init.method).toBe('POST');
  expect(JSON.parse(init.body)).toEqual({
    base_url: 'https://monica.example',
    api_token: 'tok-123',
  });
  expect(resp.session_id).toBe('s1');
});

test('connectMonica surfaces the field error', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
  await expect(connectMonica('x', 'y')).rejects.toMatchObject({ code: 'INVALID_INPUT' });
});

test('startMonicaFetch sends the session id and the include flags', async () => {
  const fetchMock = vi.fn().mockResolvedValueOnce(okResponse({}));
  vi.stubGlobal('fetch', fetchMock);

  await startMonicaFetch('s1', { include_relationships: true, include_extras: false });

  const [url, init] = fetchMock.mock.calls[0];
  expect(url).toContain('/contacts/import/monica/fetch');
  expect(JSON.parse(init.body)).toEqual({
    session_id: 's1',
    include_relationships: true,
    include_extras: false,
  });
});

test('getMonicaImportStatus is a GET to the shared status endpoint', async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValueOnce(
      okResponse({ session_id: 's1', phase: 'ready', phase_done: 3, phase_total: 3 }),
    );
  vi.stubGlobal('fetch', fetchMock);

  const st = await getMonicaImportStatus('s1');

  const [url, init] = fetchMock.mock.calls[0];
  expect(url).toContain('/contacts/import/monica/status?session_id=s1');
  expect(init.method ?? 'GET').toBe('GET');
  expect(st.phase).toBe('ready');
});

test('confirmMonicaImport POSTs the per-row actions and resolves void on 202', async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValueOnce({ ok: true, status: 202, json: async () => ({ session_id: 's1' }) });
  vi.stubGlobal('fetch', fetchMock);

  const res = await confirmMonicaImport('s1', [
    { row_index: 0, action: 'add' },
    { row_index: 1, action: 'update' },
  ]);
  expect(res).toBeUndefined();

  const [url, init] = fetchMock.mock.calls[0];
  expect(url).toContain('/contacts/import/monica/confirm');
  expect(JSON.parse(init.body)).toEqual({
    session_id: 's1',
    actions: [
      { row_index: 0, action: 'add' },
      { row_index: 1, action: 'update' },
    ],
  });
});

test('getMonicaImportPreview GETs the shared preview endpoint', async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValueOnce(
      okResponse({ session_id: 's1', rows: [], total_rows: 0, loss_report: [] }),
    );
  vi.stubGlobal('fetch', fetchMock);

  const p = await getMonicaImportPreview('s1');
  expect(fetchMock.mock.calls[0][0]).toContain('/contacts/import/monica/preview?session_id=s1');
  expect(p.loss_report).toEqual([]);
});

test('cancelMonicaImport POSTs cancel and never throws', async () => {
  const fetchMock = vi.fn().mockRejectedValueOnce(new Error('network'));
  vi.stubGlobal('fetch', fetchMock);
  await expect(cancelMonicaImport('s1')).resolves.toBeUndefined();
  expect(fetchMock.mock.calls[0][0]).toContain('/contacts/import/monica/cancel?session_id=s1');
});
