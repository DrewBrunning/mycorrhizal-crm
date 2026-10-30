import { useCallback, useEffect, useState } from 'react';
import {
  type ContactFieldVisibility,
  type ContactFieldVisibilityInput,
  getContactFieldVisibility,
  putContactFieldVisibility,
  resetContactFieldVisibility,
} from '../api/contactFieldVisibility';

// Loads and edits one contact's field visibility (issue #1997). Loading fails
// soft: with no `visibility` the page falls back to the account default it
// already has, so a transient error never blanks a contact's fields. Saves
// reject so the dialog can show the error.
export function useContactFieldVisibility(contactId: number | undefined) {
  const [visibility, setVisibility] = useState<ContactFieldVisibility | null>(null);

  useEffect(() => {
    if (contactId == null) return;
    let cancelled = false;
    setVisibility(null);
    getContactFieldVisibility(contactId)
      .then((v) => {
        if (!cancelled) setVisibility(v);
      })
      .catch(() => {
        // Fail soft: keep the account-default rendering.
      });
    return () => {
      cancelled = true;
    };
  }, [contactId]);

  const save = useCallback(
    async (input: ContactFieldVisibilityInput) => {
      if (contactId == null) return;
      setVisibility(await putContactFieldVisibility(contactId, input));
    },
    [contactId],
  );

  const reset = useCallback(async () => {
    if (contactId == null) return;
    setVisibility(await resetContactFieldVisibility(contactId));
  }, [contactId]);

  return { visibility, save, reset };
}
