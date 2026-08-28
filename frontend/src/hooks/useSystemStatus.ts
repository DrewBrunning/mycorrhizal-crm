import { useCallback, useEffect, useRef, useState } from 'react';
import { getSystemStatus, type SystemStatusResponse } from '../api/systemStatus';
import { handleFetchError } from '../utils/errorHandler';

// useSystemStatus drives the admin system-status snapshot (issue #649).
// Load-once with an explicit refresh — the snapshot is a server-side
// projection with no filters and no pagination, so it is as simple as
// useSubsystemHealth rather than useSystemEvents.
export function useSystemStatus() {
  const [data, setData] = useState<SystemStatusResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const requestRef = useRef(0);

  const refresh = useCallback(async () => {
    const requestId = requestRef.current + 1;
    requestRef.current = requestId;
    setLoading(true);
    setError(null);
    try {
      const resp = await getSystemStatus();
      if (requestRef.current !== requestId) return;
      setData(resp);
    } catch (err) {
      if (requestRef.current !== requestId) return;
      setError(handleFetchError(err, 'fetching system status'));
    } finally {
      if (requestRef.current === requestId) setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  return { data, loading, error, refresh };
}
