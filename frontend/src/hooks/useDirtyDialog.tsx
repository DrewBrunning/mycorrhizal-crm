import { type ReactElement, useCallback, useEffect, useState } from 'react';
import ConfirmDiscardDialog from '../components/ConfirmDiscardDialog';
import { useDiscardGuard } from './useDiscardGuard';

export interface DirtyDialogOptions {
  // Runs when a discard is confirmed as the result of an in-app route
  // navigation. Defaults to `onClose`, since a dialog's close handler is
  // already its full cleanup path.
  onNavigationDiscard?: () => void;
}

export interface DirtyDialog {
  isDirty: boolean;
  // Call from every user-edit handler. Programmatic initialisation (the
  // on-open effect/reset) must NOT call this -- a prefilled edit dialog or an
  // async default is the starting state, not unsaved work.
  markDirty: () => void;
  // Call when the user has saved, so closing the success view doesn't prompt.
  clearDirty: () => void;
  // Call immediately before an in-app navigation triggered by a successful
  // save, so the just-armed navigation blocker does not intercept it.
  allowNavigation: () => void;
  // Use as <AppDialog onClose> and as the Cancel button's onClick. Shows the
  // discard confirmation when dirty, otherwise closes immediately.
  requestClose: () => void;
  // Render inside <AppDialog>: the navigation blocker and the confirmation.
  guardElement: ReactElement;
}

// Issue #1780: one-line discard guard for a create/edit dialog. It tracks the
// dirty flag (reset whenever the dialog opens, set by the dialog's own edit
// handlers) and wires it into the existing useDiscardGuard, which covers
// Cancel/Escape, tab close/reload, and in-app route navigation.
export function useDirtyDialog(
  open: boolean,
  onClose: () => void,
  options: DirtyDialogOptions = {},
): DirtyDialog {
  const [isDirty, setIsDirty] = useState(false);

  // A reopened dialog starts clean. Closing keeps the flag until then, which
  // is harmless: the dialog is unmounted/closed and no close path consults it.
  useEffect(() => {
    if (open) setIsDirty(false);
  }, [open]);

  const markDirty = useCallback(() => setIsDirty(true), []);
  const clearDirty = useCallback(() => setIsDirty(false), []);

  const { guardedClose, confirmDialogProps, navigationGuardElement, allowNavigation } =
    useDiscardGuard(isDirty, {
      onNavigationDiscard: options.onNavigationDiscard ?? onClose,
    });

  const requestClose = useCallback(() => guardedClose(onClose), [guardedClose, onClose]);

  return {
    isDirty,
    markDirty,
    clearDirty,
    allowNavigation,
    requestClose,
    guardElement: (
      <>
        {navigationGuardElement}
        <ConfirmDiscardDialog {...confirmDialogProps} />
      </>
    ),
  };
}
