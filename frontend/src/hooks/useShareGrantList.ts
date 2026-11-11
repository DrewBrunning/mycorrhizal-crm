import { useCallback, useRef, useState } from 'react';
import type { ShareGrantPage } from '../api/shareGrants';
import { getErrorMessage } from '../utils/errorHandler';

// Cursor-paginated list state for the standing-share pages (issue #2292). The
// grant list endpoints have deliberately no total, so the page offers "Load
// more" while next_cursor is non-empty. Rows are replaced/removed locally after
// an action so the list answers at once, without a second round trip.
export function useShareGrantList<T extends { id: string }>(
  fetchPage: (params: { cursor?: string }) => Promise<ShareGrantPage<T>>,
) {
  const [items, setItems] = useState<T[]>([]);
  const [nextCursor, setNextCursor] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  // Only the newest request may write state: a slow earlier response must not
  // overwrite a refresh that was started after it.
  const seq = useRef(0);

  const refresh = useCallback(async () => {
    const mine = ++seq.current;
    setLoading(true);
    setError('');
    try {
      const page = await fetchPage({});
      if (mine !== seq.current) return;
      setItems(page.share_grants);
      setNextCursor(page.next_cursor);
    } catch (err) {
      if (mine !== seq.current) return;
      setError(getErrorMessage(err));
    } finally {
      if (mine === seq.current) setLoading(false);
    }
  }, [fetchPage]);

  const loadMore = useCallback(async () => {
    if (!nextCursor) return;
    const mine = ++seq.current;
    setLoading(true);
    setError('');
    try {
      const page = await fetchPage({ cursor: nextCursor });
      if (mine !== seq.current) return;
      setItems((prev) => [...prev, ...page.share_grants]);
      setNextCursor(page.next_cursor);
    } catch (err) {
      if (mine !== seq.current) return;
      setError(getErrorMessage(err));
    } finally {
      if (mine === seq.current) setLoading(false);
    }
  }, [fetchPage, nextCursor]);

  const replace = useCallback((next: T) => {
    setItems((prev) => prev.map((item) => (item.id === next.id ? next : item)));
  }, []);

  const remove = useCallback((id: string) => {
    setItems((prev) => prev.filter((item) => item.id !== id));
  }, []);

  return { items, nextCursor, loading, error, refresh, loadMore, replace, remove };
}
