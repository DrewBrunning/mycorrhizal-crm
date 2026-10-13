import { useCallback, useEffect, useRef, useState } from 'react';
import {
  getNotificationChannelHealth,
  type NotificationChannelHealth,
} from '../api/notificationHealth';
import { handleFetchError } from '../utils/errorHandler';

// useNotificationChannelHealth drives the per-channel notification delivery
// panel (issue #422). Load-once with an explicit refresh, mirroring
// useSubsystemHealth — the state is a server-side projection with no filters
// and no pagination.
export function useNotificationChannelHealth() {
  const [data, setData] = useState<NotificationChannelHealth[]>([]);
  // Stryker disable next-line BooleanLiteral: the mount effect sets loading=true before the first observable commit
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const requestRef = useRef(0);

  const refresh = useCallback(
    async () => {
      // Stryker disable next-line ArithmeticOperator: any strictly monotonic id sequence works; only inequality between successive ids is observable
      const requestId = requestRef.current + 1;
      requestRef.current = requestId;
      setLoading(true);
      setError(null);
      try {
        const resp = await getNotificationChannelHealth();
        if (requestRef.current !== requestId) return;
        setData(resp.channels || []);
      } catch (err) {
        if (requestRef.current !== requestId) return;
        setError(handleFetchError(err, 'fetching notification channel health'));
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
