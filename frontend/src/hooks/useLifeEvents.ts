import { useCallback, useEffect, useState } from 'react';
import { isPreconditionFailed } from '../api/client';
import { type Contact, getContactsByUid } from '../api/contacts';
import {
  createLifeEvent,
  deleteLifeEvent,
  type GetLifeEventsParams,
  getLifeEvents,
  type LifeEvent,
  type LifeEventInputData,
  updateLifeEvent,
} from '../api/lifeEvents';
import { handleFetchError } from '../utils/errorHandler';

export function useLifeEvents(
  entityId: string | undefined,
  // Called when an update/delete is rejected with 412 (the event changed
  // elsewhere; ADR 0008).
  onConflict?: () => void,
) {
  const [events, setEvents] = useState<LifeEvent[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [contactsByUid, setContactsByUid] = useState<Map<string, Contact>>(new Map());

  const refresh = useCallback(
    async (overrideEntityId?: string) => {
      const uid = overrideEntityId ?? entityId;
      if (!uid) return;
      setLoading(true);
      setError(null);
      try {
        const params: GetLifeEventsParams = { entity_id: uid, limit: 50 };
        const response = await getLifeEvents(params);
        const fetched = response.life_events || [];
        setEvents(fetched);
        // T17: no exact total from the cursor envelope; the fetched count is
        // the closest approximation (life events per contact are small).
        setTotal(fetched.length);

        const relatedUids: string[] = [];
        for (const e of fetched) {
          if (e.related_entity_ids) {
            for (const rid of e.related_entity_ids) {
              if (rid !== uid && !relatedUids.includes(rid)) {
                relatedUids.push(rid);
              }
            }
          }
        }
        setContactsByUid(relatedUids.length > 0 ? await getContactsByUid(relatedUids) : new Map());
      } catch (err) {
        setError(handleFetchError(err, 'fetching life events'));
      } finally {
        setLoading(false);
      }
    },
    [entityId],
  );

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const handleCreate = useCallback(
    async (data: LifeEventInputData) => {
      await createLifeEvent(data);
      await refresh();
    },
    [refresh],
  );

  const handleUpdate = useCallback(
    async (id: string, data: LifeEventInputData, revision?: number) => {
      try {
        await updateLifeEvent(id, data, revision);
      } catch (err) {
        // Rethrown so the dialog stays open ("keep editing").
        if (onConflict && isPreconditionFailed(err)) onConflict();
        throw err;
      }
      await refresh();
    },
    [refresh, onConflict],
  );

  const handleDelete = useCallback(
    async (id: string) => {
      try {
        await deleteLifeEvent(id, events.find((e) => e.id === id)?.revision);
      } catch (err) {
        if (onConflict && isPreconditionFailed(err)) {
          onConflict();
          return;
        }
        throw err;
      }
      await refresh();
    },
    [refresh, onConflict, events],
  );

  return {
    events,
    total,
    contactsByUid,
    loading,
    error,
    refresh,
    handleCreate,
    handleUpdate,
    handleDelete,
  };
}
