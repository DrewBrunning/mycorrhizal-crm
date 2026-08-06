import { describe, test, expect, vi, afterEach } from 'vitest';
import {
  getNotificationConfig,
  saveNotificationConfig,
  testNotificationChannel,
  getPushSubscriptions,
  createPushSubscription,
  deletePushSubscription,
} from './notifications';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('getNotificationConfig', () => {
  test('requests the config endpoint and parses the response', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        ntfy_url: 'https://ntfy.example.com',
        ntfy_topic: 'alerts',
        gotify_url: '',
        gotify_has_token: false,
        notify_ntfy: true,
        notify_gotify: false,
        notify_push: false,
        vapid_public_key: 'pk',
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const config = await getNotificationConfig();

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/notifications/config');
    expect(config.ntfy_url).toBe('https://ntfy.example.com');
    expect(config.notify_ntfy).toBe(true);
    expect(config.vapid_public_key).toBe('pk');
  });
});

describe('saveNotificationConfig', () => {
  test('PUTs the input and returns the saved config', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ ntfy_url: 'https://ntfy.example.com', ntfy_topic: 'alerts', gotify_url: '', gotify_has_token: true, notify_ntfy: true, notify_gotify: false, notify_push: false, vapid_public_key: 'pk' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await saveNotificationConfig({ ntfy_url: 'https://ntfy.example.com', ntfy_topic: 'alerts', gotify_token: 'tok', notify_ntfy: true });

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(init.method).toBe('PUT');
    expect(JSON.parse(String(init.body))).toMatchObject({ ntfy_url: 'https://ntfy.example.com', gotify_token: 'tok', notify_ntfy: true });
  });
});

describe('testNotificationChannel', () => {
  test('POSTs the channel and returns the result', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ ok: true }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await testNotificationChannel('ntfy');

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toContain('/notifications/config/test');
    expect(init.method).toBe('POST');
    expect(JSON.parse(String(init.body))).toEqual({ channel: 'ntfy' });
    expect(result.ok).toBe(true);
  });
});

describe('getPushSubscriptions', () => {
  test('unwraps the subscriptions array', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        subscriptions: [{ id: 1, endpoint: 'https://push.example.com/x', p256dh: 'k', auth: 'a', device_label: 'Chrome', created_at: '2026-01-01T00:00:00Z' }],
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const subs = await getPushSubscriptions();

    expect(fetchMock.mock.calls[0][0]).toContain('/notifications/push-subscriptions');
    expect(subs).toHaveLength(1);
    expect(subs[0].device_label).toBe('Chrome');
  });

  test('returns an empty list when no subscriptions key is present', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({}),
    });
    vi.stubGlobal('fetch', fetchMock);

    const subs = await getPushSubscriptions();
    expect(subs).toEqual([]);
  });
});

describe('createPushSubscription', () => {
  test('POSTs the subscription input', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ id: 5, endpoint: 'https://push.example.com/x', p256dh: 'k', auth: 'a', device_label: 'Chrome', created_at: '2026-01-01T00:00:00Z' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const created = await createPushSubscription({ endpoint: 'https://push.example.com/x', p256dh: 'k', auth: 'a', device_label: 'Chrome' });

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(init.method).toBe('POST');
    expect(JSON.parse(String(init.body))).toMatchObject({ endpoint: 'https://push.example.com/x', auth: 'a' });
    expect(created.id).toBe(5);
  });
});

describe('deletePushSubscription', () => {
  test('DELETEs the subscription by id', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, json: async () => ({}) });
    vi.stubGlobal('fetch', fetchMock);

    await deletePushSubscription(7);

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toContain('/notifications/push-subscriptions/7');
    expect(init.method).toBe('DELETE');
  });
});
