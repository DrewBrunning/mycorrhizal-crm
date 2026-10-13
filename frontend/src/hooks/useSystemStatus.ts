import { useCallback, useEffect, useRef, useState } from 'react';
import { getSystemStatus, type SystemStatusResponse } from '../api/systemStatus';
import { handleFetchError } from '../utils/errorHandler';

// useSystemStatus drives the admin system-status snapshot (issue #649).
// Load-once with an explicit refresh — the snapshot is a server-side
// projection with no filters and no pagination, so it is as simple as
// useSubsystemHealth rather than useSystemEvents.
export function useSystemStatus() {
  const [data, setData] = useState<SystemStatusResponse | null>(null);
  // Stryker disable next-line BooleanLiteral: the mount effect sets loading=true before the first observable commit
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const requestRef = useRef(0);

  const refresh = useCallback(
    async () => {
      // Stryker disable next-line ArithmeticOperator: the counter has a single increment site, so any strictly monotonic id sequence works
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
    },
    // Stryker disable next-line ArrayDeclaration: refresh/run closes over state setters and a ref only (stable), so its deps array is equivalent either way
    [],
  );

  useEffect(
    () => {
      void refresh();
    },
    // Stryker disable next-line ArrayDeclaration: refresh is stable, so the mount effect runs exactly once under either deps array
    [refresh],
  );

  return { data, loading, error, refresh };
}
