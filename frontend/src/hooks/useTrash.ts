import { useCallback, useState } from 'react';
import {
  emptyTrash,
  getTrashedContacts,
  purgeTrashedContact,
  restoreTrashedContact,
  type TrashedContact,
  type TrashRestoreResult,
} from '../api/trash';
import { handleFetchError } from '../utils/errorHandler';

// useTrash drives the contact Trash page (issue #2002). The page owns user
// feedback for the mutations (it needs to special-case a 409), so they throw.
export function useTrash() {
  const [items, setItems] = useState<TrashedContact[]>([]);
  const [retentionDays, setRetentionDays] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const all: TrashedContact[] = [];
      let page = 1;
      // The trash is small and bounded by the retention window; walk every page.
      for (let guard = 0; guard < 50; guard++) {
        const res = await getTrashedContacts({ page, limit: 100 });
        all.push(...res.contacts);
        setRetentionDays(res.retention_days);
        if (all.length >= res.total || res.contacts.length === 0) break;
        page += 1;
      }
      setItems(all);
    } catch (err) {
      setError(handleFetchError(err, 'fetching the trash'));
    } finally {
      setLoading(false);
    }
  }, []);

  const restore = async (id: number): Promise<TrashRestoreResult> => {
    const result = await restoreTrashedContact(id);
    setItems((prev) => prev.filter((i) => i.contact.id !== id));
    return result;
  };

  const purge = async (id: number): Promise<void> => {
    await purgeTrashedContact(id);
    setItems((prev) => prev.filter((i) => i.contact.id !== id));
  };

  const empty = async (): Promise<number> => {
    const purged = await emptyTrash();
    setItems([]);
    return purged;
  };

  return { items, retentionDays, loading, error, refresh, restore, purge, empty };
}
