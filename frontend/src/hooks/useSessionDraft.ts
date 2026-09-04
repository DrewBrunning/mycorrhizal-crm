import { useCallback, useEffect } from 'react';

// Issue #557 item 4: "preserving rather than only warning" -- a draft
// persisted to sessionStorage turns a 401/crash/accidental-close into an
// interruption instead of a loss, and it covers cases a close-time prompt
// physically cannot (a render crash into the ErrorBoundary, a killed tab).
// sessionStorage (not localStorage): a draft is scoped to this one tab's
// life, not carried indefinitely across browser restarts on a shared/public
// machine -- consistent with this app not persisting form drafts anywhere
// else today.
const STORAGE_PREFIX = 'mycorrhizal:draft:';

function storageKey(key: string): string {
  return `${STORAGE_PREFIX}${key}`;
}

// readSessionDraft reads a previously-saved draft, if any. Call it once, at
// the point a dialog/form (re)opens, to decide what to restore.
export function readSessionDraft<T>(key: string): T | null {
  try {
    const raw = sessionStorage.getItem(storageKey(key));
    if (!raw) return null;
    return JSON.parse(raw) as T;
  } catch {
    // sessionStorage unavailable (private browsing, disabled storage) or the
    // stored value is corrupt -- either way, behave as if there was no
    // draft. This is a convenience, never a source of truth.
    return null;
  }
}

// useSessionDraft persists `value` to sessionStorage under `key` on every
// change while `enabled` (normally: the dialog is open and the form is
// dirty). Returns clearDraft, to be called once the data is safely saved (or
// the user explicitly discards it) so a stale draft doesn't reappear later.
export function useSessionDraft<T>(
  key: string,
  value: T,
  enabled: boolean,
): { clearDraft: () => void } {
  // `value` is a fresh object each render by design (callers pass an inline
  // snapshot of their form fields), so it can't be a dependency directly --
  // every render would re-fire the effect. Depending on its JSON instead
  // makes the effect a no-op re-run whenever the content hasn't actually
  // changed, which is the real "did the draft change" question.
  const serializedValue = JSON.stringify(value);
  useEffect(() => {
    if (!enabled) return;
    try {
      sessionStorage.setItem(storageKey(key), serializedValue);
    } catch {
      // Best-effort only (quota exceeded, storage disabled) -- never block
      // the actual editing experience over a failed draft write.
    }
  }, [key, serializedValue, enabled]);

  const clearDraft = useCallback(() => {
    try {
      sessionStorage.removeItem(storageKey(key));
    } catch {
      // Nothing to do if storage isn't available.
    }
  }, [key]);

  return { clearDraft };
}
