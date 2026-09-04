import { useEffect } from 'react';

// Issue #557: warns before a tab close, reload, or external navigation
// discards unsaved work. `beforeunload` is the only guard that can catch
// those three cases at all -- react-router's useBlocker only covers
// in-app navigation, and this app's router (`<BrowserRouter>` + `<Routes>`,
// not a data router) doesn't provide the router context useBlocker needs
// regardless. In-app navigation away from a dirty *dialog* is instead
// guarded at the dialog's own close handlers (see useDiscardGuard) -- every
// editing surface in this app is a MUI Dialog, not a routed page, so that
// covers the cases useBlocker exists for here without the router migration.
//
// The browser controls the confirmation's wording; the `returnValue` string
// itself is ignored by every modern browser (it shows a fixed built-in
// message instead), but both the assignment and the return are required for
// the various engines that implement this event differently.
export function useBeforeUnloadGuard(isDirty: boolean): void {
  useEffect(() => {
    if (!isDirty) return;

    const handler = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = '';
      return '';
    };

    window.addEventListener('beforeunload', handler);
    return () => window.removeEventListener('beforeunload', handler);
  }, [isDirty]);
}
