import { useCallback, useEffect, useRef, useState } from 'react';
import { getSubsystemHealth, type SubsystemHealth } from '../api/subsystemHealth';
import { handleFetchError } from '../utils/errorHandler';

// useSubsystemHealth drives the per-subsystem last-known-good panel (issue
// #427). Load-once with an explicit refresh — the state is a server-side
// projection with no filters and no pagination, so it is far simpler than
// useSystemEvents.
export function useSubsystemHealth() {
  const [data, setData] = useState<SubsystemHealth[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const requestRef = useRef(0);

  const refresh = useCallback(async () => {
    const requestId = requestRef.current + 1;
    requestRef.current = requestId;
    setLoading(true);
    setError(null);
    try {
      const resp = await getSubsystemHealth();
      if (requestRef.current !== requestId) return;
      setData(resp.subsystems || []);
    } catch (err) {
      if (requestRef.current !== requestId) return;
      setError(handleFetchError(err, 'fetching subsystem health'));
    } finally {
      if (requestRef.current === requestId) setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  return { data, loading, error, refresh };
}
