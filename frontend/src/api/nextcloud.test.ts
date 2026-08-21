import { describe, test, expect, vi, afterEach } from 'vitest';
import {
  getNextcloudConfig,
  saveNextcloudConfig,
  deleteNextcloudConfig,
  testNextcloudConnection,
  getNextcloudDir,
  linkNextcloudItem,
  unlinkNextcloudItem,
} from './nextcloud';

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
      error: { code: 'VALIDATION_ERROR', message: 'nope', details: { name: 'base_url' } },
      request_id: 'req-1',
    }),
  };
}

const configResponse = { base_url: 'https://nc.example.com', username: 'ada', has_app_password: true };

describe('getNextcloudConfig', () => {
  test('GETs /nextcloud/config and returns parsed data', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(configResponse));
    vi.stubGlobal('fetch', fetchMock);

    const result = await getNextcloudConfig();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/nextcloud/config');
    expect(init.method).toBeUndefined();
    expect(result).toEqual(configResponse);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(getNextcloudConfig()).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

describe('saveNextcloudConfig', () => {
  test('PUTs the config to /nextcloud/config and returns parsed data', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(configResponse));
    vi.stubGlobal('fetch', fetchMock);

    const result = await saveNextcloudConfig({ base_url: 'https://nc.example.com', username: 'ada', app_password: 'p' });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/nextcloud/config');
    expect(init.method).toBe('PUT');
    expect(JSON.parse(init.body)).toEqual({
      base_url: 'https://nc.example.com',
      username: 'ada',
      app_password: 'p',
    });
    expect(result).toEqual(configResponse);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(saveNextcloudConfig({ base_url: 'x', username: 'a' })).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('deleteNextcloudConfig', () => {
  test('DELETEs /nextcloud/config', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse());
    vi.stubGlobal('fetch', fetchMock);

    await deleteNextcloudConfig();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/nextcloud/config');
    expect(init.method).toBe('DELETE');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(deleteNextcloudConfig()).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

describe('testNextcloudConnection', () => {
  test('POSTs to /nextcloud/test-connection and returns the result', async () => {
    const resultBody = { ok: true, stage: 'auth', message: 'connected' };
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(resultBody));
    vi.stubGlobal('fetch', fetchMock);

    const result = await testNextcloudConnection();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/nextcloud/test-connection');
    expect(init.method).toBe('POST');
    expect(result).toEqual(resultBody);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(testNextcloudConnection()).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

describe('getNextcloudDir', () => {
  test('GETs /nextcloud/dir with an encoded path param', async () => {
    const items = [
      { name: 'notes.md', path: '/docs/notes.md', type: 'file' as const, size: 2048, modified_at: '2026-01-01T00:00:00Z' },
    ];
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse({ items }));
    vi.stubGlobal('fetch', fetchMock);

    const result = await getNextcloudDir('/docs/notes');

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/nextcloud/dir');
    expect(url).toContain('path=%2Fdocs%2Fnotes');
    expect(init.method).toBeUndefined();
    expect(result).toEqual(items);
  });

  test('GETs /nextcloud/dir without a query string when path is omitted', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse({ items: [] }));
    vi.stubGlobal('fetch', fetchMock);

    await getNextcloudDir();

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/nextcloud/dir');
    expect(url).not.toContain('path=');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(getNextcloudDir('/')).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

describe('linkNextcloudItem', () => {
  test('POSTs the item JSON to the contact link endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse());
    vi.stubGlobal('fetch', fetchMock);

    await linkNextcloudItem('alice-uid', {
      path: '/docs/report.pdf',
      name: 'report.pdf',
      type: 'file',
      size: 1024,
      modified_at: '2026-01-01T00:00:00Z',
      file_id: 'f-1',
    });

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/nextcloud/contacts/alice-uid/link');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual({
      path: '/docs/report.pdf',
      name: 'report.pdf',
      type: 'file',
      size: 1024,
      modified_at: '2026-01-01T00:00:00Z',
      file_id: 'f-1',
    });
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(linkNextcloudItem('alice-uid', { path: '/', name: 'x', type: 'dir' })).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('unlinkNextcloudItem', () => {
  test('DELETEs the contact link endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse());
    vi.stubGlobal('fetch', fetchMock);

    await unlinkNextcloudItem('alice-uid', 'identity-1');

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/nextcloud/contacts/alice-uid/links/identity-1');
    expect(init.method).toBe('DELETE');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(unlinkNextcloudItem('alice-uid', 'identity-1')).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});
