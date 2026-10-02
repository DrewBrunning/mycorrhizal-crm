import { afterEach, describe, expect, test, vi } from 'vitest';
import {
  deleteGeoPulseConfig,
  getGeoPulseConfig,
  getGeoPulseSuggestions,
  saveGeoPulseConfig,
  testGeoPulseConnection,
} from './geopulse';

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
      error: { code: 'VALIDATION_ERROR', message: 'nope', details: {} },
      request_id: 'req-1',
    }),
  };
}

const configResponse = { base_url: 'https://geopulse.example.com', has_api_key: true };

describe('getGeoPulseConfig', () => {
  test('GETs /geopulse/config and returns parsed data', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(configResponse));
    vi.stubGlobal('fetch', fetchMock);

    const result = await getGeoPulseConfig();

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/geopulse/config');
    expect(init.method).toBeUndefined();
    expect(result).toEqual(configResponse);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(getGeoPulseConfig()).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('saveGeoPulseConfig', () => {
  test('PUTs the config (api_key is write-only) and returns parsed data', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(configResponse));
    vi.stubGlobal('fetch', fetchMock);

    const result = await saveGeoPulseConfig({
      base_url: 'https://geopulse.example.com',
      api_key: 'secret',
    });

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/geopulse/config');
    expect(init.method).toBe('PUT');
    expect(JSON.parse(init.body)).toEqual({
      base_url: 'https://geopulse.example.com',
      api_key: 'secret',
    });
    expect(result).toEqual(configResponse);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(saveGeoPulseConfig({ base_url: 'nope' })).rejects.toMatchObject({ status: 400 });
  });
});

describe('deleteGeoPulseConfig', () => {
  test('DELETEs /geopulse/config', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse({ message: 'ok' }));
    vi.stubGlobal('fetch', fetchMock);

    await deleteGeoPulseConfig();

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/geopulse/config');
    expect(init.method).toBe('DELETE');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(deleteGeoPulseConfig()).rejects.toMatchObject({ status: 400 });
  });
});

describe('testGeoPulseConnection', () => {
  test('POSTs /geopulse/test-connection and returns the diagnosis', async () => {
    const result = { ok: false, stage: 'auth', message: 'rejected' };
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(result));
    vi.stubGlobal('fetch', fetchMock);

    expect(await testGeoPulseConnection()).toEqual(result);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/geopulse/test-connection');
    expect(init.method).toBe('POST');
  });

  test('throws an ApiError when there is no usable saved connection', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(testGeoPulseConnection()).rejects.toMatchObject({ status: 400 });
  });
});

describe('getGeoPulseSuggestions', () => {
  const suggestions = { date: '2026-09-20', suggestions: [] };

  test('GETs /geopulse/suggestions with the date and timezone as query params', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(suggestions));
    vi.stubGlobal('fetch', fetchMock);

    const result = await getGeoPulseSuggestions('2026-09-20', 'Europe/London');

    const [url, init] = fetchMock.mock.calls[0];
    const parsed = new URL(url, 'http://localhost');
    expect(parsed.pathname.endsWith('/geopulse/suggestions')).toBe(true);
    expect(parsed.searchParams.get('date')).toBe('2026-09-20');
    expect(parsed.searchParams.get('timezone')).toBe('Europe/London');
    expect(init.method).toBeUndefined();
    expect(result).toEqual(suggestions);
  });

  test('omits the timezone param when none is given (server defaults to UTC)', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(suggestions));
    vi.stubGlobal('fetch', fetchMock);

    await getGeoPulseSuggestions('2026-09-20');

    const parsed = new URL(fetchMock.mock.calls[0][0], 'http://localhost');
    expect(parsed.searchParams.has('timezone')).toBe(false);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(getGeoPulseSuggestions('2026-09-20')).rejects.toMatchObject({ status: 400 });
  });
});
