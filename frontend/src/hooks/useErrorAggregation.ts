import { useCallback, useEffect, useRef, useState } from 'react';
import { type ErrorBucket, getErrorAggregation } from '../api/errorAggregation';
import { handleFetchError } from '../utils/errorHandler';

// useErrorAggregation drives the operational-error rollup panel (issue #426).
// Load-once with an explicit refresh over a fixed window — a server-side
// projection with no pagination, so it stays as simple as useSubsystemHealth.
export function useErrorAggregation(windowHours = 24) {
  const [buckets, setBuckets] = useState<ErrorBucket[]>([]);
  const [totalEvents, setTotalEvents] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const requestRef = useRef(0);

  const refresh = useCallback(async () => {
    const requestId = requestRef.current + 1;
    requestRef.current = requestId;
    setLoading(true);
    setError(null);
    try {
      const resp = await getErrorAggregation(windowHours);
      if (requestRef.current !== requestId) return;
      setBuckets(resp.buckets || []);
      setTotalEvents(resp.total_events || 0);
    } catch (err) {
      if (requestRef.current !== requestId) return;
      setError(handleFetchError(err, 'fetching error aggregation'));
    } finally {
      if (requestRef.current === requestId) setLoading(false);
    }
  }, [windowHours]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  return { buckets, totalEvents, loading, error, refresh };
}
