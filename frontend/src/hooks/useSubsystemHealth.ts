import { useCallback, useEffect, useRef, useState } from 'react';
import { getSubsystemHealth, type SubsystemHealth } from '../api/subsystemHealth';
import { handleFetchError } from '../utils/errorHandler';

// useSubsystemHealth drives the per-subsystem last-known-good panel (issue
// #427). Load-once with an explicit refresh — the state is a server-side
// projection with no filters and no pagination, so it is far simpler than
// useSystemEvents.
export function useSubsystemHealth() {
  const [data, setData] = useState<SubsystemHealth[]>([]);
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
        const resp = await getSubsystemHealth();
        if (requestRef.current !== requestId) return;
        setData(resp.subsystems || []);
      } catch (err) {
        if (requestRef.current !== requestId) return;
        setError(handleFetchError(err, 'fetching subsystem health'));
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
