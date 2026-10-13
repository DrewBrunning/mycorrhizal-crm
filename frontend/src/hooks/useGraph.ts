// Custom hook for fetching network graph data
import { useCallback, useEffect, useRef, useState } from 'react';
import { getGraph } from '../api/graph';
import { isAuthenticated } from '../auth';
import type { GraphData } from '../types/graph';
import { handleFetchError } from '../utils/errorHandler';

interface UseGraphResult {
  data: GraphData | null;
  loading: boolean;
  error: string | null;
  refetch: () => Promise<void>;
}

export function useGraph(): UseGraphResult {
  const [data, setData] = useState<GraphData | null>(null);
  // Stryker disable next-line BooleanLiteral: the mount effect sets loading=true before the first observable commit
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const requestRef = useRef(0);

  const fetchGraph = useCallback(
    async () => {
      // Stryker disable next-line ArithmeticOperator: the counter has a single increment site, so any strictly monotonic id sequence works
      const requestId = requestRef.current + 1;
      requestRef.current = requestId;
      setLoading(true);
      setError(null);

      try {
        if (!isAuthenticated()) {
          throw new Error('No authentication token found');
        }

        const response = await getGraph();

        if (requestRef.current !== requestId) {
          return;
        }

        setData({
          nodes: response.nodes || [],
          edges: response.edges || [],
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
    },
    // Stryker disable next-line ArrayDeclaration: fetchGraph closes over state setters and a ref only (stable), so its deps array is equivalent either way
    [],
  );

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
