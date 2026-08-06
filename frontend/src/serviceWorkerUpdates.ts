// Bridge between serviceWorkerRegistration's non-React callbacks and the React
// tree, so `<ServiceWorkerUpdatePrompt>` can offer the user a reload when a new
// build has been precached.
//
// A module-level holder rather than a React context because register() is
// called from index.tsx *outside* the tree, on window's load event, and can
// fire before the prompt has mounted. The pending registration is replayed to
// the first subscriber for exactly that reason.

type UpdateListener = (registration: ServiceWorkerRegistration) => void;

let pendingRegistration: ServiceWorkerRegistration | null = null;
let listener: UpdateListener | null = null;

// notifyUpdateAvailable is the onUpdate callback handed to register(). Called
// once a new service worker has finished installing and is waiting to take
// over.
export function notifyUpdateAvailable(registration: ServiceWorkerRegistration): void {
  pendingRegistration = registration;
  listener?.(registration);
}

// onUpdateAvailable subscribes to update notifications, immediately replaying
// one that already arrived. Returns an unsubscribe function.
export function onUpdateAvailable(fn: UpdateListener): () => void {
  listener = fn;
  if (pendingRegistration) {
    fn(pendingRegistration);
  }
  return () => {
    if (listener === fn) {
      listener = null;
    }
  };
}

// applyUpdate hands control to the waiting service worker and reloads once it
// has taken over.
//
// The reload must wait for 'controllerchange' rather than happening straight
// after postMessage: reloading first would just re-fetch the app shell from the
// OLD worker, which is still in control until skipWaiting resolves, and the
// user would be told about the same update again. The `reloaded` guard is the
// standard defence against the reload loop this pattern is notorious for --
// controllerchange can fire more than once, and each one would otherwise
// trigger another navigation.
export function applyUpdate(registration: ServiceWorkerRegistration): void {
  const waiting = registration.waiting;
  if (!waiting) {
    // Nothing waiting (already activated, or the update was applied in another
    // tab) -- a plain reload is all that is left to do.
    window.location.reload();
    return;
  }

  let reloaded = false;
  navigator.serviceWorker.addEventListener('controllerchange', () => {
    if (reloaded) {
      return;
    }
    reloaded = true;
    window.location.reload();
  });

  waiting.postMessage({ type: 'SKIP_WAITING' });
}

// resetServiceWorkerUpdatesForTest clears module state between tests.
export function resetServiceWorkerUpdatesForTest(): void {
  pendingRegistration = null;
  listener = null;
}
