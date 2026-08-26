import { useCallback, useState } from 'react';
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

  const refresh = useCallback(
    async (opts?: { depth?: number; relation?: string; overrideUid?: string }) => {
      const uid = opts?.overrideUid ?? fromUid;
      if (!uid) return;
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
        setConnections(result);
      } catch (err) {
        setError(handleFetchError(err, 'fetching connections'));
      } finally {
        setLoading(false);
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
