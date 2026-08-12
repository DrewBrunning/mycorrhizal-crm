import { describe, test, expect, vi, afterEach } from 'vitest';
import { getTimeline, TIMELINE_TYPES, TIMELINE_BUCKETS } from './timeline';

// The T66 timeline endpoint's query contract is the part worth pinning
// client-side: the comma-joined type filter, the bucket token, and the
// cursor/limit passthrough. A URL regression here would silently change what
// rows the explorer (T78) shows.

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('getTimeline', () => {
  test('defaults to limit=25 and no type/bucket filter', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ items: [], next_cursor: '', limit: 25 }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await getTimeline({ contactId: 42 });

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/contacts/42/timeline?limit=25');
    expect(url).not.toContain('type=');
    expect(url).not.toContain('bucket=');
  });

  test('joins a subset of types into the comma-separated ?type= token', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ items: [], next_cursor: '', limit: 25 }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await getTimeline({ contactId: 'uid-1', types: ['note', 'gift'], limit: 10 });

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/contacts/uid-1/timeline?limit=10');
    expect(url).toContain('type=note%2Cgift');
  });

  test('omits ?type= when all six types are selected (the backend default)', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ items: [], next_cursor: '', limit: 25 }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await getTimeline({ contactId: 42, types: [...TIMELINE_TYPES] });

    const [url] = fetchMock.mock.calls[0];
    expect(url).not.toContain('type=');
  });

  test('sends the recency bucket and cursor', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ items: [], next_cursor: '', limit: 25 }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await getTimeline({ contactId: 42, bucket: 'last_30_days', cursor: 'abc123' });

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('bucket=last_30_days');
    expect(url).toContain('cursor=abc123');
  });

  test('omits ?bucket= for the "all" default', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ items: [], next_cursor: '', limit: 25 }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await getTimeline({ contactId: 42, bucket: 'all' });

    const [url] = fetchMock.mock.calls[0];
    expect(url).not.toContain('bucket=');
  });

  test('returns the parsed page', async () => {
    const page = {
      items: [{ type: 'note', id: '7', date: '2026-08-12T10:00:00Z', data: { content: 'hi' } }],
      next_cursor: 'next',
      limit: 25,
    };
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValueOnce({ ok: true, json: async () => page })
    );

    const result = await getTimeline({ contactId: 42 });
    expect(result).toEqual(page);
  });

  test('throws the parsed ApiError on a 400 (unknown type token)', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValueOnce({
        ok: false,
        status: 400,
        json: async () => ({
          error: { code: 'INVALID_INPUT', message: 'unknown timeline type "banana"' },
        }),
      })
    );

    await expect(getTimeline({ contactId: 42, types: ['banana' as never] })).rejects.toThrow(
      /unknown timeline type/
    );
  });
});

describe('registry mirrors', () => {
  test('TIMELINE_TYPES has the six backend tokens, in canonical order', () => {
    expect(TIMELINE_TYPES).toEqual([
      'note', 'activity', 'completion', 'life_event', 'external_activity', 'gift',
    ]);
  });

  test('TIMELINE_BUCKETS matches the backend vocabulary', () => {
    expect(TIMELINE_BUCKETS).toEqual([
      'last_7_days', 'last_30_days', 'last_90_days', 'this_year', 'all',
    ]);
  });
});
