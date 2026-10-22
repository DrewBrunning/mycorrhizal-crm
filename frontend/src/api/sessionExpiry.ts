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

const listeners = new Set<Listener>();
const restoredListeners = new Set<() => void>();
const signInRequestListeners = new Set<() => void>();

// notifySessionExpired is called from apiFetch on every 401. No replay of a
// pending event: both the app-shell notice and <SessionExpiredGate> mount
// before any page can have issued its first authenticated request, so there is
// nothing to miss in practice, and skipping the replay buffer keeps this
// module's state trivial (nothing to reset between tests either).
export function notifySessionExpired(mode: SessionExpiryMode): void {
  for (const listener of listeners) listener({ mode });
}

// onSessionExpired subscribes to session-expiry notifications. Returns an
// unsubscribe function. Multiple subscribers are supported: the app-shell
// notice (issue #1793) and the modal gate both need the event.
export function onSessionExpired(fn: Listener): () => void {
  listeners.add(fn);
  return () => {
    listeners.delete(fn);
  };
}

// notifySessionRestored fires after an in-place re-authentication succeeds, so
// the app-shell notice can disappear without a reload.
export function notifySessionRestored(): void {
  for (const listener of restoredListeners) listener();
}

export function onSessionRestored(fn: () => void): () => void {
  restoredListeners.add(fn);
  return () => {
    restoredListeners.delete(fn);
  };
}

// requestSessionSignIn asks the gate to open its sign-in dialog — the action
// behind the main-content notice's button.
export function requestSessionSignIn(): void {
  for (const listener of signInRequestListeners) listener();
}

export function onSessionSignInRequest(fn: () => void): () => void {
  signInRequestListeners.add(fn);
  return () => {
    signInRequestListeners.delete(fn);
  };
}
