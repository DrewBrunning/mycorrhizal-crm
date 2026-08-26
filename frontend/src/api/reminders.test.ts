import { afterEach, describe, expect, test, vi } from 'vitest';
import {
  completeReminder,
  createReminder,
  deleteCompletion,
  deleteReminder,
  getCompletionsForContact,
  getRemindersForContact,
  getUpcomingReminders,
  type Reminder,
  type ReminderCompletion,
  type ReminderFormData,
  skipReminder,
  updateReminder,
} from './reminders';

afterEach(() => {
  vi.unstubAllGlobals();
});

const reminder: Reminder = {
  ID: 1,
  message: 'Call Alice',
  by_mail: false,
  remind_at: '2026-08-01T10:00:00Z',
  recurrence: 'weekly',
  reoccur_from_completion: false,
  completed: false,
  email_sent: false,
  contact_id: 5,
  CreatedAt: '2026-07-01T00:00:00Z',
  UpdatedAt: '2026-07-01T00:00:00Z',
};

const completion: ReminderCompletion = {
  ID: 2,
  reminder_id: 1,
  contact_id: 5,
  message: 'Call Alice',
  completed_at: '2026-08-01T10:05:00Z',
  CreatedAt: '2026-08-01T10:05:00Z',
  UpdatedAt: '2026-08-01T10:05:00Z',
};

const errorResponse = () => ({
  ok: false,
  status: 400,
  statusText: 'Bad Request',
  json: async () => ({
    error: { code: 'VALIDATION_ERROR', message: 'nope', details: { message: 'Required' } },
    request_id: 'req-1',
  }),
});

describe('getUpcomingReminders', () => {
  test('GETs the upcoming URL and unwraps the reminders array', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ reminders: [reminder] }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await getUpcomingReminders();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/reminders/upcoming');
    expect(init.method).toBeUndefined();
    expect(result).toEqual([reminder]);
  });

  test('returns an empty array when the response has no reminders key', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce({ ok: true, json: async () => ({}) }));
    const result = await getUpcomingReminders();
    expect(result).toEqual([]);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(getUpcomingReminders()).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('getRemindersForContact', () => {
  test('GETs the contact reminders URL and unwraps the reminders array', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ reminders: [reminder] }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await getRemindersForContact(5);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/contacts/5/reminders');
    expect(init.method).toBeUndefined();
    expect(result).toEqual([reminder]);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(getRemindersForContact(5)).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('createReminder', () => {
  test('POSTs the reminder payload and unwraps the reminder from the response', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ reminder }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const formData: ReminderFormData = {
      message: 'Call Alice',
      by_mail: false,
      remind_at: '2026-08-01T10:00:00Z',
      recurrence: 'weekly',
      reoccur_from_completion: false,
      contact_id: 5,
    };
    const result = await createReminder(5, formData);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/contacts/5/reminders');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual(formData);
    expect(result).toEqual(reminder);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(
      createReminder(5, {
        message: '',
        by_mail: false,
        remind_at: '',
        recurrence: 'once',
        reoccur_from_completion: false,
        contact_id: 5,
      }),
    ).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

describe('updateReminder', () => {
  test('PUTs the reminder payload and unwraps the reminder from the response', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ reminder }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const payload = { message: 'Call Bob' };
    const result = await updateReminder(1, payload);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/reminders/1');
    expect(init.method).toBe('PUT');
    expect(JSON.parse(init.body)).toEqual(payload);
    expect(result).toEqual(reminder);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(updateReminder(1, { message: 'x' })).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('completeReminder', () => {
  test('POSTs to the complete URL and returns the response', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ message: 'Reminder completed', reminder }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await completeReminder(1);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/reminders/1/complete');
    expect(init.method).toBe('POST');
    expect(result.message).toBe('Reminder completed');
    expect(result.reminder).toEqual(reminder);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(completeReminder(1)).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('skipReminder', () => {
  test('POSTs to the complete URL with skip=true', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ message: 'Reminder skipped', reminder }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await skipReminder(1);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/reminders/1/complete');
    expect(url).toContain('skip=true');
    expect(init.method).toBe('POST');
    expect(result.message).toBe('Reminder skipped');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(skipReminder(1)).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

describe('deleteReminder', () => {
  test('DELETEs the reminder URL', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, status: 204 });
    vi.stubGlobal('fetch', fetchMock);

    await deleteReminder(1);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/reminders/1');
    expect(init.method).toBe('DELETE');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(deleteReminder(1)).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('getCompletionsForContact', () => {
  test('GETs the completions URL and unwraps the completions array', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ completions: [completion] }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await getCompletionsForContact(5);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/contacts/5/reminder-completions');
    expect(init.method).toBeUndefined();
    expect(result).toEqual([completion]);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(getCompletionsForContact(5)).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('deleteCompletion', () => {
  test('DELETEs the completion URL', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, status: 204 });
    vi.stubGlobal('fetch', fetchMock);

    await deleteCompletion(2);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/reminder-completions/2');
    expect(init.method).toBe('DELETE');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(deleteCompletion(2)).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});
