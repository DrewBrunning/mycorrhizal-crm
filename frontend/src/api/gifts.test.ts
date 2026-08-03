import { describe, test, expect, vi, afterEach } from 'vitest';
import {
  getGifts,
  createGift,
  updateGift,
  deleteGift,
} from './gifts';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('getGifts', () => {
  test('requests the entity-scoped endpoint and parses the response', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        gifts: [
          { id: 'g1', entity_id: 'alice-uid', description: 'She liked the ceramics shop', status: 'idea' },
        ],
        next_cursor: '',
        limit: 100,
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const response = await getGifts({ entityId: 'alice-uid' });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/gifts?');
    expect(url).toContain('entity_id=alice-uid');
    expect(response.gifts[0].description).toBe('She liked the ceramics shop');
  });
});

describe('createGift', () => {
  test('POSTs the input and unwraps the created gift', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        message: 'Gift created successfully',
        gift: { id: 'g1', entity_id: 'alice-uid', description: 'A candle', status: 'idea' },
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await createGift({ entity_id: 'alice-uid', description: 'A candle' });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/gifts');
    expect(init.method).toBe('POST');
    expect(result.id).toBe('g1');
  });
});

describe('updateGift', () => {
  test('PUTs a full-replace of the editable fields', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        id: 'g1',
        entity_id: 'alice-uid',
        description: 'The espresso machine',
        status: 'given',
        value_cents: 12000,
        currency: 'EUR',
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await updateGift('g1', {
      entity_id: 'alice-uid',
      description: 'The espresso machine',
      status: 'given',
      value_cents: 12000,
      currency: 'EUR',
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/gifts/g1');
    expect(init.method).toBe('PUT');
    expect(JSON.parse(init.body).currency).toBe('EUR');
    expect(result.status).toBe('given');
  });
});

describe('deleteGift', () => {
  test('DELETEs the gift', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, json: async () => ({}) });
    vi.stubGlobal('fetch', fetchMock);

    await deleteGift('g1');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/gifts/g1');
    expect(init.method).toBe('DELETE');
  });
});
