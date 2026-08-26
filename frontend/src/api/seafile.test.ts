import { afterEach, describe, expect, test, vi } from 'vitest';
import {
  deleteSeafileConfig,
  getSeafileConfig,
  getSeafileDir,
  getSeafileLibraries,
  linkSeafileItem,
  saveSeafileConfig,
  testSeafileConnection,
  unlinkSeafileItem,
} from './seafile';

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

const configResponse = { base_url: 'https://seafile.example.com', has_api_token: true };

describe('getSeafileConfig', () => {
  test('GETs /seafile/config and returns parsed data', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(configResponse));
    vi.stubGlobal('fetch', fetchMock);

    const result = await getSeafileConfig();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/seafile/config');
    expect(init.method).toBeUndefined();
    expect(result).toEqual(configResponse);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(getSeafileConfig()).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('saveSeafileConfig', () => {
  test('PUTs the config to /seafile/config and returns parsed data', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(configResponse));
    vi.stubGlobal('fetch', fetchMock);

    const result = await saveSeafileConfig({
      base_url: 'https://seafile.example.com',
      api_token: 'secret',
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/seafile/config');
    expect(init.method).toBe('PUT');
    expect(JSON.parse(init.body)).toEqual({
      base_url: 'https://seafile.example.com',
      api_token: 'secret',
    });
    expect(result).toEqual(configResponse);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(saveSeafileConfig({ base_url: 'x' })).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('deleteSeafileConfig', () => {
  test('DELETEs /seafile/config', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse());
    vi.stubGlobal('fetch', fetchMock);

    await deleteSeafileConfig();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/seafile/config');
    expect(init.method).toBe('DELETE');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(deleteSeafileConfig()).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('testSeafileConnection', () => {
  test('POSTs to /seafile/test-connection and returns the result', async () => {
    const resultBody = { ok: true, stage: 'auth', message: 'connected' };
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(resultBody));
    vi.stubGlobal('fetch', fetchMock);

    const result = await testSeafileConnection();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/seafile/test-connection');
    expect(init.method).toBe('POST');
    expect(result).toEqual(resultBody);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(testSeafileConnection()).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('getSeafileLibraries', () => {
  test('GETs /seafile/libraries and returns result.libraries', async () => {
    const libraries = [{ id: 'lib-1', name: 'Docs', type: 'library' }];
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse({ libraries }));
    vi.stubGlobal('fetch', fetchMock);

    const result = await getSeafileLibraries();

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/seafile/libraries');
    expect(init.method).toBeUndefined();
    expect(result).toEqual(libraries);
  });

  test('returns an empty array when libraries is missing', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(okResponse({})));

    const result = await getSeafileLibraries();

    expect(result).toEqual([]);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(getSeafileLibraries()).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('getSeafileDir', () => {
  test('GETs the repo dir with the encoded repo id and path', async () => {
    const items = [
      {
        id: 'item-1',
        name: 'report.pdf',
        type: 'file' as const,
        size: 1024,
        mtime: 1700000000,
        parent_dir: '/',
      },
    ];
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse({ items }));
    vi.stubGlobal('fetch', fetchMock);

    const result = await getSeafileDir('repo 1', '/Reports');

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/seafile/libraries/repo%201/dir');
    expect(url).toContain('path=%2FReports');
    expect(init.method).toBeUndefined();
    expect(result).toEqual(items);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(getSeafileDir('repo-1', '/')).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('linkSeafileItem', () => {
  test('POSTs the item JSON to the contact link endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse());
    vi.stubGlobal('fetch', fetchMock);

    await linkSeafileItem('alice-uid', {
      repo_id: 'repo-1',
      path: '/Docs',
      name: 'report.pdf',
      type: 'file',
      size: 1024,
      mtime: 1700000000,
    });

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/seafile/contacts/alice-uid/link');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual({
      repo_id: 'repo-1',
      path: '/Docs',
      name: 'report.pdf',
      type: 'file',
      size: 1024,
      mtime: 1700000000,
    });
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(
      linkSeafileItem('alice-uid', { repo_id: 'r', path: '/', name: 'x', type: 'file' }),
    ).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

describe('unlinkSeafileItem', () => {
  test('DELETEs the contact link endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse());
    vi.stubGlobal('fetch', fetchMock);

    await unlinkSeafileItem('alice-uid', 'identity-1');

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/seafile/contacts/alice-uid/links/identity-1');
    expect(init.method).toBe('DELETE');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(unlinkSeafileItem('alice-uid', 'identity-1')).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});
