import { useState, useCallback } from 'react';
import {
  getTimeline,
  TIMELINE_TYPES,
  TimelineItem,
  TimelineType,
  TimelineBucket,
} from '../api/timeline';
import { handleFetchError } from '../utils/errorHandler';

/**
 * Data hook for the T78 timeline explorer: owns the current page of merged
 * timeline items plus the type/recency filters and cursor pagination state,
 * mirroring the useRelationshipEdges shape the ticket names as the pattern
 * to follow.
 *
 * `refresh` replaces the list with a fresh first page and is memoized on the
 * filter values, so callers just re-invoke it (or let a `[refresh]`-depended
 * effect re-fire) when filters change. `loadMore` appends the next cursor
 * page -- the explorer must paginate, never become a second unbounded fetch.
 */
export function useTimeline(contactId: string | number | undefined) {
  const [items, setItems] = useState<TimelineItem[]>([]);
  const [nextCursor, setNextCursor] = useState('');
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // Empty selection means "all types" (the backend treats an absent ?type=
  // the same way), so the default is the full set.
  const [types, setTypes] = useState<TimelineType[]>(TIMELINE_TYPES.slice());
  const [bucket, setBucket] = useState<TimelineBucket>('all');

  const refresh = useCallback(async () => {
    if (!contactId) return;
    setLoading(true);
    setError(null);
    try {
      const response = await getTimeline({ contactId, types, bucket });
      setItems(response.items || []);
      setNextCursor(response.next_cursor || '');
    } catch (err) {
      setError(handleFetchError(err, 'fetching timeline'));
    } finally {
      setLoading(false);
    }
  }, [contactId, types, bucket]);

  const loadMore = useCallback(async () => {
    if (!contactId || !nextCursor) return;
    setLoadingMore(true);
    setError(null);
    try {
      const response = await getTimeline({ contactId, types, bucket, cursor: nextCursor });
      setItems((prev) => [...prev, ...(response.items || [])]);
      setNextCursor(response.next_cursor || '');
    } catch (err) {
      setError(handleFetchError(err, 'fetching more timeline'));
    } finally {
      setLoadingMore(false);
    }
  }, [contactId, types, bucket, nextCursor]);

  return {
    items,
    nextCursor,
    loading,
    loadingMore,
    error,
    types,
    setTypes,
    bucket,
    setBucket,
    refresh,
    loadMore,
  };
}
