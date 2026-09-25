import { afterEach, describe, expect, test, vi } from 'vitest';
import {
  browserSupportsPush,
  requestPushPermission,
  subscribeBrowserPush,
  urlBase64ToUint8Array,
} from './pushSubscription';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('urlBase64ToUint8Array', () => {
  test('decodes a base64url VAPID key into bytes', () => {
    // "hello" base64url-encoded, no padding.
    const encoded = 'aGVsbG8';
    const bytes = urlBase64ToUint8Array(encoded);

    expect(Array.from(bytes)).toEqual([104, 101, 108, 108, 111]);
  });

  test('translates base64url "-" and "_" back to standard base64 alphabet', () => {
    // Byte sequence [251, 255] base64-encodes to "+/8=" standard, "-_8" url-safe.
    const standard = urlBase64ToUint8Array('+/8=');
    const urlSafe = urlBase64ToUint8Array('-_8');

    expect(Array.from(urlSafe)).toEqual(Array.from(standard));
  });
});

describe('browserSupportsPush', () => {
  test('true when both serviceWorker and PushManager are available', () => {
    vi.stubGlobal('navigator', { serviceWorker: {} });
    vi.stubGlobal('window', { PushManager: function PushManager() {} });

    expect(browserSupportsPush()).toBe(true);
  });

  test('false when serviceWorker is missing', () => {
    vi.stubGlobal('navigator', {});
    vi.stubGlobal('window', { PushManager: function PushManager() {} });

    expect(browserSupportsPush()).toBe(false);
  });

  test('false when PushManager is missing', () => {
    vi.stubGlobal('navigator', { serviceWorker: {} });
    vi.stubGlobal('window', {});

    expect(browserSupportsPush()).toBe(false);
  });
});

describe('requestPushPermission', () => {
  test('false when Notification is not supported at all', async () => {
    vi.stubGlobal('window', {});

    await expect(requestPushPermission()).resolves.toBe(false);
  });

  test('true immediately when permission is already granted', async () => {
    vi.stubGlobal('Notification', { permission: 'granted', requestPermission: vi.fn() });
    vi.stubGlobal('window', { Notification: { permission: 'granted' } });

    await expect(requestPushPermission()).resolves.toBe(true);
  });

  test('false immediately when permission was already denied, without prompting again', async () => {
    const requestPermission = vi.fn();
    vi.stubGlobal('Notification', { permission: 'denied', requestPermission });
    vi.stubGlobal('window', { Notification: { permission: 'denied' } });

    await expect(requestPushPermission()).resolves.toBe(false);
    expect(requestPermission).not.toHaveBeenCalled();
  });

  test('prompts and resolves true when the user grants the request', async () => {
    const requestPermission = vi.fn().mockResolvedValue('granted');
    vi.stubGlobal('Notification', { permission: 'default', requestPermission });
    vi.stubGlobal('window', { Notification: { permission: 'default' } });

    await expect(requestPushPermission()).resolves.toBe(true);
    expect(requestPermission).toHaveBeenCalled();
  });

  test('prompts and resolves false when the user dismisses/denies the request', async () => {
    const requestPermission = vi.fn().mockResolvedValue('denied');
    vi.stubGlobal('Notification', { permission: 'default', requestPermission });
    vi.stubGlobal('window', { Notification: { permission: 'default' } });

    await expect(requestPushPermission()).resolves.toBe(false);
  });
});

describe('subscribeBrowserPush', () => {
  function stubSubscribe(overrides: { toJSON?: () => unknown } = {}) {
    return vi.fn().mockResolvedValue({
      endpoint: 'https://push.example.com/abc123',
      toJSON:
        overrides.toJSON ?? (() => ({ keys: { p256dh: 'p256dh-value', auth: 'auth-value' } })),
    });
  }

  function stubBrowser(opts: {
    userAgent?: string;
    existingRegistration?: unknown;
    register?: ReturnType<typeof vi.fn>;
    subscribe?: ReturnType<typeof vi.fn>;
    notificationPermission?: string;
    requestPermission?: ReturnType<typeof vi.fn>;
  }) {
    const subscribe = opts.subscribe ?? stubSubscribe();
    const register = opts.register ?? vi.fn().mockResolvedValue({ pushManager: { subscribe } });
    const getRegistration = vi
      .fn()
      .mockResolvedValue(
        opts.existingRegistration === undefined ? null : opts.existingRegistration,
      );

    vi.stubGlobal('navigator', {
      serviceWorker: { getRegistration, register },
      userAgent: opts.userAgent ?? 'SomeBrowser/1.0',
    });
    const atob = (s: string) => Buffer.from(s, 'base64').toString('binary');
    vi.stubGlobal('window', {
      PushManager: function PushManager() {},
      Notification: {
        permission: opts.notificationPermission ?? 'granted',
        requestPermission: opts.requestPermission ?? vi.fn(),
      },
      atob,
    });
    vi.stubGlobal('Notification', {
      permission: opts.notificationPermission ?? 'granted',
      requestPermission: opts.requestPermission ?? vi.fn(),
    });
    vi.stubGlobal('atob', atob);

    return { subscribe, register, getRegistration };
  }

  test('throws when the browser does not support push at all', async () => {
    vi.stubGlobal('navigator', {});
    vi.stubGlobal('window', {});

    await expect(subscribeBrowserPush('vapidkey')).rejects.toThrow('push unsupported');
  });

  test('throws when the user denies notification permission', async () => {
    stubBrowser({ notificationPermission: 'denied' });

    await expect(subscribeBrowserPush('vapidkey')).rejects.toThrow('push denied');
  });

  test('reuses an existing service worker registration when one is present', async () => {
    const subscribe = stubSubscribe();
    const existingRegistration = { pushManager: { subscribe } };
    const { register } = stubBrowser({ existingRegistration, subscribe });

    await subscribeBrowserPush('vapidkey');

    expect(register).not.toHaveBeenCalled();
    expect(subscribe).toHaveBeenCalledWith({
      userVisibleOnly: true,
      applicationServerKey: expect.any(Uint8Array),
    });
  });

  test('registers a service worker as a fallback when none exists yet', async () => {
    const subscribe = stubSubscribe();
    const register = vi.fn().mockResolvedValue({ pushManager: { subscribe } });
    stubBrowser({ existingRegistration: null, register, subscribe });

    await subscribeBrowserPush('vapidkey');

    expect(register).toHaveBeenCalledWith('/service-worker.js');
  });

  test('returns the subscription in the API wire shape, with fallback empty keys', async () => {
    const subscribe = stubSubscribe({ toJSON: () => ({}) });
    stubBrowser({ subscribe });

    const result = await subscribeBrowserPush('vapidkey');

    expect(result).toEqual({
      endpoint: 'https://push.example.com/abc123',
      p256dh: '',
      auth: '',
      device_label: 'Browser',
    });
  });

  test('returns the real p256dh/auth keys when present', async () => {
    stubBrowser({});

    const result = await subscribeBrowserPush('vapidkey');

    expect(result.p256dh).toBe('p256dh-value');
    expect(result.auth).toBe('auth-value');
  });

  describe('device label detection', () => {
    test.each([
      ['Mozilla/5.0 (X11; Linux x86_64; rv:120.0) Gecko/20100101 Firefox/120.0', 'Firefox'],
      [
        'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
        'Chrome',
      ],
      [
        'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0',
        'Edge',
      ],
      [
        'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15',
        'Safari',
      ],
      [
        'Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36',
        'Chrome',
      ],
      ['Mozilla/5.0 (Linux; Android 14) SomeEngine/1.0 Mobile', 'Mobile'],
      ['Some Completely Unknown Agent/1.0', 'Browser'],
    ])('userAgent %s -> %s', async (userAgent, expectedLabel) => {
      stubBrowser({ userAgent });

      const result = await subscribeBrowserPush('vapidkey');

      expect(result.device_label).toBe(expectedLabel);
    });
  });
});
