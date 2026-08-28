import { afterEach, describe, expect, test, vi } from 'vitest';
import { getNotificationChannelHealth, NOTIFICATION_CHANNELS } from './notificationHealth';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('getNotificationChannelHealth', () => {
  test('requests /admin/notification-health and parses the response', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        channels: [
          {
            channel: 'gotify',
            status: 'failing',
            configured: true,
            reachable: false,
            enabled_user_count: 1,
            device_count: 0,
            fcm_configured: false,
            last_attempt_at: '2026-08-27T17:40:00Z',
            last_sent_at: '2026-08-27T17:04:00Z',
            last_failed_at: '2026-08-27T17:40:00Z',
            consecutive_failures: 9,
            last_error: 'HTTP 401',
            attempted_count: 12,
            delivered_count: 3,
          },
        ],
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const response = await getNotificationChannelHealth();

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/admin/notification-health');
    expect(response.channels[0].consecutive_failures).toBe(9);
    expect(response.channels[0].last_error).toBe('HTTP 401');
    expect(response.channels[0].attempted_count).toBe(12);
  });

  test('throws on a non-ok response', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: false,
      status: 403,
      json: async () => ({ error: 'forbidden' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(getNotificationChannelHealth()).rejects.toBeDefined();
  });
});

describe('NOTIFICATION_CHANNELS mirror', () => {
  test('is the frozen backend channel vocabulary, in dispatch order', () => {
    expect(NOTIFICATION_CHANNELS).toEqual(['email', 'ntfy', 'gotify', 'push']);
  });
});
