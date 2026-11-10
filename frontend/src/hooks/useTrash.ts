import { useCallback, useState } from 'react';
import { type EntityRestoreResponse, restoreEntity } from '../api/entities';
import {
  emptyTrash,
  getTrashedContacts,
  getTrashedEntities,
  purgeTrashedContact,
  restoreTrashedContact,
  type TrashedContact,
  type TrashedEntity,
  type TrashRestoreResult,
} from '../api/trash';
import { handleFetchError } from '../utils/errorHandler';

type Page = { total: number; retention_days: number };

// The trash is small and bounded by the retention window; walk every page.
async function walkPages<P extends Page, T>(
  fetchPage: (params: { page: number; limit: number }) => Promise<P>,
  rowsOf: (res: P) => T[],
  onRetention: (days: number) => void,
): Promise<T[]> {
  const all: T[] = [];
  let page = 1;
  for (let guard = 0; guard < 50; guard++) {
    const res = await fetchPage({ page, limit: 100 });
    const rows = rowsOf(res);
    all.push(...rows);
    onRetention(res.retention_days);
    if (all.length >= res.total || rows.length === 0) break;
    page += 1;
  }
  return all;
}

// useTrash drives the Trash page: contacts (issue #2002) and entities (issue
// #2446). The page owns user
// feedback for the mutations (it needs to special-case a 409), so they throw.
export function useTrash() {
  const [items, setItems] = useState<TrashedContact[]>([]);
  const [entities, setEntities] = useState<TrashedEntity[]>([]);
  const [retentionDays, setRetentionDays] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [allContacts, allEntities] = await Promise.all([
        walkPages(getTrashedContacts, (r) => r.contacts, setRetentionDays),
        walkPages(getTrashedEntities, (r) => r.entities, setRetentionDays),
      ]);
      setItems(allContacts);
      setEntities(allEntities);
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

  // Entity restore reuses POST /entities/{id}/restore (one restore code path).
  const restoreEntityRow = async (id: string): Promise<EntityRestoreResponse> => {
    const result = await restoreEntity(id);
    setEntities((prev) => prev.filter((i) => i.entity.id !== id));
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

  return {
    items,
    entities,
    restoreEntity: restoreEntityRow,
    retentionDays,
    loading,
    error,
    refresh,
    restore,
    purge,
    empty,
  };
}
