import { afterEach, describe, expect, test, vi } from 'vitest';
import {
  type CardDAVSyncScope,
  getCardDAVSyncScope,
  previewCardDAVSyncScope,
  putCardDAVSyncScope,
} from './carddavSyncScope';

afterEach(() => vi.unstubAllGlobals());

const scope: CardDAVSyncScope = {
  mode: 'include',
  circle_ids: ['c1'],
  default_circle_id: 'c1',
  updated_at: '2026-01-01T00:00:00Z',
  in_scope_count: 2,
  total_count: 5,
  warnings: [],
};

describe('carddav sync scope API', () => {
  test('GETs the caller scope', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, json: async () => scope });
    vi.stubGlobal('fetch', fetchMock);

    expect(await getCardDAVSyncScope()).toEqual(scope);
    expect(fetchMock.mock.calls[0][0]).toContain('/api/v1/users/me/carddav-sync-scope');
    expect(fetchMock.mock.calls[0][1].method).toBeUndefined();
  });

  test('PUTs the whole object', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, json: async () => scope });
    vi.stubGlobal('fetch', fetchMock);

    const input = { mode: 'include' as const, circle_ids: ['c1'], default_circle_id: 'c1' };
    expect(await putCardDAVSyncScope(input)).toEqual(scope);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/api/v1/users/me/carddav-sync-scope');
    expect(init.method).toBe('PUT');
    expect(JSON.parse(init.body)).toEqual(input);
  });

  test('POSTs the preview without persisting', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        mode: 'all',
        circle_ids: [],
        in_scope_count: 5,
        total_count: 5,
        warnings: [],
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await previewCardDAVSyncScope({ mode: 'all', circle_ids: [], default_circle_id: null });
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/api/v1/users/me/carddav-sync-scope/preview');
    expect(init.method).toBe('POST');
  });
});
