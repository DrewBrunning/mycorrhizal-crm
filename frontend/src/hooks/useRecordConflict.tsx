import { useCallback, useState } from 'react';
import { isPreconditionFailed } from '../api/client';
import RecordConflictDialog from '../components/RecordConflictDialog';

// ADR 0008 / ADR 0010 for pages that edit a single record outside the contact
// detail page: `handleConflict(err)` returns true (and opens the dialog) when
// `err` is a 412 from a stale `If-Match`, so a catch block can bail out before
// its generic error handling. "Load latest" runs `onReload` (discard the edit,
// refetch); "Keep editing" just closes the dialog. Nothing is merged.
export function useRecordConflict(onReload: () => void | Promise<void>) {
  const [open, setOpen] = useState(false);

  const handleConflict = useCallback((err: unknown): boolean => {
    if (!isPreconditionFailed(err)) return false;
    setOpen(true);
    return true;
  }, []);

  const conflictDialog = (
    <RecordConflictDialog
      open={open}
      onKeepEditing={() => setOpen(false)}
      onReload={() => {
        void Promise.resolve(onReload()).finally(() => setOpen(false));
      }}
    />
  );

  return { handleConflict, conflictDialog };
}
