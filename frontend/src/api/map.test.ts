import { afterEach, describe, expect, test, vi } from 'vitest';

vi.mock('./contacts', () => ({ getAllContacts: vi.fn() }));

import { getAllContacts } from './contacts';
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
  vi.mocked(getAllContacts).mockReset();
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

test('formatGeoUri round-trips through parseGeoUri', () => {
  expect(formatGeoUri(51.5, -0.12)).toBe('geo:51.5,-0.12');
  expect(parseGeoUri(formatGeoUri(51.5, -0.12))).toEqual({ lat: 51.5, lng: -0.12 });
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
  const contacts = [
    { ID: 1, uid: 'u1', firstname: 'Ada', lastname: 'Lovelace' },
    { ID: 2, uid: 'u2', firstname: '', lastname: '', nickname: 'Bobby' },
    { ID: 3, uid: 'u3', firstname: 'No', lastname: 'Coords' },
    { ID: 4, uid: 'u4', firstname: '', lastname: '' },
    { ID: 5, firstname: 'No', lastname: 'Uid' },
  ];

  test('joins export cards to contacts and skips unplottable addresses', async () => {
    vi.mocked(getAllContacts).mockResolvedValueOnce(contacts as never);
    const cards = [
      {
        uid: 'u1',
        addresses: {
          h: {
            coordinates: 'geo:51.5,-0.12',
            components: [
              { kind: 'name', value: '1 Main St' },
              { kind: 'locality', value: 'London' },
              { kind: 'country', value: 'UK' },
            ],
          },
          bad: { coordinates: 'geo:999,0' },
          none: {},
        },
      },
      { uid: 'u2', addresses: { w: { coordinates: 'geo:1,2', full: 'Somewhere full' } } },
      { uid: 'u3' },
      { uid: 'u4', addresses: { n: { coordinates: 'geo:3,4' } } },
      { addresses: { z: { coordinates: 'geo:5,6' } } },
      { uid: 'unknown', addresses: { x: { coordinates: 'geo:1,1' } } },
      { addresses: { y: { coordinates: 'geo:1,1' } } },
    ];
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, json: async () => cards });
    vi.stubGlobal('fetch', fetchMock);

    const points = await getMapPoints();

    expect(points).toEqual([
      {
        contactId: 1,
        contactName: 'Ada Lovelace',
        addressId: 'h',
        label: '1 Main St, London, UK',
        lat: 51.5,
        lng: -0.12,
      },
      {
        contactId: 2,
        contactName: 'Bobby',
        addressId: 'w',
        label: 'Somewhere full',
        lat: 1,
        lng: 2,
      },
      { contactId: 4, contactName: '', addressId: 'n', label: '', lat: 3, lng: 4 },
    ]);
    // The map is the owner's own view: sensitive addresses must be included.
    expect(fetchMock.mock.calls[0][0]).toMatch(/sections=addresses&include_sensitive=true$/);
  });

  test('uses the number component when there is no street name', async () => {
    vi.mocked(getAllContacts).mockResolvedValueOnce(contacts as never);
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValueOnce({
        ok: true,
        json: async () => [
          {
            uid: 'u1',
            addresses: {
              a: { coordinates: 'geo:1,2', components: [{ kind: 'number', value: '12' }] },
            },
          },
        ],
      }),
    );
    expect((await getMapPoints())[0].label).toBe('12');
  });

  test('throws the parsed error when the export fails', async () => {
    vi.mocked(getAllContacts).mockResolvedValueOnce([]);
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(getMapPoints()).rejects.toThrow('nope');
  });
});
