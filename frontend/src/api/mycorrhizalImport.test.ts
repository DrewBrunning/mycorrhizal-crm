import { afterEach, expect, test, vi } from 'vitest';
import {
  cancelMycorrhizalImport,
  confirmMycorrhizalImport,
  getMycorrhizalImportStatus,
  startMycorrhizalFetch,
  uploadMycorrhizalBundle,
} from './mycorrhizalImport';

afterEach(() => {
  vi.unstubAllGlobals();
});

function okResponse(body?: unknown) {
  return { ok: true, json: async () => body };
}

const emptyTotals = {
  contacts: 3,
  relationships: 1,
  notes: 2,
  reminders: 0,
  reminder_completions: 0,
  activities: 0,
  life_events: 0,
  gifts: 0,
  preferences: 0,
  conversation_agenda: 0,
  cadence_policies: 0,
  data_decay_policies: 0,
  households: 0,
  circles: 0,
  tags: 0,
  custom_field_definitions: 0,
  custom_field_values: 0,
  occasions: 0,
  occasion_events: 0,
};

test('uploadMycorrhizalBundle POSTs multipart form-data and returns the totals', async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValueOnce(okResponse({ session_id: 's1', version: 1, totals: emptyTotals }));
  vi.stubGlobal('fetch', fetchMock);

  const file = new File(['{}'], 'bundle.json', { type: 'application/json' });
  const resp = await uploadMycorrhizalBundle(file);

  const [url, init] = fetchMock.mock.calls[0];
  expect(url).toContain('/import/mycorrhizal/upload');
  expect(init.method).toBe('POST');
  expect(init.body).toBeInstanceOf(FormData);
  expect((init.body as FormData).get('file')).toBeInstanceOf(File);
  expect(resp.version).toBe(1);
  expect(resp.totals.contacts).toBe(3);
});

test('uploadMycorrhizalBundle surfaces a wrong-version rejection', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValueOnce({
      ok: false,
      status: 422,
      json: async () => ({
        error: { code: 'INVALID_INPUT', message: 'Unsupported account bundle version' },
      }),
    }),
  );
  await expect(uploadMycorrhizalBundle(new File(['{}'], 'b.json'))).rejects.toMatchObject({
    code: 'INVALID_INPUT',
  });
});

test('startMycorrhizalFetch sends only the session id', async () => {
  const fetchMock = vi.fn().mockResolvedValue(okResponse({}));
  vi.stubGlobal('fetch', fetchMock);

  await startMycorrhizalFetch('s1');
  expect(fetchMock.mock.calls[0][0]).toContain('/import/mycorrhizal/fetch');
  expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ session_id: 's1' });
});

test('the shared status/confirm/cancel calls are bound to the mycorrhizal base path', async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValueOnce(
      okResponse({ session_id: 's1', phase: 'ready', phase_done: 0, phase_total: 0 }),
    )
    .mockResolvedValueOnce({ ok: true, status: 202, json: async () => ({}) })
    .mockRejectedValueOnce(new Error('net'));
  vi.stubGlobal('fetch', fetchMock);

  await getMycorrhizalImportStatus('s1');
  const res = await confirmMycorrhizalImport('s1', [{ row_index: 0, action: 'add' }]);
  expect(res).toBeUndefined();
  await expect(cancelMycorrhizalImport('s1')).resolves.toBeUndefined();

  expect(fetchMock.mock.calls[0][0]).toContain('/import/mycorrhizal/status?session_id=s1');
  expect(fetchMock.mock.calls[1][0]).toContain('/import/mycorrhizal/confirm');
  expect(fetchMock.mock.calls[2][0]).toContain('/import/mycorrhizal/cancel?session_id=s1');
});
