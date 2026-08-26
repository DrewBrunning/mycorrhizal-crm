import { useCallback, useMemo, useState } from 'react';
import {
  type ConversationAgenda,
  createConversationAgenda,
  deleteConversationAgenda,
  discussConversationAgenda,
  getConversationAgenda,
  updateConversationAgenda,
} from '../api/conversationAgenda';
import { handleFetchError } from '../utils/errorHandler';

// useConversationAgenda backs the contact page's conversation-agenda surface
// (T21): the list of "things to bring up next time I see them" for one
// contact. Items are keyed to the contact by VCardUID (entityId) and resolved
// by marking them discussed — never by a date.
export function useConversationAgenda(entityId: string | undefined) {
  const [items, setItems] = useState<ConversationAgenda[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(
    async (overrideEntityId?: string) => {
      const uid = overrideEntityId ?? entityId;
      if (!uid) return;
      setLoading(true);
      setError(null);
      try {
        const response = await getConversationAgenda({ entityId: uid, limit: 100 });
        setItems(response.conversation_agenda || []);
      } catch (err) {
        setError(handleFetchError(err, 'fetching conversation agenda'));
      } finally {
        setLoading(false);
      }
    },
    [entityId],
  );

  const openItems = useMemo(() => items.filter((i) => i.discussed_at == null), [items]);
  const discussedItems = useMemo(() => items.filter((i) => i.discussed_at != null), [items]);

  const handleCreate = useCallback(
    async (data: { entity_id: string; content: string; reference_url?: string }) => {
      await createConversationAgenda(data);
      await refresh();
    },
    [refresh],
  );

  const handleUpdate = useCallback(
    async (id: string, data: { entity_id: string; content: string; reference_url?: string }) => {
      await updateConversationAgenda(id, data);
      await refresh();
    },
    [refresh],
  );

  const handleDiscuss = useCallback(
    async (id: string, activityId?: number) => {
      await discussConversationAgenda(id, activityId);
      await refresh();
    },
    [refresh],
  );

  const handleDelete = useCallback(
    async (id: string) => {
      await deleteConversationAgenda(id);
      await refresh();
    },
    [refresh],
  );

  return {
    items,
    openItems,
    discussedItems,
    loading,
    error,
    refresh,
    handleCreate,
    handleUpdate,
    handleDiscuss,
    handleDelete,
  };
}
