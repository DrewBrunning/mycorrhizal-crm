import { afterEach, describe, expect, test, vi } from 'vitest';
import { rebuildSearchIndex, searchAll } from './search';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('searchAll', () => {
  test('requests /search with the query and parses grouped results', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        query: 'symphony',
        contacts: [{ id: 7, uid: 'c-7', firstname: 'Wolfgang' }],
        notes: [],
        activities: [],
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await searchAll('symphony');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/search?');
    expect(url).toContain('q=symphony');
    expect(result.contacts[0].firstname).toBe('Wolfgang');
  });

  test('encodes special characters in the query', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ query: 'it is "great"', contacts: [], notes: [], activities: [] }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await searchAll('it is "great"');

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('q=it+is+%22great%22');
  });
});

describe('rebuildSearchIndex', () => {
  test('POSTs to the admin rebuild endpoint', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({ message: 'Search index rebuilt' }) });
    vi.stubGlobal('fetch', fetchMock);

    await rebuildSearchIndex();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/admin/search/rebuild');
    expect(init.method).toBe('POST');
  });
});
