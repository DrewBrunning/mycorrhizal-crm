import { useState, useCallback } from 'react';
import { getDuplicatePairs, dismissDuplicatePair, DuplicatePair } from '../api/duplicates';
import { handleFetchError, handleError, ErrorNotifier } from '../utils/errorHandler';

// Page size for the scan's offset pagination (backend caps at 100).
const DUPLICATES_PAGE_SIZE = 100;
// Hard cap on the number of pages the review surface will pull. The pairs
// list is bounded by the real duplicate set (review is inherently a
// full-scan surface); this guard exists only so a pathological address book
// cannot drive an unbounded fan-out.
const DUPLICATES_MAX_PAGES = 50;

// useDuplicatePairs drives the T93 duplicate-review surface
// (T93). The
// pairs are recomputed server-side on every fetch and dismissed pairs are
// already filtered out, so the client is stateless across reloads — refresh
// always restarts from page one and appends until the scan is exhausted.
export function useDuplicatePairs(notifier?: ErrorNotifier) {
  const [pairs, setPairs] = useState<DuplicatePair[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    setPairs([]);
    try {
      const all: DuplicatePair[] = [];
      let nextPage = 1;
      for (let guard = 0; guard < DUPLICATES_MAX_PAGES; guard++) {
        const response = await getDuplicatePairs({ page: nextPage, limit: DUPLICATES_PAGE_SIZE });
        all.push(...response.pairs);
        // Incremental: the list fills as each page lands instead of holding
        // the dialog on a blank spinner until a large scan is fully fetched.
        // Safe because the backend sorts the whole result set before
        // offsetting, so appending pages preserves the global strongest-first
        // order.
        setPairs(all.slice());
        if (all.length >= response.total) break;
        nextPage += 1;
      }
      setTotal(all.length);
    } catch (err) {
      setError(handleFetchError(err, 'scanning for duplicates'));
    } finally {
      setLoading(false);
    }
  }, []);

  // dismiss records a "not a duplicate" verdict, then refetches so the pair
  // disappears immediately.
  const dismiss = useCallback(
    async (pair: DuplicatePair) => {
      try {
        await dismissDuplicatePair(pair.a.uid || '', pair.b.uid || '');
        await refresh();
      } catch (err) {
        handleError(err, { operation: 'dismissing duplicate pair' }, notifier);
        throw err;
      }
    },
    [refresh, notifier]
  );

  return { pairs, total, loading, error, refresh, dismiss };
}
