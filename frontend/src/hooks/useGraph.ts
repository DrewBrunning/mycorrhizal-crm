// Custom hook for fetching network graph data
import { useCallback, useEffect, useRef, useState } from 'react';
import { getGraph } from '../api/graph';
import { isAuthenticated } from '../auth';
import type { GraphData, GraphEdge, GraphNode } from '../types/graph';
import { handleFetchError } from '../utils/errorHandler';

interface UseGraphResult {
  data: GraphData | null;
  loading: boolean;
  error: string | null;
  refetch: () => Promise<void>;
}

// full: fetch the whole account's graph instead of the default ego-network
// of the self contact (issue #1791). Changing it refetches.
// includeEntities: also fetch the opt-in entity layer (ADR 0043 decision 2).
export function useGraph(full = false, includeEntities = false): UseGraphResult {
  const [data, setData] = useState<GraphData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const requestRef = useRef(0);

  const fetchGraph = useCallback(async () => {
    // Stryker disable next-line ArithmeticOperator: the counter has a single increment site, so any strictly monotonic id sequence works
    const requestId = requestRef.current + 1;
    requestRef.current = requestId;
    setLoading(true);
    setError(null);

    try {
      if (!isAuthenticated()) {
        throw new Error('No authentication token found');
      }

      const params = {
        ...(full ? { full: true } : {}),
        ...(includeEntities ? { includeEntities: true } : {}),
      };
      const response = Object.keys(params).length > 0 ? await getGraph(params) : await getGraph();

      if (requestRef.current !== requestId) {
        return;
      }

      // Entity nodes/edges arrive under their own keys and are merged here as
      // `entity`-typed items, so no consumer of contact/relationship data ever
      // sees them unless it opts in by type.
      const entityNodes: GraphNode[] = (response.entity_nodes ?? []).map((n) => ({
        id: n.id,
        type: 'entity',
        label: n.label,
        entity_kind: n.kind,
      }));
      const entityEdges: GraphEdge[] = (response.entity_edges ?? []).map((e) => ({
        id: e.id,
        source: e.source,
        target: e.target,
        type: 'entity',
        label: '',
      }));
      setData({
        nodes: [...(response.nodes || []), ...entityNodes],
        edges: [...(response.edges || []), ...entityEdges],
        scoped: response.scoped,
        depth: response.depth,
      });
    } catch (err) {
      if (requestRef.current !== requestId) {
        return;
      }
      const message = handleFetchError(err, 'fetching network graph');
      setError(message);
    } finally {
      if (requestRef.current === requestId) {
        setLoading(false);
      }
    }
  }, [full, includeEntities]);

  useEffect(
    () => {
      void fetchGraph();
    },
    // Stryker disable next-line ArrayDeclaration: fetchGraph is stable, so the mount effect runs exactly once under either deps array
    [fetchGraph],
  );

  return {
    data,
    loading,
    error,
    refetch: fetchGraph,
  };
}
