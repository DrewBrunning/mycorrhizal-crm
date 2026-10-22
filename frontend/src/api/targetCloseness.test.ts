import { afterEach, describe, expect, test, vi } from 'vitest';
import { ApiError } from './client';
import {
  type ContactTargetClosenessResponse,
  clearTargetCloseness,
  setTargetCloseness,
} from './targetCloseness';

afterEach(() => {
  vi.unstubAllGlobals();
});

const response: ContactTargetClosenessResponse = {
  contact_id: 7,
  target_closeness: 'close',
  default_interval_days: 14,
  cadence_policy_offer: {
    entity_id: 'alice-uid',
    target_interval_days: 14,
    qualifying_types: [],
  },
};

describe('setTargetCloseness', () => {
  test('PUTs the tier to the contact target-closeness endpoint and returns the body', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => response,
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await setTargetCloseness(7, 'close');

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/contacts/7/target-closeness');
    expect(init.method).toBe('PUT');
    expect(init.headers).toMatchObject({ 'Content-Type': 'application/json' });
    expect(JSON.parse(init.body)).toEqual({ tier: 'close' });
    expect(result).toEqual(response);
  });

  test('accepts a string contact id', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => response,
    });
    vi.stubGlobal('fetch', fetchMock);

    await setTargetCloseness('7', 'immediate');

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/contacts/7/target-closeness');
  });

  test('throws a parsed ApiError (with its code) on a non-ok response', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: false,
      status: 400,
      json: async () => ({
        error: { code: 'INVALID_INPUT', message: 'Invalid target closeness' },
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const error = await setTargetCloseness(7, 'close').catch((err) => err);

    expect(error).toBeInstanceOf(ApiError);
    expect(error.code).toBe('INVALID_INPUT');
    expect(error.status).toBe(400);
    expect(error.message).toBe('Invalid target closeness');
  });
});

describe('clearTargetCloseness', () => {
  test('DELETEs the contact target-closeness endpoint and returns the cleared body', async () => {
    const cleared: ContactTargetClosenessResponse = {
      contact_id: 7,
      target_closeness: null,
      default_interval_days: null,
      cadence_policy_offer: null,
    };
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => cleared,
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await clearTargetCloseness(7);

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/contacts/7/target-closeness');
    expect(init.method).toBe('DELETE');
    expect(init.body).toBeUndefined();
    expect(result).toEqual(cleared);
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

    await expect(clearTargetCloseness(999)).rejects.toMatchObject({
      code: 'NOT_FOUND',
      status: 404,
    });
  });
});
