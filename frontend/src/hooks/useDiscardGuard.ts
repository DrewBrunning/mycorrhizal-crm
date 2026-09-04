import { useCallback, useState } from 'react';
import { useBeforeUnloadGuard } from './useBeforeUnloadGuard';

interface DiscardGuard {
  // Wrap a real close/cancel handler with this. When the form is clean it
  // runs immediately; when dirty it opens <ConfirmDiscardDialog> instead and
  // only runs once the user confirms discarding.
  guardedClose: (closeFn: () => void) => void;
  // Spread directly onto <ConfirmDiscardDialog>.
  confirmDialogProps: {
    open: boolean;
    onKeepEditing: () => void;
    onDiscard: () => void;
  };
}

// Issue #557: pairs a beforeunload guard (tab close / reload / external nav)
// with a confirm-before-discard guard for the dialog's own close paths
// (Cancel button, Escape key -- AppDialog already blocks a stray backdrop
// click for every dialog in this app, dirty or not). One hook covers both,
// since both guards exist to answer the same question: does closing right
// now lose something the user hasn't saved.
export function useDiscardGuard(isDirty: boolean): DiscardGuard {
  useBeforeUnloadGuard(isDirty);

  const [pendingClose, setPendingClose] = useState<(() => void) | null>(null);

  const guardedClose = useCallback(
    (closeFn: () => void) => {
      if (isDirty) {
        setPendingClose(() => closeFn);
      } else {
        closeFn();
      }
    },
    [isDirty],
  );

  const onDiscard = useCallback(() => {
    setPendingClose((current) => {
      current?.();
      return null;
    });
  }, []);

  const onKeepEditing = useCallback(() => setPendingClose(null), []);

  return {
    guardedClose,
    confirmDialogProps: {
      open: pendingClose !== null,
      onKeepEditing,
      onDiscard,
    },
  };
}
