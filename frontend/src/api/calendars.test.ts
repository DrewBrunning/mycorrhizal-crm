import { describe, test, expect, vi, afterEach } from 'vitest';
import {
  getCalendarSubscriptions,
  createCalendarSubscription,
  updateCalendarSubscription,
  deleteCalendarSubscription,
  syncCalendarSubscription,
  CalendarSubscription,
  CalendarSubscriptionInput,
} from './calendars';

afterEach(() => {
  vi.unstubAllGlobals();
});

const subscription: CalendarSubscription = {
  id: 1,
  name: 'Work calendar',
  url: 'https://cal.example.com/work.ics',
  username: 'alice',
  has_password: true,
  sync_enabled: true,
  past_days: 30,
  future_days: 90,
  last_synced_at: '2026-08-01T00:00:00Z',
  last_sync_status: 'success',
  last_sync_error: '',
  created_at: '2026-07-01T00:00:00Z',
};

const okResponse = (body: unknown) => ({
  ok: true,
  status: 200,
  text: async () => JSON.stringify(body),
});

const errorResponse = () => ({
  ok: false,
  status: 400,
  statusText: 'Bad Request',
  text: async () => JSON.stringify({ error: { message: 'Invalid URL' } }),
});

describe('getCalendarSubscriptions', () => {
  test('GETs the calendars URL and unwraps the calendars array', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(
      okResponse({ calendars: [subscription] })
    );
    vi.stubGlobal('fetch', fetchMock);

    const result = await getCalendarSubscriptions();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/calendars');
    expect(init.method).toBe('GET');
    expect(result).toEqual([subscription]);
  });

  test('returns an empty array when the response has no calendars key', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(okResponse({})));
    const result = await getCalendarSubscriptions();
    expect(result).toEqual([]);
  });

  test('throws the parsed error message when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(getCalendarSubscriptions()).rejects.toThrow('Invalid URL');
  });
});

describe('createCalendarSubscription', () => {
  test('POSTs the input payload and returns the created subscription', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(subscription));
    vi.stubGlobal('fetch', fetchMock);

    const input: CalendarSubscriptionInput = {
      name: 'Work calendar',
      url: 'https://cal.example.com/work.ics',
      username: 'alice',
      password: 'secret',
      sync_enabled: true,
      past_days: 30,
      future_days: 90,
    };
    const result = await createCalendarSubscription(input);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/calendars');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual(input);
    expect(result).toEqual(subscription);
  });

  test('throws the parsed error message when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(
      createCalendarSubscription({ name: 'x', url: 'not-a-url' })
    ).rejects.toThrow('Invalid URL');
  });
});

describe('updateCalendarSubscription', () => {
  test('PUTs the input payload and returns the updated subscription', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(subscription));
    vi.stubGlobal('fetch', fetchMock);

    const input: CalendarSubscriptionInput = {
      name: 'Renamed',
      url: 'https://cal.example.com/work.ics',
      clear_password: true,
    };
    const result = await updateCalendarSubscription(1, input);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/calendars/1');
    expect(init.method).toBe('PUT');
    expect(JSON.parse(init.body)).toEqual(input);
    expect(result).toEqual(subscription);
  });

  test('throws the parsed error message when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(
      updateCalendarSubscription(1, { name: 'x', url: 'https://cal.example.com/x.ics' })
    ).rejects.toThrow('Invalid URL');
  });
});

describe('deleteCalendarSubscription', () => {
  test('DELETEs the calendar URL', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse({}));
    vi.stubGlobal('fetch', fetchMock);

    await deleteCalendarSubscription(1);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/calendars/1');
    expect(init.method).toBe('DELETE');
  });

  test('throws the parsed error message when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(deleteCalendarSubscription(1)).rejects.toThrow('Invalid URL');
  });
});

describe('syncCalendarSubscription', () => {
  test('POSTs to the sync URL and returns the sync result', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(
      okResponse({ message: 'Synced', created: 2, updated: 1, skipped: 0 })
    );
    vi.stubGlobal('fetch', fetchMock);

    const result = await syncCalendarSubscription(1);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/calendars/1/sync');
    expect(init.method).toBe('POST');
    expect(result.created).toBe(2);
    expect(result.updated).toBe(1);
  });

  test('throws the parsed error message when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(syncCalendarSubscription(1)).rejects.toThrow('Invalid URL');
  });
});
