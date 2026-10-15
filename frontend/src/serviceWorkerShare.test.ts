import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { makeFakeCaches } from './test/fakeCaches';
import { takeSharedDraft } from './utils/shareTarget';

// Issue #1294: drives service-worker.ts's share-target fetch listener against a
// stubbed worker global with the workbox modules mocked out.
vi.mock('workbox-core', () => ({ clientsClaim: vi.fn() }));
vi.mock('workbox-expiration', () => ({ ExpirationPlugin: vi.fn() }));
vi.mock('workbox-precaching', () => ({
  createHandlerBoundToURL: vi.fn(),
  precacheAndRoute: vi.fn(),
}));
vi.mock('workbox-routing', () => ({ registerRoute: vi.fn() }));
vi.mock('workbox-strategies', () => ({ StaleWhileRevalidate: vi.fn() }));

const ORIGIN = window.location.origin;
type Listener = (event: unknown) => void;
let listeners: Record<string, Listener>;

beforeEach(async () => {
  listeners = {};
  const fakeCaches = makeFakeCaches();
  vi.stubGlobal('caches', fakeCaches);
  vi.stubGlobal('self', {
    __WB_MANIFEST: [],
    location: { origin: ORIGIN },
    caches: fakeCaches,
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

function dispatch(method: string, url: string, form?: Record<string, string>) {
  const body = new FormData();
  for (const [k, v] of Object.entries(form ?? {})) body.set(k, v);
  const request = {
    method,
    url,
    formData: async () => body,
  } as unknown as Request;
  let responded: Promise<Response> | undefined;
  listeners.fetch({
    request,
    respondWith: (p: Promise<Response>) => {
      responded = p;
    },
  });
  return responded;
}

describe('service worker share target', () => {
  it('stores the sanitised share and redirects with a single-use key', async () => {
    const res = await dispatch('POST', `${ORIGIN}/share`, {
      title: 'Hi',
      text: 'a\u0000b',
      url: 'https://x.test',
    });
    expect(res?.status).toBe(303);
    const location = new URL(res?.headers.get('Location') ?? '');
    expect(location.pathname).toBe('/share');
    const key = location.searchParams.get('key');
    expect(location.href).not.toContain('Hi');
    expect(await takeSharedDraft(key)).toBe('Hi\n\nab\n\nhttps://x.test');
    expect(await takeSharedDraft(key)).toBeNull();
  });

  it('redirects without a key when nothing usable was shared', async () => {
    const res = await dispatch('POST', `${ORIGIN}/share`, { text: '​' });
    expect(res?.status).toBe(303);
    expect(new URL(res?.headers.get('Location') ?? '').search).toBe('');
  });

  it('redirects without a key when the form cannot be read', async () => {
    let responded: Promise<Response> | undefined;
    listeners.fetch({
      request: {
        method: 'POST',
        url: `${ORIGIN}/share`,
        formData: async () => {
          throw new Error('bad body');
        },
      },
      respondWith: (p: Promise<Response>) => {
        responded = p;
      },
    });
    const res = await responded;
    expect(new URL(res?.headers.get('Location') ?? '').search).toBe('');
  });

  it('ignores GETs, other paths and other origins', () => {
    expect(dispatch('GET', `${ORIGIN}/share`)).toBeUndefined();
    expect(dispatch('POST', `${ORIGIN}/api/v1/notes`)).toBeUndefined();
    expect(dispatch('POST', 'https://evil.test/share')).toBeUndefined();
  });
});
