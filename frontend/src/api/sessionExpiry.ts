// Bridge between apiFetch's 401 handling and the React tree, mirroring
// serviceWorkerUpdates.ts's non-React-callback pattern.
//
// Issue #557: apiFetch used to react to a 401 by clearing localStorage and
// doing `window.location.href = '/login'` -- a hard navigation that tears
// down the whole React tree, including any unsaved form the user was in the
// middle of. That call site can't render UI itself (it's a plain fetch
// wrapper, not a component), so it publishes an event here instead, and
// <SessionExpiredGate> (mounted once, at the app root, outside anything that
// could itself crash or get route-unmounted) subscribes and decides what to
// show.
//
// A 401 arrives for two very different reasons and the response has to
// differ:
//   - 'passive': a GET (a background poll, a dashboard refresh, an
//     autocomplete lookup) came back 401. The user may be mid-keystroke in a
//     dialog that has nothing to do with this request; grabbing focus with a
//     modal would be its own small data-loss/annoyance bug. Non-blocking.
//   - 'blocking': a mutating request (POST/PUT/PATCH/DELETE) came back 401.
//     Every mutation in this app is a direct, synchronous user action (Save,
//     Delete, Confirm...) -- the user is actively waiting on this one and
//     needs to know right away that it didn't go through, before they
//     navigate off and assume it did.
export type SessionExpiryMode = 'blocking' | 'passive';

export interface SessionExpiryEvent {
  mode: SessionExpiryMode;
}

type Listener = (event: SessionExpiryEvent) => void;

let listener: Listener | null = null;

// notifySessionExpired is called from apiFetch on every 401. No replay of a
// pending event: <SessionExpiredGate> mounts at the very top of the tree
// before any page can have issued its first authenticated request, so there
// is nothing to miss in practice, and skipping the replay buffer keeps this
// module's state trivial (nothing to reset between tests either).
export function notifySessionExpired(mode: SessionExpiryMode): void {
  listener?.({ mode });
}

// onSessionExpired subscribes to session-expiry notifications. Returns an
// unsubscribe function. Only one subscriber is expected in practice (the one
// <SessionExpiredGate>), same as onUpdateAvailable.
export function onSessionExpired(fn: Listener): () => void {
  listener = fn;
  return () => {
    if (listener === fn) {
      listener = null;
    }
  };
}
