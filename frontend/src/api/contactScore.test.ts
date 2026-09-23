import { afterEach, describe, expect, test, vi } from 'vitest';
import { ApiError } from './client';
import { type ContactScoreResponse, getContactScore } from './contactScore';

afterEach(() => {
  vi.unstubAllGlobals();
});

const score: ContactScoreResponse = {
  contact_id: 1,
  score: 72,
  band: 'moss',
  recency: { value: 80, weight: 35, reason: 'Last qualifying interaction 5 day(s) ago' },
  frequency: { value: 60, weight: 20, reason: 'Roughly on pace' },
  closeness: { value: 85, weight: 20, reason: 'Marked as close' },
  reach_out: { value: 100, weight: 15, reason: 'No reach-out overdue' },
  last_updated: { value: 90, weight: 10, reason: 'Profile recently updated' },
};

describe('getContactScore', () => {
  test('fetches the score for the given contact id', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => score,
    });
    vi.stubGlobal('fetch', fetchMock);

    const response = await getContactScore(1);

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/contacts/1/score');
    expect(init.method).toBeUndefined();
    expect(response).toEqual(score);
  });

  test('accepts a string contact id', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => score,
    });
    vi.stubGlobal('fetch', fetchMock);

    await getContactScore('1');

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/contacts/1/score');
  });

  test('throws a parsed ApiError on a non-ok response', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: false,
      status: 404,
      json: async () => ({
        error: { code: 'NOT_FOUND', message: 'Contact not found' },
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(getContactScore(999)).rejects.toBeInstanceOf(ApiError);
  });
});
