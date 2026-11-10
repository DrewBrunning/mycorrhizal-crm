import { useCallback, useEffect, useRef, useState } from 'react';
import { type Entity, type EntityKind, listEntities } from '../api/entities';
import { getErrorMessage } from '../utils/errorHandler';

// Paged list of the caller's entities of one kind, optionally narrowed by a
// name query (issue #2301). A request counter drops any response that is no
// longer the latest request, so a slow reply for the previous tab or query can
// never overwrite the current list (same guard as useContacts).
export function useEntityList(kind: EntityKind, query: string) {
  const [entities, setEntities] = useState<Entity[]>([]);
  const [nextCursor, setNextCursor] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const requestRef = useRef(0);
  const q = query.trim();

  const load = useCallback(
    async (cursor?: string) => {
      const requestId = ++requestRef.current;
      setLoading(true);
      setError(null);
      try {
        const page = await listEntities({ kind, q: q || undefined, cursor, limit: 50 });
        if (requestRef.current !== requestId) return;
        setEntities((prev) => (cursor ? [...prev, ...page.entities] : page.entities));
        setNextCursor(page.next_cursor);
      } catch (err) {
        if (requestRef.current !== requestId) return;
        setError(getErrorMessage(err));
      } finally {
        if (requestRef.current === requestId) setLoading(false);
      }
    },
    [kind, q],
  );

  useEffect(() => {
    setEntities([]);
    setNextCursor('');
    void load();
  }, [load]);

  const loadMore = useCallback(() => load(nextCursor), [load, nextCursor]);
  const refresh = useCallback(() => load(), [load]);

  return { entities, loading, error, hasMore: nextCursor !== '', loadMore, refresh };
}
