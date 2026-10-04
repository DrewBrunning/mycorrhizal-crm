import { afterEach, describe, expect, test, vi } from 'vitest';

import {
  formatGeoUri,
  geocodeAddress,
  getMapConfig,
  getMapPoints,
  parseCoordinateInput,
  parseGeoUri,
} from './map';

afterEach(() => {
  vi.unstubAllGlobals();
});

const errorResponse = () => ({
  ok: false,
  status: 400,
  statusText: 'Bad Request',
  json: async () => ({ error: { code: 'VALIDATION_ERROR', message: 'nope' }, request_id: 'r' }),
});

describe('parseGeoUri', () => {
  test.each([
    ['geo:51.5007,-0.1246', { lat: 51.5007, lng: -0.1246 }],
    ['GEO:-33,151', { lat: -33, lng: 151 }],
    ['geo:48.2,16.3,190', { lat: 48.2, lng: 16.3 }],
    ['geo:48.2,16.3;u=35', { lat: 48.2, lng: 16.3 }],
    ['  geo:90,180  ', { lat: 90, lng: 180 }],
  ])('parses %s', (input, want) => {
    expect(parseGeoUri(input)).toEqual(want);
  });

  test.each([
    undefined,
    null,
    '',
    'geo:',
    'geo:abc,def',
    'geo:91,0',
    'geo:-91,0',
    'geo:0,181',
    'geo:0,-181',
    'http://x/1,2',
    'geo:1',
    'geo:1,2,x',
    'geo:1,2,3,4',
  ])('rejects %s', (input) => {
    expect(parseGeoUri(input as string | undefined | null)).toBeNull();
  });
});

describe('formatGeoUri', () => {
  test('round-trips ordinary values through parseGeoUri', () => {
    expect(formatGeoUri(51.5, -0.12)).toBe('geo:51.5,-0.12');
    expect(parseGeoUri(formatGeoUri(51.5, -0.12))).toEqual({ lat: 51.5, lng: -0.12 });
  });

  // Regression for #1447: Number#toString switches to scientific notation below
  // 1e-3 (e.g. 1e-7 -> "1e-7"), which decimal()'s /^[-0-9.]+$/ rejects, so a
  // small coordinate written by the editor failed to reload. The fixed-point
  // formatter must always emit a plain decimal.
  test.each([
    [0.0001, -0.0004, { lat: 0.0001, lng: -0.0004 }],
    [-0.0004, 0.0001, { lat: -0.0004, lng: 0.0001 }],
    [0.001, -0.001, { lat: 0.001, lng: -0.001 }],
    // 1e-7 is below the shared six-decimal precision, so it rounds to zero.
    [1e-7, 0, { lat: 0, lng: 0 }],
    [0, 0, { lat: 0, lng: 0 }],
    [-33.8688, 151.2093, { lat: -33.8688, lng: 151.2093 }],
    [90, 180, { lat: 90, lng: 180 }],
  ])('round-trips %s,%s without scientific notation', (lat, lng, expected) => {
    const uri = formatGeoUri(lat, lng);
    // The `geo:` scheme itself contains an `e`; the coordinates must not.
    expect(uri.slice(4)).not.toMatch(/[eE]/);
    expect(parseGeoUri(uri)).toEqual(expected);
  });

  test('emits the fixed-point form the backend stores', () => {
    expect(formatGeoUri(0.0001, -0.0004)).toBe('geo:0.0001,-0.0004');
    expect(formatGeoUri(1e-7, 0)).toBe('geo:0,0');
    expect(formatGeoUri(0, -0)).toBe('geo:0,0');
  });
});

describe('parseCoordinateInput', () => {
  test.each([
    ['51.5007, -0.1246', { lat: 51.5007, lng: -0.1246 }],
    ['51.5007,-0.1246', { lat: 51.5007, lng: -0.1246 }],
    ['  10 20 ', { lat: 10, lng: 20 }],
    ['-90, 180', { lat: -90, lng: 180 }],
  ])('parses %s', (input, want) => {
    expect(parseCoordinateInput(input)).toEqual(want);
  });

  test.each(['', '12', '1,2,3', 'a, b', '91, 0', '0, 181', '-91, 0', '1.2.3, 4', '1;2'])(
    'rejects %j',
    (input) => {
      expect(parseCoordinateInput(input)).toBeNull();
    },
  );
});

describe('getMapConfig', () => {
  test('GETs /config/map', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ tile_style_url: 'https://tiles.example/style' }),
    });
    vi.stubGlobal('fetch', fetchMock);
    await expect(getMapConfig()).resolves.toEqual({
      tile_style_url: 'https://tiles.example/style',
    });
    expect(fetchMock.mock.calls[0][0]).toMatch(/\/config\/map$/);
  });

  test('throws the parsed error on failure', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(getMapConfig()).rejects.toThrow('nope');
  });
});

describe('geocodeAddress', () => {
  test('POSTs without include_sensitive by default', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ address_id: 'a/1', coordinates: 'geo:1,2', cached: false }),
    });
    vi.stubGlobal('fetch', fetchMock);
    const result = await geocodeAddress(7, 'a/1');
    expect(result.coordinates).toBe('geo:1,2');
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toMatch(/\/contacts\/7\/addresses\/a%2F1\/geocode$/);
    expect(init.method).toBe('POST');
  });

  test('sends include_sensitive=true when opted in', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ address_id: 'a', coordinates: 'geo:1,2', cached: true }),
    });
    vi.stubGlobal('fetch', fetchMock);
    await geocodeAddress('7', 'a', true);
    expect(fetchMock.mock.calls[0][0]).toMatch(/geocode\?include_sensitive=true$/);
  });

  test('throws the parsed error on failure', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(geocodeAddress(1, 'a')).rejects.toThrow('nope');
  });
});

describe('getMapPoints', () => {
  const wire = (patch: object = {}) => ({
    contact_id: 1,
    contact_uid: 'u1',
    contact_name: 'Ada Lovelace',
    address_id: 'a1',
    label: '10 Downing St, London',
    coordinates: 'geo:51.5034,-0.1276',
    ...patch,
  });

  test('GETs /contacts/map and maps points to lat/lng', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        points: [wire(), wire({ contact_id: 2, address_id: 'a2', coordinates: 'geo:1,2' })],
        truncated: false,
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await getMapPoints();

    expect(fetchMock.mock.calls[0][0]).toMatch(/\/contacts\/map$/);
    expect(result.truncated).toBe(false);
    expect(result.points).toEqual([
      {
        contactId: 1,
        contactName: 'Ada Lovelace',
        addressId: 'a1',
        label: '10 Downing St, London',
        lat: 51.5034,
        lng: -0.1276,
      },
      {
        contactId: 2,
        contactName: 'Ada Lovelace',
        addressId: 'a2',
        label: '10 Downing St, London',
        lat: 1,
        lng: 2,
      },
    ]);
  });

  test('skips a point whose coordinates the client cannot parse', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          points: [wire({ coordinates: 'geo:999,0' }), wire({ address_id: 'ok' })],
          truncated: false,
        }),
      }),
    );
    const { points } = await getMapPoints();
    expect(points.map((p) => p.addressId)).toEqual(['ok']);
  });

  test('passes through the truncated flag', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValueOnce({ ok: true, json: async () => ({ points: [], truncated: true }) }),
    );
    expect(await getMapPoints()).toEqual({ points: [], truncated: true });
  });

  test('throws the parsed error on failure', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(getMapPoints()).rejects.toThrow('nope');
  });
});
