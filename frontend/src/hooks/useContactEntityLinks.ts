import { useCallback, useEffect, useRef, useState } from 'react';
import {
  adoptContactEntityLink,
  type ContactEntityLink,
  type ContactEntityLinkFromExistingInput,
  type ContactEntityLinkInput,
  createContactEntityLink,
  deleteContactEntityLink,
  linkContactEntityFromExisting,
  listContactEntityLinks,
  updateContactEntityLink,
} from '../api/entities';
import { getErrorMessage } from '../utils/errorHandler';

interface Options {
  /**
   * Called after any write that can change what the contact's record projects
   * (a link, a primary flag, an adopt): the page reloads the record so the
   * Work and address sections show the new ORG/ADR.
   */
  onRecordChanged?: () => void | Promise<void>;
}

// A contact's entity links (issue #2301, ADR 0043 decisions 2, 3 and 10).
export function useContactEntityLinks(contactId: number | undefined, options: Options = {}) {
  const [links, setLinks] = useState<ContactEntityLink[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const requestRef = useRef(0);
  const onChangedRef = useRef(options.onRecordChanged);
  onChangedRef.current = options.onRecordChanged;

  const refresh = useCallback(async () => {
    if (contactId === undefined) return;
    const requestId = ++requestRef.current;
    setLoading(true);
    setError(null);
    try {
      const fetched = await listContactEntityLinks(contactId);
      if (requestRef.current === requestId) setLinks(fetched);
    } catch (err) {
      if (requestRef.current === requestId) {
        setLinks([]);
        setError(getErrorMessage(err));
      }
    } finally {
      if (requestRef.current === requestId) setLoading(false);
    }
  }, [contactId]);

  useEffect(() => {
    setLinks([]);
    void refresh();
  }, [refresh]);

  // Run a write, then refresh the links and tell the page. A failed write
  // rethrows so the calling dialog can keep its own state and show the error.
  const mutate = useCallback(
    async <T>(write: (id: number) => Promise<T>): Promise<T> => {
      if (contactId === undefined) throw new Error('no contact');
      const result = await write(contactId);
      await refresh();
      await onChangedRef.current?.();
      return result;
    },
    [contactId, refresh],
  );

  const save = useCallback(
    (input: ContactEntityLinkInput, editing?: ContactEntityLink | null) =>
      mutate((id) =>
        editing
          ? updateContactEntityLink(id, editing.id, input, editing.revision)
          : createContactEntityLink(id, input),
      ),
    [mutate],
  );

  const setPrimary = useCallback(
    (link: ContactEntityLink, isPrimary: boolean) =>
      mutate((id) =>
        updateContactEntityLink(
          id,
          link.id,
          {
            entity_id: link.entity_id,
            role: link.role,
            role_label: link.role_label,
            is_primary: isPrimary,
            ...(link.period ? { period: link.period } : {}),
            sensitivity: link.sensitivity,
          },
          link.revision,
        ),
      ),
    [mutate],
  );

  const remove = useCallback(
    (link: ContactEntityLink) => mutate((id) => deleteContactEntityLink(id, link.id)),
    [mutate],
  );

  const adopt = useCallback(
    (link: ContactEntityLink) => mutate((id) => adoptContactEntityLink(id, link.id)),
    [mutate],
  );

  const linkFromExisting = useCallback(
    (input: ContactEntityLinkFromExistingInput) =>
      mutate((id) => linkContactEntityFromExisting(id, input)),
    [mutate],
  );

  return { links, loading, error, refresh, save, setPrimary, remove, adopt, linkFromExisting };
}
