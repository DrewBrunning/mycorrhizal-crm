import { afterEach, describe, expect, test, vi } from 'vitest';
import { getErrorAggregation } from './errorAggregation';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('getErrorAggregation', () => {
  test('requests /admin/error-aggregation with the window and parses the response', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        window_hours: 24,
        since: '2026-08-26T12:00:00Z',
        until: '2026-08-27T12:00:00Z',
        total_events: 21,
        buckets: [
          {
            component: 'contact_sync',
            cause: 'carddav authentication failed (http <n>)',
            sample_error: 'CardDAV authentication failed (HTTP 401)',
            event_types: ['sync_failed'],
            count: 17,
            recurring: true,
            first_seen: '2026-08-27T09:00:00Z',
            last_seen: '2026-08-27T11:30:00Z',
            event_ids: [1, 2, 3],
            event_ids_truncated: false,
          },
        ],
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const response = await getErrorAggregation(12);

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/admin/error-aggregation');
    expect(url).toContain('window_hours=12');
    expect(response.total_events).toBe(21);
    expect(response.buckets[0].count).toBe(17);
    expect(response.buckets[0].recurring).toBe(true);
    expect(response.buckets[0].event_ids).toEqual([1, 2, 3]);
  });

  test('defaults the window to 24h', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ window_hours: 24, since: '', until: '', total_events: 0, buckets: [] }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await getErrorAggregation();
    expect(fetchMock.mock.calls[0][0]).toContain('window_hours=24');
  });

  test('throws on a non-ok response', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: false,
      status: 403,
      json: async () => ({ error: 'forbidden' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(getErrorAggregation()).rejects.toBeDefined();
  });
});
