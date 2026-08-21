import { describe, test, expect, vi, afterEach } from 'vitest';
import {
  getActivities,
  getContactActivities,
  getActivity,
  createActivity,
  updateActivity,
  deleteActivity,
  Activity,
} from './activities';

afterEach(() => {
  vi.unstubAllGlobals();
});

const activity: Activity = {
  ID: 1,
  title: 'Coffee',
  description: 'Catch up',
  location: 'Cafe',
  date: '2026-08-01T10:00:00Z',
  CreatedAt: '2026-08-01T09:00:00Z',
  UpdatedAt: '2026-08-01T09:00:00Z',
  contacts: [{ ID: 5, firstname: 'Alice', lastname: 'Anderson', nickname: 'Ali' }],
};

const errorResponse = () => ({
  ok: false,
  status: 400,
  statusText: 'Bad Request',
  json: async () => ({
    error: { code: 'VALIDATION_ERROR', message: 'nope', details: { title: 'Required' } },
    request_id: 'req-1',
  }),
});

describe('getActivities', () => {
  test('GETs the list URL with limit, cursor, include, search, and date filters', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ activities: [activity], next_cursor: 'CURSOR-1', limit: 25 }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await getActivities({
      cursor: 'PREV',
      limit: 25,
      includeContacts: true,
      search: '  coffee  ',
      fromDate: '2026-08-01',
      toDate: '2026-08-31',
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/api/v1/activities');
    expect(url).toContain('limit=25');
    expect(url).toContain('cursor=PREV');
    expect(url).toContain('include=contacts');
    expect(url).toContain('search=coffee');
    expect(url).toContain('fromDate=2026-08-01');
    expect(url).toContain('toDate=2026-08-31');
    expect(result.activities[0]).toEqual(activity);
    expect(result.next_cursor).toBe('CURSOR-1');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(getActivities({})).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

describe('getContactActivities', () => {
  test('GETs the contact activities URL and returns the activities', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ activities: [activity] }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await getContactActivities(5);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/contacts/5/activities');
    expect(init.method).toBeUndefined();
    expect(result.activities).toEqual([activity]);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(getContactActivities(5)).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

describe('getActivity', () => {
  test('GETs the single activity URL and returns the activity', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => activity,
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await getActivity(1);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/activities/1');
    expect(init.method).toBeUndefined();
    expect(result).toEqual(activity);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(getActivity(1)).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

describe('createActivity', () => {
  test('POSTs the activity payload and returns the created activity', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => activity,
    });
    vi.stubGlobal('fetch', fetchMock);

    const payload = {
      title: 'Coffee',
      description: 'Catch up',
      location: 'Cafe',
      date: '2026-08-01T10:00:00Z',
      contact_ids: [5],
    };
    const result = await createActivity(payload);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/activities');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual(payload);
    expect(result).toEqual(activity);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(
      createActivity({ title: 'Coffee', description: '', location: '', date: '', contact_ids: [] })
    ).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

describe('updateActivity', () => {
  test('PUTs the activity payload and returns the updated activity', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => activity,
    });
    vi.stubGlobal('fetch', fetchMock);

    const payload = { title: 'Coffee 2', date: '2026-08-02T10:00:00Z' };
    const result = await updateActivity(1, payload);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/activities/1');
    expect(init.method).toBe('PUT');
    expect(JSON.parse(init.body)).toEqual(payload);
    expect(result).toEqual(activity);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(updateActivity(1, { title: 'x' })).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('deleteActivity', () => {
  test('DELETEs the activity URL', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, status: 204 });
    vi.stubGlobal('fetch', fetchMock);

    await deleteActivity(1);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/activities/1');
    expect(init.method).toBe('DELETE');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(deleteActivity(1)).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});
