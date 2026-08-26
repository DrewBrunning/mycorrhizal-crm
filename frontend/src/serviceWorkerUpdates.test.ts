import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import {
  applyUpdate,
  notifyUpdateAvailable,
  onUpdateAvailable,
  resetServiceWorkerUpdatesForTest,
} from './serviceWorkerUpdates';

// A minimal stand-in for the bits of ServiceWorkerRegistration this module
// touches. `waiting` is the new worker that has installed but not taken over.
function fakeRegistration(waiting: { postMessage: (m: unknown) => void } | null) {
  return { waiting } as unknown as ServiceWorkerRegistration;
}

// Captures the controllerchange listener applyUpdate installs so a test can
// fire it, and counts reloads.
function stubServiceWorkerContainer() {
  const listeners: Array<() => void> = [];
  const reload = vi.fn();

  vi.stubGlobal('navigator', {
    serviceWorker: {
      addEventListener: (event: string, fn: () => void) => {
        if (event === 'controllerchange') {
          listeners.push(fn);
        }
      },
    },
  });
  vi.stubGlobal('window', { location: { reload } });

  return {
    reload,
    fireControllerChange: () =>
      listeners.forEach((fn) => {
        fn();
      }),
    listenerCount: () => listeners.length,
  };
}

beforeEach(() => {
  resetServiceWorkerUpdatesForTest();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('onUpdateAvailable', () => {
  test('delivers an update that arrives after subscribing', () => {
    const seen = vi.fn();
    onUpdateAvailable(seen);

    const reg = fakeRegistration(null);
    notifyUpdateAvailable(reg);

    expect(seen).toHaveBeenCalledWith(reg);
  });

  // register() runs on window's load event, outside React, so the update can
  // land before the prompt has mounted. Without the replay the notice would
  // never be shown for that ordering.
  test('replays an update that arrived before anyone subscribed', () => {
    const reg = fakeRegistration(null);
    notifyUpdateAvailable(reg);

    const seen = vi.fn();
    onUpdateAvailable(seen);

    expect(seen).toHaveBeenCalledWith(reg);
  });

  test('stops delivering after unsubscribe', () => {
    const seen = vi.fn();
    const unsubscribe = onUpdateAvailable(seen);
    seen.mockClear();

    unsubscribe();
    notifyUpdateAvailable(fakeRegistration(null));

    expect(seen).not.toHaveBeenCalled();
  });
});

describe('applyUpdate', () => {
  test('asks the waiting worker to take over and reloads once it has', () => {
    const sw = stubServiceWorkerContainer();
    const postMessage = vi.fn();

    applyUpdate(fakeRegistration({ postMessage }));

    expect(postMessage).toHaveBeenCalledWith({ type: 'SKIP_WAITING' });
    // Crucially NOT reloaded yet: the old worker still controls the page, so
    // reloading now would re-serve the old shell and re-prompt.
    expect(sw.reload).not.toHaveBeenCalled();

    sw.fireControllerChange();
    expect(sw.reload).toHaveBeenCalledTimes(1);
  });

  // controllerchange can fire more than once; without the guard each one
  // navigates again, which is the reload loop this pattern is known for.
  test('reloads only once even if controllerchange fires repeatedly', () => {
    const sw = stubServiceWorkerContainer();

    applyUpdate(fakeRegistration({ postMessage: vi.fn() }));
    sw.fireControllerChange();
    sw.fireControllerChange();
    sw.fireControllerChange();

    expect(sw.reload).toHaveBeenCalledTimes(1);
  });

  test('reloads immediately when there is no waiting worker', () => {
    const sw = stubServiceWorkerContainer();

    applyUpdate(fakeRegistration(null));

    expect(sw.reload).toHaveBeenCalledTimes(1);
    expect(sw.listenerCount()).toBe(0);
  });
});
