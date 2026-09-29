import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// Issue #1270: drives service-worker.ts's push / notificationclick listeners
// against a stubbed worker global with the workbox modules mocked out.
vi.mock('workbox-core', () => ({ clientsClaim: vi.fn() }));
vi.mock('workbox-expiration', () => ({ ExpirationPlugin: vi.fn() }));
vi.mock('workbox-precaching', () => ({
  createHandlerBoundToURL: vi.fn(),
  precacheAndRoute: vi.fn(),
}));
vi.mock('workbox-routing', () => ({ registerRoute: vi.fn() }));
vi.mock('workbox-strategies', () => ({ StaleWhileRevalidate: vi.fn() }));

const ORIGIN = 'https://crm.example';
type Listener = (event: unknown) => void;

let listeners: Record<string, Listener>;
let showNotification: ReturnType<typeof vi.fn>;
let matchAll: ReturnType<typeof vi.fn>;
let openWindow: ReturnType<typeof vi.fn>;
let skipWaiting: ReturnType<typeof vi.fn>;

beforeEach(async () => {
  listeners = {};
  showNotification = vi.fn().mockResolvedValue(undefined);
  matchAll = vi.fn().mockResolvedValue([]);
  openWindow = vi.fn().mockResolvedValue(null);
  skipWaiting = vi.fn();
  vi.stubGlobal('self', {
    __WB_MANIFEST: [],
    location: { origin: ORIGIN },
    registration: { showNotification },
    clients: { matchAll, openWindow },
    skipWaiting,
    addEventListener: (name: string, fn: Listener) => {
      listeners[name] = fn;
    },
  });
  vi.resetModules();
  await import('./service-worker');
});

afterEach(() => {
  vi.unstubAllGlobals();
});

async function push(data: { json?: unknown; text?: string } | null) {
  let waited: Promise<unknown> = Promise.resolve();
  listeners.push({
    data:
      data === null
        ? null
        : {
            json: () => {
              if (data.json === undefined) throw new Error('not json');
              return data.json;
            },
            text: () => data.text ?? '',
          },
    waitUntil: (p: Promise<unknown>) => {
      waited = p;
    },
  });
  await waited;
}

async function click(path: unknown) {
  let waited: Promise<unknown> = Promise.resolve();
  const close = vi.fn();
  listeners.notificationclick({
    notification: { close, data: path === undefined ? undefined : { path } },
    waitUntil: (p: Promise<unknown>) => {
      waited = p;
    },
  });
  await waited;
  return close;
}

describe('service worker push', () => {
  it('shows the notification with the payload path attached as data', async () => {
    await push({ json: { title: 'Reminder', body: 'Call Ann', path: '/contacts/42' } });
    expect(showNotification).toHaveBeenCalledWith(
      'Reminder',
      expect.objectContaining({ body: 'Call Ann', data: { path: '/contacts/42' } }),
    );
  });

  it('falls back to defaults for a payload with no fields, no data, or plain text', async () => {
    await push({ json: {} });
    expect(showNotification).toHaveBeenLastCalledWith(
      'Mycorrhizal CRM',
      expect.objectContaining({ body: '', data: { path: undefined } }),
    );
    await push(null);
    expect(showNotification).toHaveBeenCalledTimes(2);
    await push({ text: 'plain' });
    expect(showNotification).toHaveBeenLastCalledWith(
      'Mycorrhizal CRM',
      expect.objectContaining({ body: 'plain' }),
    );
  });
});

describe('service worker notificationclick', () => {
  it('closes the notification and opens the contact when no window exists', async () => {
    const close = await click('/contacts/42');
    expect(close).toHaveBeenCalled();
    expect(openWindow).toHaveBeenCalledWith('/contacts/42');
  });

  it('opens / for a missing or disallowed path', async () => {
    await click(undefined);
    await click('https://evil.example/');
    await click('//evil.example');
    expect(openWindow.mock.calls).toEqual([['/'], ['/'], ['/']]);
  });

  it('focuses and navigates an existing same-origin window', async () => {
    const win = {
      url: `${ORIGIN}/dashboard`,
      focus: vi.fn().mockResolvedValue(undefined),
      navigate: vi.fn().mockResolvedValue(undefined),
    };
    matchAll.mockResolvedValue([win]);
    await click('/contacts/7');
    expect(win.focus).toHaveBeenCalled();
    expect(win.navigate).toHaveBeenCalledWith(`${ORIGIN}/contacts/7`);
    expect(openWindow).not.toHaveBeenCalled();
  });

  it('skips a cross-origin or non-window client and opens a new window', async () => {
    const foreign = { url: 'https://evil.example/', focus: vi.fn(), navigate: vi.fn() };
    matchAll.mockResolvedValue([foreign, { url: `${ORIGIN}/x` }]);
    await click('/contacts/7');
    expect(foreign.focus).not.toHaveBeenCalled();
    expect(foreign.navigate).not.toHaveBeenCalled();
    expect(openWindow).toHaveBeenCalledWith('/contacts/7');
  });
});

describe('service worker message', () => {
  it('skips waiting only for a same-origin SKIP_WAITING message', () => {
    listeners.message({ origin: 'https://evil.example', data: { type: 'SKIP_WAITING' } });
    listeners.message({ origin: ORIGIN, data: { type: 'other' } });
    listeners.message({ origin: ORIGIN, data: null });
    expect(skipWaiting).not.toHaveBeenCalled();
    listeners.message({ origin: ORIGIN, data: { type: 'SKIP_WAITING' } });
    expect(skipWaiting).toHaveBeenCalledTimes(1);
  });
});
