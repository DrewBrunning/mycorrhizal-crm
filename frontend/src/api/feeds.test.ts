import { afterEach, describe, expect, test, vi } from 'vitest';
import {
  absoluteFeedUrl,
  createFeed,
  type Feed,
  listFeeds,
  revokeAllFeeds,
  revokeFeed,
  rotateFeed,
} from './feeds';

afterEach(() => {
  vi.unstubAllGlobals();
});

function reply(status: number, body?: unknown) {
  return {
    ok: status >= 200 && status < 300,
    status,
    text: async () => (body === undefined ? '' : JSON.stringify(body)),
  };
}

function stubFetch(...responses: ReturnType<typeof reply>[]) {
  const fetchMock = vi.fn();
  for (const r of responses) fetchMock.mockResolvedValueOnce(r);
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

const feed: Feed = {
  id: 'f1',
  name: 'All contacts',
  kind: 'aggregate',
  entity_id: '',
  detail: 'headlines',
  created_at: '2026-01-01T00:00:00Z',
  last_accessed_at: null,
};

describe('absoluteFeedUrl', () => {
  test('prefixes the page origin onto a relative URL', () => {
    expect(absoluteFeedUrl('/api/v1/feeds/atom?token=t')).toBe(
      `${window.location.origin}/api/v1/feeds/atom?token=t`,
    );
  });

  test('leaves an absolute URL untouched', () => {
    const url = 'https://crm.example.com/api/v1/feeds/atom?token=t';
    expect(absoluteFeedUrl(url)).toBe(url);
  });
});

describe('listFeeds', () => {
  test('GETs /feeds and unwraps the feeds array', async () => {
    const fetchMock = stubFetch(reply(200, { feeds: [feed] }));
    expect(await listFeeds()).toEqual([feed]);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toMatch(/\/feeds$/);
    expect(init.method).toBe('GET');
  });

  test('a body with no feeds key yields an empty list', async () => {
    stubFetch(reply(200, {}));
    expect(await listFeeds()).toEqual([]);
  });

  test('surfaces the server error message', async () => {
    stubFetch(reply(500, { error: { message: 'boom' } }));
    await expect(listFeeds()).rejects.toThrow('boom');
  });
});

describe('createFeed', () => {
  test('POSTs the input and returns the feed with its one-time URL', async () => {
    const fetchMock = stubFetch(reply(201, { feed, url: '/api/v1/feeds/atom?token=t' }));
    const result = await createFeed({ name: 'All contacts', kind: 'aggregate' });
    expect(result.url).toBe('/api/v1/feeds/atom?token=t');
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toMatch(/\/feeds$/);
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual({ name: 'All contacts', kind: 'aggregate' });
  });

  test('surfaces a 422 feed-limit message', async () => {
    stubFetch(reply(422, { error: { message: 'feed limit reached' } }));
    await expect(createFeed({ name: 'x', kind: 'aggregate' })).rejects.toThrow(
      'feed limit reached',
    );
  });
});

describe('rotateFeed', () => {
  test('POSTs to the rotate endpoint and returns the replacement URL', async () => {
    const fetchMock = stubFetch(reply(201, { feed, url: '/api/v1/feeds/atom?token=new' }));
    const result = await rotateFeed('f1');
    expect(result.url).toContain('token=new');
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toMatch(/\/feeds\/f1\/rotate$/);
    expect(init.method).toBe('POST');
  });

  test('a 404 rejects', async () => {
    stubFetch(reply(404, { error: { message: 'not found' } }));
    await expect(rotateFeed('gone')).rejects.toThrow('not found');
  });
});

describe('revokeFeed', () => {
  test('DELETEs the feed and tolerates the empty 204 body', async () => {
    const fetchMock = stubFetch(reply(204));
    await expect(revokeFeed('f1')).resolves.toBeUndefined();
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toMatch(/\/feeds\/f1$/);
    expect(init.method).toBe('DELETE');
  });

  test('a 404 rejects', async () => {
    stubFetch(reply(404, { error: { message: 'not found' } }));
    await expect(revokeFeed('gone')).rejects.toThrow('not found');
  });
});

describe('revokeAllFeeds', () => {
  test('POSTs revoke-all and returns the count', async () => {
    const fetchMock = stubFetch(reply(200, { revoked: 3 }));
    expect(await revokeAllFeeds()).toEqual({ revoked: 3 });
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toMatch(/\/feeds\/revoke-all$/);
    expect(init.method).toBe('POST');
  });

  test('a body without a numeric count reports zero', async () => {
    stubFetch(reply(200, {}));
    expect(await revokeAllFeeds()).toEqual({ revoked: 0 });
  });
});
