import { useCallback, useRef, useState } from 'react';
import { type GraphConnectionsResponse, getConnections } from '../api/graph';
import { handleFetchError } from '../utils/errorHandler';

// useConnections backs the contact page's multi-hop chain surface (T10): from
// a contact's VCardUID, fetch every reachable contact within a depth, with the
// relation chain describing each. The optional relation filter accepts a
// canonical token or a registry synonym ("brother" -> sibling_of, T11).
export function useConnections(fromUid: string | undefined) {
  const [connections, setConnections] = useState<GraphConnectionsResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // A depth/relation change can leave an older request in flight. Only the
  // newest request may write state, or a slow depth-1 response can land after
  // a fast depth-2 response and show the wrong network.
  const requestRef = useRef(0);

  const refresh = useCallback(
    async (opts?: { depth?: number; relation?: string; overrideUid?: string }) => {
      const uid = opts?.overrideUid ?? fromUid;
      if (!uid) return;
      const requestId = ++requestRef.current;
      setLoading(true);
      setError(null);
      try {
        const result = await getConnections({
          from: uid,
          // Default to 1 hop, matching ConnectionsPanel's default (testing
          // feedback) — the hook should never silently fall back to a wider
          // traversal than the panel's own default.
          depth: opts?.depth ?? 1,
          relation: opts?.relation?.trim() || undefined,
        });
        if (requestRef.current !== requestId) return;
        setConnections(result);
      } catch (err) {
        if (requestRef.current !== requestId) return;
        setError(handleFetchError(err, 'fetching connections'));
      } finally {
        if (requestRef.current === requestId) setLoading(false);
      }
    },
    [fromUid],
  );

  return {
    connections,
    loading,
    error,
    refresh,
  };
}
