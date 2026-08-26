import { afterEach, describe, expect, test, vi } from 'vitest';
import {
  createExternalActivity,
  createExternalIdentity,
  deleteExternalActivity,
  deleteExternalIdentity,
  type ExternalActivity,
  type ExternalActivityInput,
  type ExternalIdentity,
  type ExternalIdentityInput,
  getExternalActivities,
  getExternalIdentities,
  updateExternalIdentity,
} from './externalLinks';

afterEach(() => {
  vi.unstubAllGlobals();
});

const identity: ExternalIdentity = {
  id: 'ident-1',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  entity_id: 'alice-uid',
  system: 'github',
  external_id: 'alice-dev',
  url: 'https://github.com/alice-dev',
  metadata: { org: 'acme' },
  sync_status: 'synced',
  last_synced_at: '2026-08-01T00:00:00Z',
};

const activity: ExternalActivity = {
  id: 'act-1',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  entity_id: 'alice-uid',
  source_system: 'github',
  external_id: 'commit-1',
  type: 'push',
  occurred_at: '2026-08-01T00:00:00Z',
  payload: { ref: 'main' },
  provenance: 'external',
  sync_state: 'synced',
};

const errorResponse = () => ({
  ok: false,
  status: 400,
  statusText: 'Bad Request',
  json: async () => ({
    error: { code: 'VALIDATION_ERROR', message: 'nope', details: { system: 'Required' } },
    request_id: 'req-1',
  }),
});

describe('getExternalIdentities', () => {
  test('GETs the identities URL with contact_id, system, cursor, and limit', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        external_identities: [identity],
        total: 1,
        next_cursor: 'CURSOR-1',
        limit: 100,
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await getExternalIdentities({
      contactId: 'alice-uid',
      system: 'github',
      cursor: 'PREV',
      limit: 100,
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/external-identities');
    expect(url).toContain('contact_id=alice-uid');
    expect(url).toContain('system=github');
    expect(url).toContain('cursor=PREV');
    expect(url).toContain('limit=100');
    expect(init.method).toBeUndefined();
    expect(result.external_identities).toEqual([identity]);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(getExternalIdentities()).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('createExternalIdentity', () => {
  test('POSTs the input payload and unwraps external_identity from the response', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ external_identity: identity }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const input: ExternalIdentityInput = {
      entity_id: 'alice-uid',
      system: 'github',
      external_id: 'alice-dev',
      url: 'https://github.com/alice-dev',
      metadata: { org: 'acme' },
    };
    const result = await createExternalIdentity(input);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/external-identities');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual(input);
    expect(result).toEqual(identity);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(
      createExternalIdentity({ entity_id: 'u', system: '', external_id: '' }),
    ).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

describe('updateExternalIdentity', () => {
  test('PUTs the input payload and returns the updated identity', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, json: async () => identity });
    vi.stubGlobal('fetch', fetchMock);

    const input: ExternalIdentityInput = {
      entity_id: 'alice-uid',
      system: 'github',
      external_id: 'alice-dev',
      sync_status: 'synced',
    };
    const result = await updateExternalIdentity('ident-1', input);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/external-identities/ident-1');
    expect(init.method).toBe('PUT');
    expect(JSON.parse(init.body)).toEqual(input);
    expect(result).toEqual(identity);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(
      updateExternalIdentity('ident-1', { entity_id: 'u', system: 's', external_id: 'e' }),
    ).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

describe('deleteExternalIdentity', () => {
  test('DELETEs the identity URL', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, status: 204 });
    vi.stubGlobal('fetch', fetchMock);

    await deleteExternalIdentity('ident-1');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/external-identities/ident-1');
    expect(init.method).toBe('DELETE');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(deleteExternalIdentity('ident-1')).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('getExternalActivities', () => {
  test('GETs the activities URL with contact_id, system, cursor, and limit', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        external_activities: [activity],
        total: 1,
        next_cursor: 'CURSOR-1',
        limit: 50,
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await getExternalActivities({
      contactId: 'alice-uid',
      system: 'github',
      cursor: 'PREV',
      limit: 50,
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/external-activities');
    expect(url).toContain('contact_id=alice-uid');
    expect(url).toContain('system=github');
    expect(url).toContain('cursor=PREV');
    expect(url).toContain('limit=50');
    expect(init.method).toBeUndefined();
    expect(result.external_activities).toEqual([activity]);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(getExternalActivities()).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('createExternalActivity', () => {
  test('POSTs the input payload and unwraps external_activity from the response', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ external_activity: activity }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const input: ExternalActivityInput = {
      entity_id: 'alice-uid',
      source_system: 'github',
      external_id: 'commit-1',
      type: 'push',
      occurred_at: '2026-08-01T00:00:00Z',
      payload: { ref: 'main' },
      provenance: 'external',
      sync_state: 'synced',
    };
    const result = await createExternalActivity(input);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/external-activities');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual(input);
    expect(result).toEqual(activity);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(
      createExternalActivity({
        entity_id: 'u',
        source_system: '',
        external_id: '',
        type: '',
        occurred_at: '',
      }),
    ).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

describe('deleteExternalActivity', () => {
  test('DELETEs the activity URL', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, status: 204 });
    vi.stubGlobal('fetch', fetchMock);

    await deleteExternalActivity('act-1');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/external-activities/act-1');
    expect(init.method).toBe('DELETE');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(deleteExternalActivity('act-1')).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});
