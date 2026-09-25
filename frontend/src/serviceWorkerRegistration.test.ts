import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

// serviceWorkerRegistration.ts computes `isLocalhost` once, at module import
// time, from `window.location.hostname`. To exercise both the localhost and
// non-localhost branches this file resets the module registry and stubs
// `window`/`navigator` *before* each dynamic import, then imports fresh.

type LoadListener = () => void;

function stubWindow(opts: { hostname: string; href: string; origin: string }) {
  const reload = vi.fn();
  let loadListener: LoadListener | undefined;
  vi.stubGlobal('window', {
    location: { hostname: opts.hostname, href: opts.href, origin: opts.origin, reload },
    addEventListener: (event: string, fn: LoadListener) => {
      if (event === 'load') {
        loadListener = fn;
      }
    },
  });
  return {
    reload,
    fireLoad: () => loadListener?.(),
  };
}

function fakeInstallingWorker() {
  return { state: 'installing', onstatechange: null as (() => void) | null };
}

function stubNavigator(overrides: Record<string, unknown> = {}) {
  vi.stubGlobal('navigator', overrides);
}

const originalEnv = { PROD: import.meta.env.PROD, BASE_URL: import.meta.env.BASE_URL };

beforeEach(() => {
  vi.resetModules();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- test-only reset of vite's env
  (import.meta.env as any).PROD = originalEnv.PROD;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- test-only reset of vite's env
  (import.meta.env as any).BASE_URL = originalEnv.BASE_URL;
});

describe('register', () => {
  test('does nothing when not a production build', async () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any -- test-only override of vite's env
    (import.meta.env as any).PROD = false;
    const swRegister = vi.fn();
    stubWindow({
      hostname: 'example.com',
      href: 'https://example.com/',
      origin: 'https://example.com',
    });
    stubNavigator({ serviceWorker: { register: swRegister } });

    const { register } = await import('./serviceWorkerRegistration');
    register();

    expect(swRegister).not.toHaveBeenCalled();
  });

  test('does nothing when the browser has no serviceWorker support', async () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any -- test-only override of vite's env
    (import.meta.env as any).PROD = true;
    const win = stubWindow({
      hostname: 'example.com',
      href: 'https://example.com/',
      origin: 'https://example.com',
    });
    stubNavigator({});

    const { register } = await import('./serviceWorkerRegistration');
    register();
    win.fireLoad();

    // No crash and no reload attempted -- the whole block is skipped.
    expect(win.reload).not.toHaveBeenCalled();
  });

  test('is a no-op when the public URL origin differs from the page origin', async () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any -- test-only override of vite's env
    (import.meta.env as any).PROD = true;
    // A CDN-served BASE_URL: an absolute, different-origin URL rather than
    // the usual relative '/'.
    // eslint-disable-next-line @typescript-eslint/no-explicit-any -- test-only override of vite's env
    (import.meta.env as any).BASE_URL = 'https://cdn.example.net/';
    const swRegister = vi.fn();
    const win = stubWindow({
      hostname: 'example.com',
      href: 'https://example.com/',
      origin: 'https://example.com',
    });
    stubNavigator({ serviceWorker: { register: swRegister } });

    const { register } = await import('./serviceWorkerRegistration');
    register();
    win.fireLoad();
    await Promise.resolve();

    // The function returns before addEventListener('load', ...) is ever
    // reached when origins mismatch, so nothing gets registered.
    expect(swRegister).not.toHaveBeenCalled();
  });

  test('registers the service worker directly on a non-localhost production origin', async () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any -- test-only override of vite's env
    (import.meta.env as any).PROD = true;
    const registration: {
      onupdatefound: (() => void) | null;
      installing: ReturnType<typeof fakeInstallingWorker> | null;
    } = { onupdatefound: null, installing: null };
    const swRegister = vi.fn().mockResolvedValue(registration);
    const win = stubWindow({
      hostname: 'example.com',
      href: 'http://example.com/',
      origin: 'http://example.com',
    });
    stubNavigator({ serviceWorker: { register: swRegister } });

    const { register } = await import('./serviceWorkerRegistration');
    const onSuccess = vi.fn();
    register({ onSuccess });
    win.fireLoad();
    await Promise.resolve();
    await Promise.resolve();

    expect(swRegister).toHaveBeenCalledWith('/service-worker.js');
    expect(registration.onupdatefound).toBeInstanceOf(Function);

    // Simulate the browser installing a fresh worker with no prior
    // controller -- the "everything precached for the first time" path.
    const worker = fakeInstallingWorker();
    registration.installing = worker;
    registration.onupdatefound?.();
    worker.state = 'installed';
    worker.onstatechange?.();

    expect(onSuccess).toHaveBeenCalledWith(registration);
  });

  test('ignores state changes other than "installed"', async () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any -- test-only override of vite's env
    (import.meta.env as any).PROD = true;
    const registration: {
      onupdatefound: (() => void) | null;
      installing: ReturnType<typeof fakeInstallingWorker> | null;
    } = { onupdatefound: null, installing: null };
    const swRegister = vi.fn().mockResolvedValue(registration);
    const win = stubWindow({
      hostname: 'example.com',
      href: 'http://example.com/',
      origin: 'http://example.com',
    });
    stubNavigator({ serviceWorker: { register: swRegister } });

    const { register } = await import('./serviceWorkerRegistration');
    const onSuccess = vi.fn();
    const onUpdate = vi.fn();
    register({ onSuccess, onUpdate });
    win.fireLoad();
    await Promise.resolve();
    await Promise.resolve();

    const worker = fakeInstallingWorker();
    registration.installing = worker;
    registration.onupdatefound?.();
    worker.state = 'installing';
    worker.onstatechange?.();

    expect(onSuccess).not.toHaveBeenCalled();
    expect(onUpdate).not.toHaveBeenCalled();
  });

  test('a successful precache with no onSuccess callback configured does not throw', async () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any -- test-only override of vite's env
    (import.meta.env as any).PROD = true;
    const registration: {
      onupdatefound: (() => void) | null;
      installing: ReturnType<typeof fakeInstallingWorker> | null;
    } = { onupdatefound: null, installing: null };
    const swRegister = vi.fn().mockResolvedValue(registration);
    const win = stubWindow({
      hostname: 'example.com',
      href: 'http://example.com/',
      origin: 'http://example.com',
    });
    stubNavigator({ serviceWorker: { register: swRegister } });

    const { register } = await import('./serviceWorkerRegistration');
    register();
    win.fireLoad();
    await Promise.resolve();
    await Promise.resolve();

    const worker = fakeInstallingWorker();
    registration.installing = worker;
    registration.onupdatefound?.();
    worker.state = 'installed';
    expect(() => worker.onstatechange?.()).not.toThrow();
  });

  test('reports an update via onUpdate when a controller already exists', async () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any -- test-only override of vite's env
    (import.meta.env as any).PROD = true;
    const registration: {
      onupdatefound: (() => void) | null;
      installing: ReturnType<typeof fakeInstallingWorker> | null;
    } = { onupdatefound: null, installing: null };
    const swRegister = vi.fn().mockResolvedValue(registration);
    const win = stubWindow({
      hostname: 'example.com',
      href: 'http://example.com/',
      origin: 'http://example.com',
    });
    stubNavigator({ serviceWorker: { register: swRegister, controller: {} } });

    const { register } = await import('./serviceWorkerRegistration');
    const onUpdate = vi.fn();
    register({ onUpdate });
    win.fireLoad();
    await Promise.resolve();
    await Promise.resolve();

    const worker = fakeInstallingWorker();
    registration.installing = worker;
    registration.onupdatefound?.();
    worker.state = 'installed';
    worker.onstatechange?.();

    expect(onUpdate).toHaveBeenCalledWith(registration);
  });

  test('an update with no onUpdate callback configured does not throw', async () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any -- test-only override of vite's env
    (import.meta.env as any).PROD = true;
    const registration: {
      onupdatefound: (() => void) | null;
      installing: ReturnType<typeof fakeInstallingWorker> | null;
    } = { onupdatefound: null, installing: null };
    const swRegister = vi.fn().mockResolvedValue(registration);
    const win = stubWindow({
      hostname: 'example.com',
      href: 'http://example.com/',
      origin: 'http://example.com',
    });
    stubNavigator({ serviceWorker: { register: swRegister, controller: {} } });

    const { register } = await import('./serviceWorkerRegistration');
    register();
    win.fireLoad();
    await Promise.resolve();
    await Promise.resolve();

    const worker = fakeInstallingWorker();
    registration.installing = worker;
    registration.onupdatefound?.();
    worker.state = 'installed';
    expect(() => worker.onstatechange?.()).not.toThrow();
  });

  test('ignores onupdatefound when installing worker is null', async () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any -- test-only override of vite's env
    (import.meta.env as any).PROD = true;
    const registration: {
      onupdatefound: (() => void) | null;
      installing: null;
    } = { onupdatefound: null, installing: null };
    const swRegister = vi.fn().mockResolvedValue(registration);
    const win = stubWindow({
      hostname: 'example.com',
      href: 'http://example.com/',
      origin: 'http://example.com',
    });
    stubNavigator({ serviceWorker: { register: swRegister } });

    const { register } = await import('./serviceWorkerRegistration');
    register();
    win.fireLoad();
    await Promise.resolve();
    await Promise.resolve();

    expect(() => registration.onupdatefound?.()).not.toThrow();
  });

  test('logs but does not throw when registration itself fails', async () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any -- test-only override of vite's env
    (import.meta.env as any).PROD = true;
    const swRegister = vi.fn().mockRejectedValue(new Error('network down'));
    const win = stubWindow({
      hostname: 'example.com',
      href: 'http://example.com/',
      origin: 'http://example.com',
    });
    stubNavigator({ serviceWorker: { register: swRegister } });
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

    const { register } = await import('./serviceWorkerRegistration');
    register();
    win.fireLoad();
    await Promise.resolve();
    await Promise.resolve();

    expect(errorSpy).toHaveBeenCalledWith(
      'Error during service worker registration:',
      expect.any(Error),
    );
  });

  describe('on localhost', () => {
    function stubFetch(response: { status: number; contentType: string | null } | 'offline') {
      if (response === 'offline') {
        return vi.fn().mockRejectedValue(new Error('offline'));
      }
      return vi.fn().mockResolvedValue({
        status: response.status,
        headers: { get: () => response.contentType },
      });
    }

    test('reloads the page when no service worker script is found (404)', async () => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- test-only override of vite's env
      (import.meta.env as any).PROD = true;
      const unregister = vi.fn().mockResolvedValue(undefined);
      const win = stubWindow({
        hostname: 'localhost',
        href: 'http://localhost/',
        origin: 'http://localhost',
      });
      const readyRegistration = { unregister };
      stubNavigator({
        serviceWorker: {
          register: vi.fn(),
          ready: Promise.resolve(readyRegistration),
        },
      });
      vi.stubGlobal('fetch', stubFetch({ status: 404, contentType: 'text/html' }));

      const { register } = await import('./serviceWorkerRegistration');
      register();
      win.fireLoad();
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();

      expect(unregister).toHaveBeenCalled();
      expect(win.reload).toHaveBeenCalled();
    });

    test('reloads the page when the response is not javascript', async () => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- test-only override of vite's env
      (import.meta.env as any).PROD = true;
      const unregister = vi.fn().mockResolvedValue(undefined);
      const win = stubWindow({
        hostname: 'localhost',
        href: 'http://localhost/',
        origin: 'http://localhost',
      });
      stubNavigator({
        serviceWorker: {
          register: vi.fn(),
          ready: Promise.resolve({ unregister }),
        },
      });
      vi.stubGlobal('fetch', stubFetch({ status: 200, contentType: 'text/html' }));

      const { register } = await import('./serviceWorkerRegistration');
      register();
      win.fireLoad();
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();

      expect(unregister).toHaveBeenCalled();
      expect(win.reload).toHaveBeenCalled();
    });

    test('registers normally when a real service worker script is found', async () => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- test-only override of vite's env
      (import.meta.env as any).PROD = true;
      const swRegister = vi.fn().mockResolvedValue({ onupdatefound: null, installing: null });
      const win = stubWindow({
        hostname: 'localhost',
        href: 'http://localhost/',
        origin: 'http://localhost',
      });
      stubNavigator({
        serviceWorker: {
          register: swRegister,
          ready: Promise.resolve({ unregister: vi.fn() }),
        },
      });
      vi.stubGlobal('fetch', stubFetch({ status: 200, contentType: 'application/javascript' }));
      const logSpy = vi.spyOn(console, 'log').mockImplementation(() => {});

      const { register } = await import('./serviceWorkerRegistration');
      register();
      win.fireLoad();
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();

      expect(swRegister).toHaveBeenCalledWith('/service-worker.js');
      // The localhost dev-mode "served cache-first" logging path.
      expect(logSpy).toHaveBeenCalledWith(expect.stringContaining('cache-first'));
    });

    test('logs offline mode when the validity check itself fails to fetch', async () => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- test-only override of vite's env
      (import.meta.env as any).PROD = true;
      const win = stubWindow({
        hostname: 'localhost',
        href: 'http://localhost/',
        origin: 'http://localhost',
      });
      stubNavigator({
        serviceWorker: {
          register: vi.fn(),
          ready: Promise.resolve({ unregister: vi.fn() }),
        },
      });
      vi.stubGlobal('fetch', stubFetch('offline'));
      const logSpy = vi.spyOn(console, 'log').mockImplementation(() => {});

      const { register } = await import('./serviceWorkerRegistration');
      register();
      win.fireLoad();
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();

      expect(logSpy).toHaveBeenCalledWith(
        'No internet connection found. App is running in offline mode.',
      );
      expect(win.reload).not.toHaveBeenCalled();
    });
  });
});

describe('unregister', () => {
  test('unregisters the ready registration when supported', async () => {
    const unregister = vi.fn();
    stubNavigator({ serviceWorker: { ready: Promise.resolve({ unregister }) } });

    const { unregister: unregisterSW } = await import('./serviceWorkerRegistration');
    unregisterSW();
    await Promise.resolve();
    await Promise.resolve();

    expect(unregister).toHaveBeenCalled();
  });

  test('does nothing when the browser has no serviceWorker support', async () => {
    stubNavigator({});

    const { unregister: unregisterSW } = await import('./serviceWorkerRegistration');
    expect(() => unregisterSW()).not.toThrow();
  });

  test('logs the error message when the ready promise rejects', async () => {
    stubNavigator({
      serviceWorker: { ready: Promise.reject(new Error('boom')) },
    });
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

    const { unregister: unregisterSW } = await import('./serviceWorkerRegistration');
    unregisterSW();
    await Promise.resolve();
    await Promise.resolve();

    expect(errorSpy).toHaveBeenCalledWith('boom');
  });
});
