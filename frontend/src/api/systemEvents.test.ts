import { afterEach, describe, expect, test, vi } from 'vitest';
import { getSystemEvents, SYSTEM_EVENT_SEVERITIES, SYSTEM_EVENT_TYPES } from './systemEvents';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('getSystemEvents', () => {
  test('requests /admin/system-events with the default limit and parses the response', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        system_events: [
          {
            id: 7,
            created_at: '2026-08-27T10:00:00Z',
            occurred_at: '2026-08-27T10:00:00Z',
            event_type: 'sync_failed',
            severity: 'error',
            component: 'contact_sync',
            correlation_id: 'chain-A',
          },
        ],
        total: 1,
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const response = await getSystemEvents();

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/admin/system-events?');
    expect(url).toContain('limit=100');
    expect(response.total).toBe(1);
    expect(response.system_events[0].event_type).toBe('sync_failed');
  });

  test('sends every filter that is set', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({ system_events: [], total: 0 }) });
    vi.stubGlobal('fetch', fetchMock);

    await getSystemEvents({
      component: 'scheduler',
      severity: 'error',
      event_type: 'job_failed',
      correlation_id: 'job:purge_deleted:abc',
      ids: [11, 12, 13],
      since: '2026-08-01T00:00:00Z',
      limit: 250,
    });

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('component=scheduler');
    expect(url).toContain('severity=error');
    expect(url).toContain('event_type=job_failed');
    expect(url).toContain('correlation_id=job%3Apurge_deleted%3Aabc');
    expect(url).toContain('ids=11%2C12%2C13');
    expect(url).toContain('since=2026-08-01');
    expect(url).toContain('limit=250');
  });

  test('omits the ids filter when the list is empty', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({ system_events: [], total: 0 }) });
    vi.stubGlobal('fetch', fetchMock);

    await getSystemEvents({ ids: [] });
    expect(fetchMock.mock.calls[0][0]).not.toContain('ids=');
  });

  test('omits empty filters', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({ system_events: [], total: 0 }) });
    vi.stubGlobal('fetch', fetchMock);

    await getSystemEvents({ component: '', severity: '', correlation_id: '' });

    const [url] = fetchMock.mock.calls[0];
    expect(url).not.toContain('component=');
    expect(url).not.toContain('severity=');
    expect(url).not.toContain('correlation_id=');
  });

  test('throws on a non-ok response', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: false,
      status: 403,
      json: async () => ({ error: 'forbidden' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(getSystemEvents()).rejects.toBeDefined();
  });
});

describe('enum mirrors', () => {
  test('event-type and severity lists are the frozen backend vocabulary', () => {
    expect(SYSTEM_EVENT_TYPES).toHaveLength(17);
    expect(SYSTEM_EVENT_SEVERITIES).toEqual(['info', 'warn', 'error']);
  });
});
