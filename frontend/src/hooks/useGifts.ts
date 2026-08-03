import { useState, useCallback, useMemo } from 'react';
import {
  getGifts,
  createGift,
  updateGift,
  deleteGift,
  Gift,
  GiftInput,
} from '../api/gifts';
import { handleFetchError } from '../utils/errorHandler';

// useGifts backs the contact page's gift surface (T20b): the gift records for
// one contact, keyed by VCardUID (entityId). Open ideas are surfaced
// separately from the resolved (purchased/given/received) records so the
// opportunistic capture side stays one click away.
export function useGifts(entityId: string | undefined) {
  const [items, setItems] = useState<Gift[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(
    async (overrideEntityId?: string) => {
      const uid = overrideEntityId ?? entityId;
      if (!uid) return;
      setLoading(true);
      setError(null);
      try {
        const response = await getGifts({ entityId: uid, limit: 100 });
        setItems(response.gifts || []);
      } catch (err) {
        setError(handleFetchError(err, 'fetching gifts'));
      } finally {
        setLoading(false);
      }
    },
    [entityId]
  );

  const ideas = useMemo(() => items.filter((g) => g.status === 'idea'), [items]);
  const resolved = useMemo(() => items.filter((g) => g.status !== 'idea'), [items]);

  const handleCreate = useCallback(
    async (data: GiftInput) => {
      await createGift(data);
      await refresh();
    },
    [refresh]
  );

  const handleUpdate = useCallback(
    async (id: string, data: GiftInput) => {
      await updateGift(id, data);
      await refresh();
    },
    [refresh]
  );

  const handleDelete = useCallback(
    async (id: string) => {
      await deleteGift(id);
      await refresh();
    },
    [refresh]
  );

  return {
    items,
    ideas,
    resolved,
    loading,
    error,
    refresh,
    handleCreate,
    handleUpdate,
    handleDelete,
  };
}
