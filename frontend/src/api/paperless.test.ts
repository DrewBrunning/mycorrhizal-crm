import { describe, test, expect, vi, afterEach } from 'vitest';
import {
  getPaperlessConfig,
  savePaperlessConfig,
  deletePaperlessConfig,
  testPaperlessConnection,
  getPaperlessDocuments,
  linkPaperlessDocument,
  unlinkPaperlessDocument,
} from './paperless';

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

const configResponse = { base_url: 'https://paperless.example.com', has_api_token: true };

describe('getPaperlessConfig', () => {
  test('GETs /paperless/config and returns parsed data', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(configResponse));
    vi.stubGlobal('fetch', fetchMock);

    const result = await getPaperlessConfig();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/paperless/config');
    expect(init.method).toBeUndefined();
    expect(result).toEqual(configResponse);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(getPaperlessConfig()).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

describe('savePaperlessConfig', () => {
  test('PUTs the config to /paperless/config and returns parsed data', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(configResponse));
    vi.stubGlobal('fetch', fetchMock);

    const result = await savePaperlessConfig({ base_url: 'https://paperless.example.com', api_token: 'secret' });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/paperless/config');
    expect(init.method).toBe('PUT');
    expect(JSON.parse(init.body)).toEqual({
      base_url: 'https://paperless.example.com',
      api_token: 'secret',
    });
    expect(result).toEqual(configResponse);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(savePaperlessConfig({ base_url: 'x' })).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

describe('deletePaperlessConfig', () => {
  test('DELETEs /paperless/config', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse());
    vi.stubGlobal('fetch', fetchMock);

    await deletePaperlessConfig();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/paperless/config');
    expect(init.method).toBe('DELETE');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(deletePaperlessConfig()).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

describe('testPaperlessConnection', () => {
  test('POSTs to /paperless/test-connection and returns the result', async () => {
    const resultBody = { ok: false, stage: 'auth', message: 'token invalid' };
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(resultBody));
    vi.stubGlobal('fetch', fetchMock);

    const result = await testPaperlessConnection();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/paperless/test-connection');
    expect(init.method).toBe('POST');
    expect(result).toEqual(resultBody);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(testPaperlessConnection()).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

describe('getPaperlessDocuments', () => {
  test('GETs /paperless/documents with an encoded query param', async () => {
    const documents = [
      { id: 1, title: 'Invoice', file_name: 'invoice.pdf', created: '2026-01-01', added: '2026-01-02' },
    ];
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse({ documents }));
    vi.stubGlobal('fetch', fetchMock);

    const result = await getPaperlessDocuments('electric bill');

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/paperless/documents');
    expect(url).toContain('query=electric%20bill');
    expect(init.method).toBeUndefined();
    expect(result).toEqual(documents);
  });

  test('GETs /paperless/documents without a query string when query is omitted', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse({ documents: [] }));
    vi.stubGlobal('fetch', fetchMock);

    await getPaperlessDocuments();

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/paperless/documents');
    expect(url).not.toContain('query=');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(getPaperlessDocuments('x')).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

describe('linkPaperlessDocument', () => {
  test('POSTs the document id as a string to the contact link endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse());
    vi.stubGlobal('fetch', fetchMock);

    await linkPaperlessDocument('alice-uid', 42);

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/paperless/contacts/alice-uid/link');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual({ document_id: '42' });
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(linkPaperlessDocument('alice-uid', 42)).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

describe('unlinkPaperlessDocument', () => {
  test('DELETEs the contact link endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse());
    vi.stubGlobal('fetch', fetchMock);

    await unlinkPaperlessDocument('alice-uid', 'identity-1');

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/paperless/contacts/alice-uid/links/identity-1');
    expect(init.method).toBe('DELETE');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(unlinkPaperlessDocument('alice-uid', 'identity-1')).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});
