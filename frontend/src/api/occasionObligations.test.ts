import { afterEach, describe, expect, test, vi } from 'vitest';
import {
  createOccasionObligation,
  deleteOccasionObligation,
  downloadOccasionCardListCSV,
  getGiftShoppingList,
  getOccasionObligations,
  getUpcomingOccasions,
  updateOccasionObligation,
} from './occasionObligations';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('getOccasionObligations', () => {
  test('requests the entity-scoped endpoint and parses the response', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        occasion_obligations: [
          {
            id: 'o1',
            entity_id: 'alice-uid',
            kind: 'card',
            label: 'Christmas card',
            lead_time_days: 0,
            active: true,
            sensitivity: 'normal',
            created_at: '2026-01-01T00:00:00Z',
            updated_at: '2026-01-01T00:00:00Z',
          },
        ],
        total: 1,
        next_cursor: '',
        limit: 100,
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const response = await getOccasionObligations({ entityId: 'alice-uid' });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/occasion-obligations?');
    expect(url).toContain('entity_id=alice-uid');
    expect(response.total).toBe(1);
    expect(response.occasion_obligations[0].label).toBe('Christmas card');
  });
});

describe('createOccasionObligation', () => {
  test('POSTs the input and unwraps the created obligation', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        message: 'Occasion obligation created successfully',
        occasion_obligation: {
          id: 'o1',
          entity_id: 'alice-uid',
          kind: 'card',
          label: 'Christmas card',
          lead_time_days: 0,
          active: true,
          sensitivity: 'normal',
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await createOccasionObligation({
      entity_id: 'alice-uid',
      kind: 'card',
      label: 'Christmas card',
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/occasion-obligations');
    expect(init.method).toBe('POST');
    expect(result.id).toBe('o1');
    expect(result.label).toBe('Christmas card');
  });
});

describe('updateOccasionObligation', () => {
  test('PUTs to the id-scoped endpoint and returns the raw obligation', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        id: 'o1',
        entity_id: 'alice-uid',
        kind: 'card',
        label: 'Updated card',
        lead_time_days: 7,
        active: false,
        sensitivity: 'normal',
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-02T00:00:00Z',
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await updateOccasionObligation('o1', {
      entity_id: 'alice-uid',
      kind: 'card',
      label: 'Updated card',
      active: false,
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/occasion-obligations/o1');
    expect(init.method).toBe('PUT');
    expect(result.active).toBe(false);
  });
});

describe('deleteOccasionObligation', () => {
  test('DELETEs the id-scoped endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, json: async () => ({}) });
    vi.stubGlobal('fetch', fetchMock);

    await deleteOccasionObligation('o1');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/occasion-obligations/o1');
    expect(init.method).toBe('DELETE');
  });

  test('throws the parsed error on a failed response', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: false,
      status: 404,
      json: async () => ({ error: { code: 'NOT_FOUND', message: 'not found' } }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(deleteOccasionObligation('missing')).rejects.toThrow();
  });
});

describe('getUpcomingOccasions', () => {
  test('passes days and include_sensitive through as query params', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ occasions: [], days: 90 }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await getUpcomingOccasions({ days: 90, includeSensitive: true });

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/occasions/upcoming?');
    expect(url).toContain('days=90');
    expect(url).toContain('include_sensitive=true');
  });

  test('defaults to a 30-day window with no include_sensitive param', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ occasions: [], days: 30 }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await getUpcomingOccasions();

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('days=30');
    expect(url).not.toContain('include_sensitive');
  });
});

describe('getGiftShoppingList', () => {
  test('requests the gift-shopping-list endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ gift_shopping_list: [], days: 30 }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await getGiftShoppingList({ days: 30 });

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/occasion-obligations/gift-shopping-list?');
  });
});

describe('downloadOccasionCardListCSV', () => {
  test('requests the card-list endpoint with the given kind', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      headers: new Headers({ 'Content-Disposition': 'attachment; filename="cards.csv"' }),
      blob: async () => new Blob(['a,b\n'], { type: 'text/csv' }),
    });
    vi.stubGlobal('fetch', fetchMock);
    // downloadFileFromResponse manipulates the DOM to trigger a download;
    // createObjectURL/revokeObjectURL aren't implemented in jsdom.
    vi.stubGlobal('URL', {
      ...URL,
      createObjectURL: vi.fn(() => 'blob:mock'),
      revokeObjectURL: vi.fn(),
    });

    await downloadOccasionCardListCSV({ kind: 'gift' });

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/occasion-obligations/card-list?');
    expect(url).toContain('kind=gift');
  });
});
