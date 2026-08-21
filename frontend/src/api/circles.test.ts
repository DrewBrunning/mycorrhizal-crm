import { describe, test, expect, vi, afterEach } from 'vitest';
import {
  listCircles,
  createCircle,
  updateCircle,
  deleteCircle,
  addCircleMember,
  removeCircleMember,
  Circle,
  CircleMember,
} from './circles';

afterEach(() => {
  vi.unstubAllGlobals();
});

const circle: Circle = {
  id: 'circle-1',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  name: 'Family',
};

const member: CircleMember = {
  id: 1,
  circle_id: 'circle-1',
  member_vcard_uid: 'alice-uid',
};

const errorResponse = () => ({
  ok: false,
  status: 400,
  statusText: 'Bad Request',
  json: async () => ({
    error: { code: 'VALIDATION_ERROR', message: 'nope', details: { name: 'Required' } },
    request_id: 'req-1',
  }),
});

describe('listCircles', () => {
  test('GETs the circles URL with limit, cursor, and include_members', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        circles: [circle],
        total: 1,
        next_cursor: 'CURSOR-1',
        limit: 100,
        members: [member],
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await listCircles({ cursor: 'PREV', limit: 100, include_members: true });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/circles');
    expect(url).toContain('limit=100');
    expect(url).toContain('cursor=PREV');
    expect(url).toContain('include_members=true');
    expect(init.method).toBeUndefined();
    expect(result.circles).toEqual([circle]);
    expect(result.members).toEqual([member]);
  });

  test('uses the default limit and omits include_members when not requested', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ circles: [], total: 0, next_cursor: '', limit: 100 }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await listCircles();

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('limit=100');
    expect(url).not.toContain('include_members');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(listCircles()).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

describe('createCircle', () => {
  test('POSTs the circle name and returns the created circle', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ message: 'Circle created', circle }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await createCircle('Family');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/circles');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual({ name: 'Family' });
    expect(result.circle).toEqual(circle);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(createCircle('Family')).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

describe('updateCircle', () => {
  test('PUTs the circle name and returns the updated circle', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, json: async () => circle });
    vi.stubGlobal('fetch', fetchMock);

    const result = await updateCircle('circle-1', 'Renamed');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/circles/circle-1');
    expect(init.method).toBe('PUT');
    expect(JSON.parse(init.body)).toEqual({ name: 'Renamed' });
    expect(result).toEqual(circle);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(updateCircle('circle-1', 'x')).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('deleteCircle', () => {
  test('DELETEs the circle URL', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, status: 204 });
    vi.stubGlobal('fetch', fetchMock);

    await deleteCircle('circle-1');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/circles/circle-1');
    expect(init.method).toBe('DELETE');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(deleteCircle('circle-1')).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('addCircleMember', () => {
  test('POSTs the member payload and returns the created membership', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, json: async () => member });
    vi.stubGlobal('fetch', fetchMock);

    const result = await addCircleMember('circle-1', 'alice-uid');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/circles/circle-1/members');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual({ member_vcard_uid: 'alice-uid' });
    expect(result).toEqual(member);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(addCircleMember('circle-1', 'alice-uid')).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('removeCircleMember', () => {
  test('DELETEs the member URL with the vcard uid', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, status: 204 });
    vi.stubGlobal('fetch', fetchMock);

    await removeCircleMember('circle-1', 'alice-uid');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/circles/circle-1/members/alice-uid');
    expect(init.method).toBe('DELETE');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(removeCircleMember('circle-1', 'alice-uid')).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});
