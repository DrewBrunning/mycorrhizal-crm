import { afterEach, describe, expect, test, vi } from 'vitest';
import type { User } from '../types';
import {
  createUser,
  deleteUser,
  getCurrentUser,
  getUsers,
  triggerReminders,
  updateUser,
} from './admin';

afterEach(() => {
  vi.unstubAllGlobals();
});

const user: User = {
  id: 1,
  email: 'admin@example.com',
  username: 'admin',
  language: 'en',
  is_admin: true,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  enabled_contact_fields: ['email', 'phone'],
  self_contact_vcard_uid: 'me-uid',
};

const errorResponse = () => ({
  ok: false,
  status: 400,
  statusText: 'Bad Request',
  json: async () => ({
    error: { code: 'VALIDATION_ERROR', message: 'nope', details: { email: 'Invalid' } },
    request_id: 'req-1',
  }),
});

describe('getCurrentUser', () => {
  test('GETs /users/me and returns the current user', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => user,
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await getCurrentUser();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/users/me');
    expect(init.method).toBe('GET');
    expect(result).toEqual(user);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(getCurrentUser()).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

describe('getUsers', () => {
  test('GETs the admin list with page and limit query params', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        users: [user],
        total: 1,
        page: 2,
        limit: 10,
        total_pages: 1,
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await getUsers(2, 10);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/admin/users');
    expect(url).toContain('page=2');
    expect(url).toContain('limit=10');
    expect(init.method).toBe('GET');
    expect(result.users).toEqual([user]);
    expect(result.total_pages).toBe(1);
  });

  test('uses the default page and limit when not passed', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ users: [], total: 0, page: 1, limit: 25, total_pages: 0 }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await getUsers();

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('page=1');
    expect(url).toContain('limit=25');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(getUsers()).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

describe('createUser', () => {
  test('POSTs the user payload and returns the created user', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => user,
    });
    vi.stubGlobal('fetch', fetchMock);

    const payload = {
      username: 'admin',
      email: 'admin@example.com',
      password: 'secret',
      is_admin: true,
    };
    const result = await createUser(payload);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/admin/users');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual(payload);
    expect(result).toEqual(user);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(
      createUser({ username: 'a', email: 'a@b.com', password: 'x' }),
    ).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

describe('updateUser', () => {
  test('PATCHes the user payload and returns the updated user', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => user,
    });
    vi.stubGlobal('fetch', fetchMock);

    const payload = { email: 'new@example.com' };
    const result = await updateUser(1, payload);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/admin/users/1');
    expect(init.method).toBe('PATCH');
    expect(JSON.parse(init.body)).toEqual(payload);
    expect(result).toEqual(user);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(updateUser(1, { is_admin: false })).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('triggerReminders', () => {
  test('POSTs to the trigger-reminders endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, status: 200 });
    vi.stubGlobal('fetch', fetchMock);

    await triggerReminders();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/admin/trigger-reminders');
    expect(init.method).toBe('POST');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(triggerReminders()).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('deleteUser', () => {
  test('DELETEs the admin user URL', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, status: 204 });
    vi.stubGlobal('fetch', fetchMock);

    await deleteUser(1);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/admin/users/1');
    expect(init.method).toBe('DELETE');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(deleteUser(1)).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});
