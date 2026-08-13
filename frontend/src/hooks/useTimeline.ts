import { useState, useCallback, useEffect, useRef } from 'react';
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
 *
 * A monotonic request epoch (`requestSeq`) guards every fetch the same way
 * `useActivities`/`useAudit`/`useGraph` do: each request claims the next
 * epoch, and a response whose epoch is no longer current (a filter changed
 * or "load more" was clicked again while it was in flight, or the contact
 * changed underneath it) is discarded instead of appending stale rows over
 * fresh ones.
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
  const requestSeq = useRef(0);

  // A different contact is a clean slate. The dialog stays mounted across
  // contact navigation (same route component), so without this the previous
  // contact's page would linger and briefly render -- and an in-flight fetch
  // started for it could resolve after the switch. Bumping the epoch retires
  // those, and clearing the page prevents the wrong-contact flash.
  useEffect(() => {
    requestSeq.current += 1;
    setItems([]);
    setNextCursor('');
    setLoading(false);
    setLoadingMore(false);
    setError(null);
  }, [contactId]);

  const refresh = useCallback(async () => {
    if (!contactId) return;
    const seq = ++requestSeq.current;
    setLoading(true);
    setLoadingMore(false); // a refresh supersedes any in-flight load-more
    setError(null);
    try {
      const response = await getTimeline({ contactId, types, bucket });
      if (seq !== requestSeq.current) return;
      setItems(response.items || []);
      setNextCursor(response.next_cursor || '');
    } catch (err) {
      if (seq !== requestSeq.current) return;
      setError(handleFetchError(err, 'fetching timeline'));
    } finally {
      if (seq === requestSeq.current) setLoading(false);
    }
  }, [contactId, types, bucket]);

  const loadMore = useCallback(async () => {
    if (!contactId || !nextCursor) return;
    const seq = ++requestSeq.current;
    setLoadingMore(true);
    setError(null);
    try {
      const response = await getTimeline({ contactId, types, bucket, cursor: nextCursor });
      if (seq !== requestSeq.current) return;
      setItems((prev) => [...prev, ...(response.items || [])]);
      setNextCursor(response.next_cursor || '');
    } catch (err) {
      if (seq !== requestSeq.current) return;
      setError(handleFetchError(err, 'fetching more timeline'));
    } finally {
      if (seq === requestSeq.current) setLoadingMore(false);
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
